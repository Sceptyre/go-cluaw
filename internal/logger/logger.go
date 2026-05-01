package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
)

type Logger struct {
	info  *log.Logger
	warn  *log.Logger
	err   *log.Logger
	debug *log.Logger
}

func New() *Logger {
	return &Logger{
		info:  log.New(os.Stdout, "[INFO] ", log.Ldate|log.Ltime|log.Lshortfile),
		warn:  log.New(os.Stdout, "[WARN] ", log.Ldate|log.Ltime|log.Lshortfile),
		err:   log.New(os.Stderr, "[ERROR] ", log.Ldate|log.Ltime|log.Lshortfile),
		debug: log.New(os.Stdout, "[DEBUG] ", log.Ldate|log.Ltime|log.Lshortfile),
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.info.Output(2, formatMsg(format, v))
}

func (l *Logger) Warn(format string, v ...interface{}) {
	l.warn.Output(2, formatMsg(format, v))
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.err.Output(2, formatMsg(format, v))
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.debug.Output(2, formatMsg(format, v))
}

func formatMsg(format string, v []interface{}) string {
	if len(v) == 0 {
		return format
	}
	result := format
	for _, val := range v {
		switch v := val.(type) {
		case string:
			result = strings.Replace(result, "%s", v, 1)
		case int:
			result = strings.Replace(result, "%d", fmt.Sprint(v), 1)
		case interface{}:
			result = strings.Replace(result, "%v", fmt.Sprint(v), 1)
		}
	}
	return result
}
