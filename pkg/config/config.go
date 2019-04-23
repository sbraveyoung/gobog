package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

var configPath = flag.String("config", "./conf/cfg.toml", "config path")

type LogConfig struct {
	FileName string `mapstructure:"filename" json:"filename"`
	Level    int    `mapstructure:"level" json:"level"`
	Maxlines int    `mapstructure:"maxlines" json:"maxlines"`
	Maxsize  int    `mapstructure:"maxsize" json:"maxsize"`
	Daily    bool   `mapstructure:"daily" json:"daily"`
	Maxdays  int    `mapstructure:"maxdays" json:"maxdays"`
	Color    bool   `mapstructure:"color" json:"color"`
	Perm     string `mapstructure:"perm" json:"perm"`
}

type HttpConfig struct {
	Addr  []string `mapstructure:"addr"`
	Addrs []string `mapstructure:"addrs"`
	Cert  string   `mapstructure:"cert"`
	Key   string   `mapstructure:"key"`
}

type BlogConfig struct {
	Domain      string `mapstructure:"domain"`
	Title       string `mapstructure:"title"`
	Subtitle    string `mapstructure:"subtitle"`
	Description string `mapstructure:"description"`
	Author      string `mapstructure:"author"`
	Theme       string `mapstructure:"theme"`
	Source      string `mapstructure:"source"`
}

type Config struct {
	Blog BlogConfig `mapstructure:"blog"`
	Http HttpConfig `mapsturcture:"http"` //console the time and other args of server
	Log  LogConfig  `mapstructure:"log"`
	once sync.Once
}

func (c *Config) parseFlag() {
	flag.Parse()
	if _, err := os.Stat(*configPath); err != nil {
		fmt.Printf("check config file: %s happend error: %v\n", *configPath, err)
		os.Exit(1)
	}

	for index, arg := range os.Args {
		arg = strings.TrimPrefix(arg, "-")
		switch arg {
		case "h", "help":
			showHelp()
			os.Exit(0)
		case "init":
			var path string
			if index+1 < len(os.Args) {
				path = os.Args[index+1]
			} else {
				var err error
				if path, err = os.Getwd(); err != nil {
					fmt.Printf("get current dir fail. err: %v\n", err)
					os.Exit(1)
				}
			}
			if err := initWorkSpace(path); err != nil {
				fmt.Printf("init dir %s fail. err: %v\n", path, err)
				os.Exit(1)
			}
			os.Exit(0)
		case "version":
			showVersion()
			os.Exit(0)
		default:
		}
	}
}

func (c *Config) saveConfig() {
	viper.SetConfigFile(*configPath)
	viper.SetConfigType("toml")
	viper.ReadInConfig()

	if err := viper.Unmarshal(c); err != nil {
		fmt.Printf("save config file %s fail. err: %v\n", *configPath, err)
		os.Exit(1)
	}
}

func New() (c *Config, err error) {
	c = &Config{}

	c.once.Do(func() {
		c.parseFlag()
		c.saveConfig()
	})

	return c, nil
}

func showHelp() {
	helpInfo := `Usage: %s <command>
	
Commands:
	help            Get help on a command.
	version         Show version information.
	init [path]     Create a new blog folder. It default current folder if no path option.
`

	fmt.Printf(helpInfo, strings.TrimPrefix(os.Args[0], "./"))
}

func showVersion() {
	fmt.Println("version 1.0")
}
