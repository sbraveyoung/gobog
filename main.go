package main

import (
	"fmt"
	"os"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/dao"
	"github.com/SmartBrave/gobog/pkg/server"
	"github.com/logs"
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

	logs.LogInit(c.Log.RunMode, c.Log.LogFileName, c.Log.LogMaxLines, c.Log.LogMaxSize, c.Log.LogDaily, c.Log.LogRotate, c.Log.LogLevel)
	logs.SetLogFuncCall(true)

	if err = dao.Init(c); err != nil {
		fmt.Printf("dao.Init() fail. err: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	server.New(c)
}
