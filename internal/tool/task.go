package tool

import "context"

// TaskInput 是 task 工具的输入参数。
type TaskInput struct {
	SubagentName string `json:"subagent_name"`
	Prompt       string `json:"prompt"`
	// Depth 调用者（父 agent）的嵌套深度，由 agent 层注入，LLM 不感知。
	// 主代理 depth=0，每调一层子代理 depth+1，用于限制递归层数。
	Depth int `json:"-"`
}

// TaskResult 是 task 工具的结果。
type TaskResult struct {
	SubagentName string `json:"subagent_name"`
	Output       string `json:"output"`
}

// TaskFunc 是 task 工具的执行函数类型。
// 由 agent 层注入实际实现（避免 tool → agent 循环依赖）。
// ctx 来自调用者（父 agent）的 context，子 agent 跟随父 agent 生命周期，
// 父 agent 被 cancel 时子 agent 自动中止（对齐原版 opencode 的 context cancel 机制）。
type TaskFunc func(ctx context.Context, input TaskInput) (*TaskResult, error)

// TaskHandler 是注入的 task 工具处理器。
var TaskHandler TaskFunc

// Task 调用子代理执行任务。
// 实际实现由 agent 层注入到 TaskHandler。
func Task(ctx context.Context, input TaskInput) (*TaskResult, error) {
	if TaskHandler == nil {
		return nil, ErrTaskHandlerNotSet
	}
	return TaskHandler(ctx, input)
}
