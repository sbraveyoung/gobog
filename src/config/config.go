package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
)

var (
	C Config
)

type LogConfig struct {
	FileName string
	Level    int
	Maxlines int
	Maxsize  int
	Daily    bool
	Maxdays  int
	Color    bool
	Perm     string
}

type HttpConfig struct {
	Addr  string
	Addrs string
	Cert  string
	Key   string
}

type BlogConfig struct {
	Domain      string
	Title       string
	Subtitle    string
	Description string
	Author      string
	Theme       string
	Source      string
}

type Config struct {
	Blog BlogConfig
	Http HttpConfig
	Log  LogConfig
}

func init() {
	configPath := flag.String("config", "./conf/config.toml", "config path")
	flag.Parse()

	data, err := ioutil.ReadFile(*configPath)
	if err != nil {
		fmt.Println("open config file:", err)
		os.Exit(1)
	}
	toml.Decode(string(data), &C)
	fmt.Printf("config:%+v\n", C)
}
