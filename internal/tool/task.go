package tool

// TaskInput 是 task 工具的输入参数。
type TaskInput struct {
	SubagentName string `json:"subagent_name"`
	Prompt       string `json:"prompt"`
}

// TaskResult 是 task 工具的结果。
type TaskResult struct {
	SubagentName string `json:"subagent_name"`
	Output       string `json:"output"`
}

// TaskFunc 是 task 工具的执行函数类型。
// 由 agent 层注入实际实现（避免 tool → agent 循环依赖）。
type TaskFunc func(input TaskInput) (*TaskResult, error)

// TaskHandler 是注入的 task 工具处理器。
var TaskHandler TaskFunc

// Task 调用子代理执行任务。
// 实际实现由 agent 层注入到 TaskHandler。
func Task(input TaskInput) (*TaskResult, error) {
	if TaskHandler == nil {
		return nil, ErrTaskHandlerNotSet
	}
	return TaskHandler(input)
}
