package config

import (
	"flag"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"
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
	ConfigFile string
	BlogPath   string        `mapstructure:"blogPath"`
	Mysql      MysqlConfig   `mapstructure:"mysql"`
	Log        LogConfig     `mapstructure:"log"`
	Article    ArticleConfig `mapstructure:"article"`
	Http       HttpConfig    `mapsturcture:"http"`
	once       sync.Once
}

func (c *Config) parseFlag() {
	flag.StringVar(&c.ConfigFile, "c", "./conf/cfg.toml", "config file")
	help := flag.Bool("h", false, "help")

	flag.Parse()
	if *help {
		showHelp()
		os.Exit(0)
	}

	if _, err := os.Stat(c.ConfigFile); err != nil {
		fmt.Printf("check config file: %s happend error: %v\n", c.ConfigFile, err)
		os.Exit(1)
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
	fmt.Println("Usage:")
	fmt.Println("	//TODO")
}
