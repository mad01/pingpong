package main

import (
	"fmt"
	"log"
	"math/rand"
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

type randomMsgServer struct{}

func (s *randomMsgServer) GetRandomMsg(ctx context.Context, in *pb.RandomMsgRequest) (*pb.RandomMsgResponse, error) {
	time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)
	response := pb.RandomMsgResponse{Msg: "funny random message"}
	return &response, nil
}

func serveRandomMsgGRPC(addr string, errChan chan error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	//
	// appdash

	collector := appdash.NewRemoteCollector(appDashHTTPEndpoint)
	tracer := appdashtracer.NewTracer(collector)

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

	pb.RegisterRandomMsgServer(middlewareServer, &randomMsgServer{})

	grpc_prometheus.Register(middlewareServer)
	grpc_prometheus.EnableHandlingTimeHistogram()

	// Register reflection service on gRPC server.
	reflection.Register(middlewareServer)

	go func() {
		errChan <- middlewareServer.Serve(lis)
	}()

}

func serveRandomMsgHTTP(addr string, errChan chan error) {
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		errChan <- http.ListenAndServe(addr, nil)
	}()
}

func serveRandomMsgAll(c *config) {
	errChan := make(chan error, 10)

	go serveRandomMsgHTTP(c.httpMsgAddr, errChan)
	go serveRandomMsgGRPC(c.grpcMsgAddr, errChan)

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
