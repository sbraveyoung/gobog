package log

import (
	"fmt"
	"os"

	"github.com/op/go-logging"
)

func Init(formatStr, logPath string) {
	var log = logging.MustGetLogger("gobog")
	var format = logging.MustStringFormatter(formatStr)
	var file *os.File

	if _, err := os.Stat(logPath); err == os.ErrNotExist {
		if file, err = os.Create(logPath); err != nil {
			fmt.Printf("os.Create(%s) fail. err: %v\n", logPath, err)
			os.Exit(1)
		}
	} else if err == nil {
		if file, err = os.Open(logPath); err != nil {
			fmt.Printf("os.Open(%s) fail. err: %v\n", logPath, err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("os.Stat(%s) fail. err: %v\n", logPath, err)
		os.Exit(1)
	}
	backend := logging.NewLogBackend(f)
	backendFormatter := logging.NewBackendFormatter(backend, formatStr)
}
