package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	cc     *grpc.ClientConn
	closer io.Closer
}

func (p *pingServer) MsgConn(addr string) error {
	cc, closer, err := clientGRPCconn(addr, "pinger")
	if err != nil {
		return err
	}
	p.closer = closer
	p.cc = cc

	return nil
}

func (p *pingServer) Ping(ctx context.Context, in *pb.PingRequest) (*pb.PongResponse, error) {
	client := pb.NewRandomMsgClient(p.cc)
	msgResp, err := client.GetRandomMsg(ctx, &pb.RandomMsgRequest{})
	if err != nil {
		return nil, fmt.Errorf("Fail to get msg from downstream: %v", err.Error())
	}

	response := pb.PongResponse{Msg: msgResp.Msg}
	return &response, nil
}

func servePingGRPC(conf *config, errChan chan error) {
	lis, err := net.Listen("tcp", conf.grpcPingerAddr)
	if err != nil {
		errChan <- err
	}

	//
	// opentracing

	tracer, closer, err := getTracer("pinger")
	if err != nil {
		errChan <- err
	}

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
			otgrpc.OpenTracingServerInterceptor(*tracer),
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
		defer pinger.cc.Close()
		defer pinger.closer.Close()
		defer closer.Close()
		errChan <- middlewareServer.Serve(lis)
	}()

}

func servePingHTTP(conf *config, errChan chan error) {
	http.Handle("/metrics2", promhttp.Handler())
	go func() {
		errChan <- http.ListenAndServe(conf.httpPingerAddr, nil)
	}()
}

func servePingAll(c *config) {
	errChan := make(chan error, 10)

	go servePingHTTP(c, errChan)
	go servePingGRPC(c, errChan)

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
