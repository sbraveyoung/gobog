package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SmartBrave/gobog/cmd"
	"github.com/spf13/viper"
)

const (
	FILE = iota
	DIR
	CFG = "./conf/cfg.toml"
)

type LogConfig struct {
	FormatStr   string `mapstructure:"formatStr"`
	RunMode     string `mapstructure:"runMode"`
	LogFileName string `mapstructure:"logFileName"`
	LogMaxLines int    `mapstructure:"logMaxLines"`
	LogMaxSize  int    `mapstructure:"logMaxSize"`
	LogDaily    bool   `mapstructure:"logDaily"`
	LogRotate   bool   `mapstructure:"logRotate"`
	LogLevel    int    `mapstructure:"logLevel"`
}

type HttpConfig struct {
	Addr  []string `mapstructure:"addr"`
	Addrs []string `mapstructure:"addrs"`
	Cert  string   `mapstructure:"cert"`
	Key   string   `mapstructure:"key"`
}

type BlogConfig struct {
	Title       string `mapstructure:"title"`
	Subtitle    string `mapstructure:"subtitle"`
	Description string `mapstructure:"description"`
	Author      string `mapstructure:"author"`
	Theme       string `mapstructure:"theme"`
	Domain      string `mapstructure:"domain"`
	Articles    ArticlesType
	PostPath    string `mapstructure:"postpath"`
	AboutPath   string `mapstructure:"aboutpath"`
	ImagePath   string `mapstructure:"imagepath"`
	CssPath     string `mapstructure:"csspath"`
	JsPath      string `mapstructure:"jspath"`
	VideoPath   string `mapstructure:"videopath"`
	AudioPath   string `mapstructure:"audiopath"`
}

type Config struct {
	ConfigFile string     `mapstructure:"configFile"`
	Blog       BlogConfig `mapstructure:"blog"`
	Http       HttpConfig `mapsturcture:"http"` //console the time and other args of server
	Log        LogConfig  `mapstructure:"log"`
	once       sync.Once
}

type ArticleType struct {
	Id          string
	FilePath    string
	Url         string
	Title       string
	Description string
	Author      string
	Time        string //time of write this article
	ModifyTime  int64  //time of create article file. also publish
	Content     []byte
	Parse       string
	Comments    []comment
	Tag         int
	SubArticle  ArticlesType
}

type ArticlesType []*ArticleType

func (a ArticlesType) Len() int {
	return len(a)
}

func (a ArticlesType) Less(i, j int) bool {
	if strings.Contains(strings.ToLower(a[i].Title), "about") {
		return false
	}
	if strings.Contains(strings.ToLower(a[j].Title), "about") {
		return true
	}
	if a[i].SubArticle == nil && a[j].SubArticle != nil {
		return false
	}
	if a[i].SubArticle != nil && a[j].SubArticle == nil {
		return true
	}
	//return a[i].ModifyTime > a[j].ModifyTime
	ti, err := time.Parse("2006-01-02 15:04:05", a[i].Time)
	if err != nil {
		return false
	}
	tj, err := time.Parse("2006-01-02 15:04:05", a[j].Time)
	if err != nil {
		return true
	}
	return ti.Unix() > tj.Unix()
}

func (a ArticlesType) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a *ArticleType) IsSame(id string) bool {
	return strings.Compare(a.Id, id) == 0
}

type comment struct {
	Id               int
	ArticleId        int
	Publisher        User //must login
	ReponseCommentId int
	Time             string
}

type User struct {
	Id           int
	Name         string
	Phone        string
	Email        string
	RegisterTime string
}

func NewUser(name, phone, email string) *User {
	return &User{
		Name:  name,
		Phone: phone,
		Email: email,
	}
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
		//Mysql:   MysqlConfig{},
		//Log:     LogConfig{},
		//Article: ArticleConfig{},
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
