package main

import (
	"flag"
	"fmt"

	"github.com/mad01/pingpong/common"
)

type config struct {
	ServerPort int
	Version    bool
}

func newCmd() *config {
	c := new(config)
	flag.IntVar(&c.ServerPort, "server.port", 8888, "server port")
	flag.BoolVar(&c.Version, "version", false, "show version")
	flag.Parse()

	if c.Version {
		fmt.Printf("Version: %v", common.Version)
	}

	return c
}

func main() {
	newCmd()
}
