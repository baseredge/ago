// Package base 提供基础层能力：日志和统一错误类型。
package base

import (
	"log"
	"os"
)

// Logger 是全局日志器，封装标准 log 包。
var Logger = log.New(os.Stderr, "[opencode] ", log.LstdFlags|log.Lmsgprefix)

// Debug 控制调试日志开关，默认关闭。
var Debug = false

// Logf 输出普通日志。
func Logf(format string, args ...any) {
	Logger.Printf(format, args...)
}

// Debugf 输出调试日志，仅在 Debug 开启时生效。
func Debugf(format string, args ...any) {
	if Debug {
		Logger.Printf("[DEBUG] "+format, args...)
	}
}

// Errorf 输出错误日志。
func Errorf(format string, args ...any) {
	Logger.Printf("[ERROR] "+format, args...)
}
