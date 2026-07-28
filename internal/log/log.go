package log

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// Level 日志级别
type Level int

const (
	LevelTrace Level = iota // 逐帧详细输出
	LevelDebug              // 诊断信息
	LevelInfo               // 启动、里程碑
	LevelWarn               // 可恢复问题
	LevelError              // 错误
)

var levelNames = [...]string{"TRACE", "DEBUG", "INFO", "WARN", "ERROR"}

var currentLevel atomic.Int32

func init() {
	currentLevel.Store(int32(LevelDebug))
}

// SetLevel 设置全局日志级别
func SetLevel(l Level) {
	currentLevel.Store(int32(l))
}

// ParseLevel 从字符串解析日志级别，无法识别时默认 DEBUG
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

// Logf 输出一条日志到 stderr，格式：
// 2006/01/02 15:04:05.000 [级别] [标签] 文件:行号 消息
func Logf(level Level, tag string, format string, args ...any) {
	if level < Level(currentLevel.Load()) {
		return
	}
	ts := time.Now().Format("2006/01/02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	caller := shortCaller()
	fmt.Fprintf(os.Stderr, "%s [%s] [%s] %s %s\n", ts, levelNames[level], tag, caller, msg)
}

// shortCaller 返回调用方的 短路径:行号（如 server/main.go:42）
func shortCaller() string {
	// skip=2: runtime.Caller → shortCaller → Logf → 实际调用方
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		return "??:0"
	}
	// 截取最后两级路径，兼容 / 和 \ 分隔符
	file = toSlash(file)
	i := strings.LastIndexByte(file, '/')
	if i >= 0 {
		j := strings.LastIndexByte(file[:i], '/')
		if j >= 0 {
			file = file[j+1:]
		}
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func toSlash(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}
