package main

import (
	"fmt"
	"os"

	"github.com/SmartBrave/gobog/src/config"
	"github.com/SmartBrave/gobog/src/server"
)

func main() {
	if config.ExportDir != "" {
		if err := server.Export(config.ExportDir); err != nil {
			fmt.Fprintln(os.Stderr, "export:", err)
			os.Exit(1)
		}
		return
	}
	if err := server.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "serve:", err)
		os.Exit(1)
	}
}
