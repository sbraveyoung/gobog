package main

import (
	"fmt"
	"os"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/server"
)

var (
	c *config.Config
)

func init() {
	var err error
	c, err = config.New()
	if err != nil {
		fmt.Printf("config.New() fail. err: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	server.New(c.Http.Addr)
}
