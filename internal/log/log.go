package log

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Level int

const (
	LevelTrace Level = iota // per-frame verbose
	LevelDebug              // diagnostic
	LevelInfo               // startup, milestones
	LevelWarn               // recoverable issues
	LevelError              // failures
)

var levelNames = [...]string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"}

var currentLevel = LevelDebug

func SetLevel(l Level) {
	currentLevel = l
}

func ParseLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "TRACE":
		return LevelTrace
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelDebug
	}
}

func Logf(level Level, tag string, format string, args ...any) {
	if level < currentLevel {
		return
	}
	ts := time.Now().Format("2006/01/02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%s [%s] [%s] %s\n", ts, levelNames[level], tag, msg)
}
