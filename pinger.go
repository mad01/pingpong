package main

import (
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

type pingServer struct {
	cc *grpc.ClientConn
}

func (p *pingServer) MsgConn(addr string) error {
	cc, err := clientGRPCconn(addr)
	if err != nil {
		return err
	}
	p.cc = cc

	return nil
}

func (p *pingServer) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PongResponse, error) {
	client := pb.NewRandomMsgClient(p.cc)
	msgResp, err := client.GetRandomMsg(context.Background(), &pb.RandomMsgRequest{})
	if err != nil {
		return nil, fmt.Errorf("Fail to get msg from downstream: %v", err.Error())
	}

	response := pb.PongResponse{Msg: msgResp.Msg}
	return &response, nil
}

func servePingGRPC(addr string, errChan chan error, conf *config) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	//
	// zipkin/appdash

	collector := appdash.NewRemoteCollector(appDashHTTPEndpoint)
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

	pinger := pingServer{}
	pinger.MsgConn(conf.grpcMsgAddr)

	pb.RegisterPingerServer(middlewareServer, &pinger)

	grpc_prometheus.Register(middlewareServer)
	grpc_prometheus.EnableHandlingTimeHistogram()

	// Register reflection service on gRPC server.
	reflection.Register(middlewareServer)

	go func() {
		errChan <- middlewareServer.Serve(lis)
	}()

}

func servePingHTTP(addr string, errChan chan error) {
	http.Handle("/metrics2", promhttp.Handler())
	go func() {
		errChan <- http.ListenAndServe(addr, nil)
	}()
}

func servePingAll(c *config) {
	errChan := make(chan error, 10)

	go servePingHTTP(c.httpPingerAddr, errChan)
	go servePingGRPC(c.grpcPingerAddr, errChan, c)

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
