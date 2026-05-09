package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	C Config

	// ExportDir, when non-empty, switches gobog into static export mode:
	// the binary renders the entire site into ExportDir and exits without
	// starting the HTTP servers. Set via the -export flag.
	ExportDir string

	// Testing is true when running under `go test`. Production code paths in
	// dependent packages can use this to skip filesystem-dependent init.
	Testing bool
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
	Addr        string
	Addrs       string
	Cert        string
	Key         string
	RedirectTLS bool `toml:"redirect_tls"`
}

type BlogConfig struct {
	Domain      string
	Title       string
	Subtitle    string
	Description string
	Author      string
	Theme       string
	Source      string
	CNAME       string
	// Layout controls how Source is interpreted. "auto" (default) picks
	// legacy when <source>/post/ exists, vault otherwise. "vault" forces
	// recursive scan from <source> root. "legacy" forces the original
	// gobog convention (<source>/post + <source>/about). Setting this
	// avoids the failure mode where a user happens to have a folder
	// named "post" in their vault and accidentally trips legacy.
	Layout string `toml:"layout"`
	// ExcludeDirs is a list of top-level directory names under <source>
	// that the vault scanner should ignore (e.g. "templates", "drafts",
	// "_archive"). Comparison is case-insensitive on basename.
	ExcludeDirs    []string `toml:"exclude_dirs"`
	IncludeDrafts  bool     `toml:"include_drafts"`
	IncludeHidden  bool     `toml:"include_hidden"`
	GoogleAnalytic string   `toml:"google_analytic"`
}

// AuthConfig gates private articles and snippet creation. Username is the
// HTTP Basic auth user; PasswordHash is a hex sha256 of the password.
// When either is empty, auth is disabled and any /private content / snippet
// POST returns 503.
type AuthConfig struct {
	Username     string `toml:"username"`
	PasswordHash string `toml:"password_hash"`
	// Realm shown in the WWW-Authenticate header. Optional.
	Realm string `toml:"realm"`
}

// DataConfig points at a directory where the server keeps mutable state
// (view counts, snippets). Defaults to "./gobog-data" (working dir-relative)
// so the user's vault stays untouched.
type DataConfig struct {
	Dir string `toml:"dir"`
}

// ImageConfig drives the on-the-fly watermark applied to JPEG/PNG responses
// from /image/. Watermark text is the only required field; when blank, the
// server hands images through untouched.
type ImageConfig struct {
	WatermarkText     string `toml:"watermark_text"`
	WatermarkPosition string `toml:"watermark_position"` // top-left, top-right, bottom-left, bottom-right (default), center
}

// BackupConfig periodically tars + gzips [blog].source into <dir>. Off by
// default; turn on by setting Enabled=true and a sensible Interval.
type BackupConfig struct {
	Enabled  bool   `toml:"enabled"`
	Dir      string `toml:"dir"`      // where to write archives; defaults to <data>/backups
	Interval string `toml:"interval"` // Go duration, e.g. "1h", "30m", "24h". Default 1h.
	Keep     int    `toml:"keep"`     // rotation: keep newest N archives. Default 7.
}

type Config struct {
	Blog   BlogConfig
	Http   HttpConfig
	Log    LogConfig
	Auth   AuthConfig
	Data   DataConfig
	Image  ImageConfig
	Backup BackupConfig
}

func init() {
	Testing = strings.HasSuffix(os.Args[0], ".test")
	if Testing {
		// Under `go test`: don't touch flag.CommandLine (the testing
		// framework will parse its own flags later) and don't read a
		// config file. Tests should populate C directly if they need it.
		return
	}

	configPath := flag.String("config", "./conf/config.toml", "config path")
	exportDir := flag.String("export", "", "if set, render the site into this directory and exit")
	flag.Parse()
	ExportDir = strings.TrimSpace(*exportDir)

	data, err := ioutil.ReadFile(*configPath)
	if err != nil {
		fmt.Println("open config file:", err)
		os.Exit(1)
	}
	if _, err := toml.Decode(string(data), &C); err != nil {
		fmt.Println("decode config file:", err)
		os.Exit(1)
	}
	fmt.Printf("config:%+v export=%q\n", C, ExportDir)
}
