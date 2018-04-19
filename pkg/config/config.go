package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/SmartBrave/gobog/cmd"
	"github.com/spf13/viper"
)

const (
	CFG = "./conf/cfg.toml"
)

type MysqlConfig struct {
}

type LogConfig struct {
	formatStr string `mapstructure:"formatStr"`
}

type ArticleConfig struct {
}

type HttpConfig struct {
	Addr string `mapstructure:"addr"`
}

type Config struct {
	ConfigFile string        `mapstructure:"configFile"`
	Mysql      MysqlConfig   `mapstructure:"mysql"`
	Log        LogConfig     `mapstructure:"log"`
	Article    ArticleConfig `mapstructure:"article"`
	Http       HttpConfig    `mapsturcture:"http"`
	once       sync.Once
}

func (c *Config) parseFlag() {
	c.ConfigFile = CFG
	if _, err := os.Stat(c.ConfigFile); err != nil {
		fmt.Printf("check config file: %s happend error: %v\n", c.ConfigFile, err)
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
			if err := cmd.InitWorkSpace(path); err != nil {
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
	viper.SetConfigFile(c.ConfigFile)
	viper.SetConfigType("toml")
	viper.ReadInConfig()

	if err := viper.Unmarshal(c); err != nil {
		fmt.Printf("save config file %s fail. err: %v\n", c.ConfigFile, err)
		os.Exit(1)
	}
}

func New() (c *Config, err error) {
	c = &Config{
		Mysql:   MysqlConfig{},
		Log:     LogConfig{},
		Article: ArticleConfig{},
	}

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
	init [path]     Create a new blog folder.It default current folder if no path option.
`

	fmt.Printf(helpInfo, strings.TrimPrefix(os.Args[0], "./"))
}

func showVersion() {
	fmt.Println("version 1.0")
}
