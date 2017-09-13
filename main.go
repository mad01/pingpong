package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sourcegraph.com/sourcegraph/appdash"
	appdashtracer "sourcegraph.com/sourcegraph/appdash/opentracing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	pb "github.com/mad01/pingpong/com"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

//
// Server
//
var (
	zipkinHTTPEndpoint  = "0.0.0.0:9411"
	appDashHTTPEndpoint = "0.0.0.0:8700"
)

type config struct {
	server   bool
	clinet   bool
	msg      string
	grpcAddr string
	httpAddr string
	Version  bool
}

func newServerCmd() *config {
	c := new(config)
	flag.StringVar(&c.grpcAddr, "grpc.addr", "0.0.0.0:8881", "grpc server port")
	flag.StringVar(&c.httpAddr, "http.addr", "0.0.0.0:8882", "http server port")
	flag.BoolVar(&c.Version, "version", false, "show version")
	flag.BoolVar(&c.server, "server", false, "run as server")
	flag.BoolVar(&c.clinet, "client", false, "run as client")
	flag.StringVar(&c.msg, "msg", "foobar", "message to send in ping")
	flag.Parse()

	if c.Version {
		fmt.Printf("Version: %v", Version)
	}

	return c
}

type server struct{}

func (s *server) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PongResponse, error) {
	response := pb.PongResponse{Msg: in.Msg}
	return &response, nil
}

func serveGRPC(addr string, errChan chan error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	//
	// zipkin/appdash

	collector := appdash.NewRemoteCollector("localhost:8700")
	tracer := appdashtracer.NewTracer(collector)

	// collector, err := zipkin.NewHTTPCollector(zipkinHTTPEndpoint)
	// if err != nil {
	// 	errChan <- err
	// }
	// tracer, err := zipkin.NewTracer(
	// 	zipkin.NewRecorder(collector, false, "0.0.0.0:0", "pingpong"),
	// )
	// if err != nil {
	// 	errChan <- err
	// }

	//
	//

	//
	// zap
	zapOpts := []grpc_zap.Option{
		grpc_zap.WithDurationField(func(duration time.Duration) zapcore.Field {
			return zap.Int64("grpc.time_ns", duration.Nanoseconds())
		}),
	}
	zapLogger, _ := zap.NewProduction()
	defer zapLogger.Sync() // flushes buffer, if any
	//
	//

	middlewareServer := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zapLogger, zapOpts...),
			grpc_prometheus.StreamServerInterceptor,
		)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_ctxtags.UnaryServerInterceptor(),
			otgrpc.OpenTracingServerInterceptor(tracer),
			grpc_zap.UnaryServerInterceptor(zapLogger, zapOpts...),
			grpc_prometheus.UnaryServerInterceptor,
		)),
	)

	pb.RegisterPingerServer(middlewareServer, &server{})

	grpc_prometheus.Register(middlewareServer)
	grpc_prometheus.EnableHandlingTimeHistogram()

	// Register reflection service on gRPC server.
	reflection.Register(middlewareServer)

	go func() {
		errChan <- middlewareServer.Serve(lis)
	}()

}

func serveHTTP(addr string, errChan chan error) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		errChan <- http.ListenAndServe(addr, nil)
	}()
}

func serveAll(c *config) {
	errChan := make(chan error, 10)

	go serveHTTP(c.httpAddr, errChan)
	go serveGRPC(c.grpcAddr, errChan)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errChan:
			if err != nil {
				fmt.Printf("%s \n", err)
				os.Exit(1)
			}
		case <-signalChan:
			fmt.Println("Shutdown signal received, exiting...")
			os.Exit(0)
		}
	}
}

//
// End Server
//

//
// Client
//

func clientGRPCconn(addr string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err.Error())
	}
	return conn, nil
}

func clientPing(cc *grpc.ClientConn, msg string) {
	client := pb.NewPingerClient(cc)

	request := pb.PingRequest{Msg: msg}
	resp, err := client.Ping(context.Background(), &request)
	if err != nil {
		fmt.Printf("ping err: %v", err.Error())
		os.Exit(1)
	}
	fmt.Printf("Pong: %v", resp.Msg)
}

//
// End Client
//

func main() {
	conf := newServerCmd()
	if conf.server {
		serveAll(conf)
	}

	if conf.clinet {
		cc, err := clientGRPCconn(conf.grpcAddr)
		if err != nil {
			fmt.Printf("Fail connect to server: %v", err.Error())
			os.Exit(1)
		}
		clientPing(cc, conf.msg)
	}

}
