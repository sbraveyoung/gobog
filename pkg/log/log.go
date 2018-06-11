package log

import (
	"io"
	"log"
)

var (
	Debug func(str string)
	Info  func(str string)
	Warn  func(str string)
	Error func(str string)

	debug *log.Logger
	info  *log.Logger
	warn  *log.Logger
	erro  *log.Logger
)

func Init(w io.Writer) {
	debug = log.New(w, "Debug: ", log.Ldate|log.Ltime|log.Lshortfile)
	info = log.New(w, "Info: ", log.Ldate|log.Ltime|log.Lshortfile)
	warn = log.New(w, "Warn: ", log.Ldate|log.Ltime|log.Lshortfile)
	erro = log.New(w, "Error: ", log.Ldate|log.Ltime|log.Lshortfile)
	Debug = func(str string) {
		debug.Output(2, str)
	}
	Info = func(str string) {
		info.Output(2, str)
	}
	Warn = func(str string) {
		warn.Output(2, str)
	}
	Error = func(str string) {
		erro.Output(2, str)
	}
}
