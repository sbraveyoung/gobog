package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/SmartBrave/gobog/pkg/config"
	"github.com/SmartBrave/gobog/pkg/dao"
	"github.com/SmartBrave/gobog/pkg/server"
	"github.com/astaxie/beego/logs"
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

	logConfig, err := json.Marshal(c.Log)
	if err != nil {
		fmt.Printf("marshal fail. err: %v\n", err)
		os.Exit(1)
	}
	logs.EnableFuncCallDepth(true)
	logs.SetLogger(logs.AdapterFile, string(logConfig))

	if err = dao.Init(c); err != nil {
		fmt.Printf("dao.Init() fail. err: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	blog := server.New(c)
	blog.Run()
}
