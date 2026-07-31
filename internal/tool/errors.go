package tool

import "errors"

// ErrTaskHandlerNotSet 表示 task 工具处理器未设置（agent 层未注入）。
var ErrTaskHandlerNotSet = errors.New("task handler not set, agent layer not initialized")
