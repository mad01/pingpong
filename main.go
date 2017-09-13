package main

import (
	"flag"
	"fmt"
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
		fmt.Printf("Version: %v", Version)
	}

	return c
}

func main() {
	newCmd()
}
