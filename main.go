package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/grpc-ecosystem/grpc-opentracing/go/otgrpc"
	pb "github.com/mad01/pingpong/com"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

//
// Server
//
var (
	zipkinHTTPEndpoint  = "0.0.0.0:9411"
	appDashHTTPEndpoint = "0.0.0.0:5775"
)

type config struct {
	server         bool
	clinet         bool
	msg            string
	grpcPingerAddr string
	httpPingerAddr string
	httpMsgAddr    string
	grpcMsgAddr    string
	Version        bool
}

func newServerCmd() *config {
	c := new(config)
	flag.StringVar(&c.grpcPingerAddr, "grpc.ping.addr", "0.0.0.0:8881", "grpc ping server port")
	flag.StringVar(&c.httpPingerAddr, "http.ping.addr", "0.0.0.0:8882", "http ping server port")
	flag.StringVar(&c.grpcMsgAddr, "grpc.msg.addr", "0.0.0.0:8883", "grpc msg server port")
	flag.StringVar(&c.httpMsgAddr, "http.msg.addr", "0.0.0.0:8884", "http msg server port")
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

//
// End Server
//

//
// Client
//

func clientGRPCconn(addr, name string) (*grpc.ClientConn, io.Closer, error) {
	tracer, closer, err := getTracer(name)
	if err != nil {
		return nil, nil, err
	}
	conn, err := grpc.Dial(
		addr,
		grpc.WithInsecure(),
		grpc.WithUnaryInterceptor(otgrpc.OpenTracingClientInterceptor(*tracer)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server: %v", err.Error())
	}
	return conn, closer, nil
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
		go serveRandomMsgAll(conf)
		servePingAll(conf)
	}

	if conf.clinet {
		cc, closer, err := clientGRPCconn(conf.grpcPingerAddr, "cli")
		defer cc.Close()
		defer closer.Close()
		if err != nil {
			fmt.Printf("Fail connect to server: %v", err.Error())
			os.Exit(1)
		}
		clientPing(cc, conf.msg)
	}

}
