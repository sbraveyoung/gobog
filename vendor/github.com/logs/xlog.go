package logs

import (
	"encoding/json"
	"strings"
)

var xlog *XLogger

func LogInit(xRunnode string, xFilename string, xMaxlines int, xMaxsize int, xDaily bool, xRotate bool, xLevel int) {
	var config string
	xlog = NewXLogger(10000)

	f := FileLogWriter{
		Filename: xFilename,
		Maxlines: xMaxlines,
		Maxdays:  365,
		Maxsize:  xMaxsize,
		Daily:    xDaily,
		Rotate:   xRotate,
		Level:    xLevel,
	}
	j, err := json.Marshal(f)
	if err != nil {
		Error(err)
		config = "{\"filename\":\""
		config += xFilename
		config += "\"}"
	} else {
		config = string(j)
	}

	SetXLogger("file", config)
	if xRunnode == "debug" {
		SetXLogger("console", `{"level":8}`)

	}
}

// SetLogLevel sets the global log level used by the simple
// logger.
func SetLevel(l int) {
	xlog.SetLevel(l)
}

func SetExtra(extra string) {
	xlog.SetExtra(extra)
}

func SetLogFuncCall(b bool) {
	xlog.EnableFuncCallDepth(b)
	xlog.SetLogFuncCallDepth(3)
}

// SetXLogger sets a new logger.
func SetXLogger(adaptername string, config string) error {
	err := xlog.SetXLogger(adaptername, config)
	if err != nil {
		return err
	}
	return nil
}

// Error logs a message at error level.
func Error(v ...interface{}) {
	xlog.Error(generateFmtStr(len(v)), v...)
}

// compatibility alias for Warning()
func Warn(v ...interface{}) {
	xlog.Warn(generateFmtStr(len(v)), v...)
}

func Notice(v ...interface{}) {
	xlog.Notice(generateFmtStr(len(v)), v...)
}

// compatibility alias for Warning()
func Info(v ...interface{}) {
	xlog.Info(generateFmtStr(len(v)), v...)
}

// Debug logs a message at debug level.
func Debug(v ...interface{}) {
	xlog.Debug(generateFmtStr(len(v)), v...)
}

// Trace logs a message at trace level.
// compatibility alias for Warning()
func Trace(v ...interface{}) {
	xlog.Trace(generateFmtStr(len(v)), v...)
}

func generateFmtStr(n int) string {
	return strings.Repeat("%v ", n)
}
