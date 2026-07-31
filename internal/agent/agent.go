// Package agent 实现 Agent 运行时，包括主代理和子代理。
// 每个 agent 独立 goroutine + 独立消息 channel，实现超高并发。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"ago/internal/base"
	"ago/internal/config"
	"ago/internal/provider"
	"ago/internal/tool"
	"ago/internal/transport"
)

// AgentRuntime 是 agent 运行时，每个实例独立 goroutine。
// 参考 packages/core/src/agent.ts + packages/schema/src/agent.ts。
type AgentRuntime struct {
	AgentID  string
	Model    string            // 完整模型 ID（provider/model）
	System   string            // 系统提示词
	Mode     string            // primary/subagent/all
	Tools    map[string]bool   // 工具启用映射（nil=继承全部；非空按 map 控制，未列出=禁用）
	Provider provider.Provider // LLM provider
	ModelID  string            // 不含 provider 前缀的模型 ID
	Hub      *transport.Hub    // 通信中枢
	factory  *provider.Factory // provider 工厂
	cfg      *config.Config
	msgChan  <-chan transport.Message
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  atomic.Bool
	steps    int // 最大工具调用轮数（0 用默认值）
	depth    int // 当前子代理嵌套深度（主代理=0，每调一层+1）
	taskBudget int  // 调用子代理的次数预算（0=不限制）
	taskUsed   int  // 已调用子代理次数
}

// 默认步数与深度常量。
const (
	// defaultAgentSteps 默认最大工具调用轮数（原版 opencode 未显式配置时的回退值）。
	defaultAgentSteps = 10
	// defaultSubagentDepth 默认子代理嵌套深度上限（对齐原版默认值 3）。
	// 原版配置未显式设置 subagent_depth 时用此值兜底。
	defaultSubagentDepth = 3
)

// AgentOption 是 agent 创建选项。
type AgentOption func(*AgentRuntime)

// WithSystem 设置系统提示词。
func WithSystem(system string) AgentOption {
	return func(a *AgentRuntime) { a.System = system }
}

// WithMode 设置 agent 模式。
func WithMode(mode string) AgentOption {
	return func(a *AgentRuntime) { a.Mode = mode }
}

// WithTools 设置工具启用映射。
func WithTools(tools map[string]bool) AgentOption {
	return func(a *AgentRuntime) { a.Tools = tools }
}

// WithSteps 设置最大工具调用轮数。
func WithSteps(steps int) AgentOption {
	return func(a *AgentRuntime) { a.steps = steps }
}

// WithDepth 设置子代理嵌套深度。
func WithDepth(depth int) AgentOption {
	return func(a *AgentRuntime) { a.depth = depth }
}

// WithTaskBudget 设置调用子代理的次数预算。
func WithTaskBudget(budget int) AgentOption {
	return func(a *AgentRuntime) { a.taskBudget = budget }
}

// New 创建 agent 运行时。
func New(agentID, model string, hub *transport.Hub, factory *provider.Factory, cfg *config.Config, opts ...AgentOption) (*AgentRuntime, error) {
	// 解析 model ID 获取 provider
	p, modelIDAfterProvider, err := factory.GetProvider(model)
	if err != nil {
		return nil, fmt.Errorf("agent %s init provider: %w", agentID, err)
	}

	a := &AgentRuntime{
		AgentID:  agentID,
		Model:    model,
		ModelID:  modelIDAfterProvider,
		Hub:      hub,
		factory:  factory,
		cfg:      cfg,
		Provider: p,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Start 启动 agent goroutine，开始监听消息。
func (a *AgentRuntime) Start(ctx context.Context) error {
	if a.started.Load() {
		return fmt.Errorf("agent %s already started", a.AgentID)
	}

	// 注册消息 channel
	a.msgChan = a.Hub.Register(a.AgentID, 64)

	// 创建可取消的 context
	runCtx, cancel := context.WithCancel(ctx)
	a.cancel = cancel

	a.wg.Add(1)
	a.started.Store(true)
	go a.runLoop(runCtx)
	base.Debugf("agent %s started (model=%s)", a.AgentID, a.Model)
	return nil
}

// Stop 停止 agent，关闭 goroutine。
func (a *AgentRuntime) Stop() {
	if !a.started.Load() {
		return
	}
	a.cancel()
	a.wg.Wait()
	a.Hub.Unregister(a.AgentID)
	a.started.Store(false)
	base.Debugf("agent %s stopped", a.AgentID)
}

// runLoop 是 agent 主消息循环。
func (a *AgentRuntime) runLoop(ctx context.Context) {
	defer a.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-a.msgChan:
			if !ok {
				return
			}
			a.handleMessage(ctx, msg)
		}
	}
}

// handleMessage 处理收到的消息。
func (a *AgentRuntime) handleMessage(ctx context.Context, msg transport.Message) {
	// 确定回复目标：优先 ReplyTo，其次 SrcAgentID
	replyDst := msg.ReplyTo
	if replyDst == "" {
		replyDst = msg.SrcAgentID
	}

	switch msg.Type {
	case transport.MsgTypeUser:
		// 用户消息：调用 LLM 处理
		userText, _ := msg.Payload.(string)
		result, err := a.processWithLLM(ctx, userText)
		if err != nil {
			base.Errorf("agent %s process failed: %v", a.AgentID, err)
			// 回复错误
			a.Hub.Send(ctx, transport.Message{
				SrcAgentID: a.AgentID,
				DstAgentID: replyDst,
				Type:       transport.MsgTypeError,
				Payload:    err.Error(),
			})
			return
		}
		// 回复处理结果
		a.Hub.Send(ctx, transport.Message{
			SrcAgentID: a.AgentID,
			DstAgentID: replyDst,
			Type:       transport.MsgTypeAssistant,
			Payload:    result,
		})
	case transport.MsgTypeStop:
		return
	}
}

// processWithLLM 调用 LLM 处理用户输入，支持工具调用循环。
// 返回最终文本输出。
func (a *AgentRuntime) processWithLLM(ctx context.Context, userInput string) (string, error) {
	// 构造消息历史
	messages := []provider.Message{}
	if a.System != "" {
		messages = append(messages, provider.Message{Role: "system", Content: a.System})
	}
	messages = append(messages, provider.Message{Role: "user", Content: userInput})

	// 工具定义
	tools := a.getToolDefinitions()

	// 工具调用轮数上限：agent 配置优先，否则用默认值
	maxRounds := a.steps
	if maxRounds <= 0 {
		maxRounds = defaultAgentSteps
	}
	for round := 0; round < maxRounds; round++ {
		// 调用 LLM
		streamCh, err := a.Provider.StreamComplete(ctx, provider.CompleteRequest{
			Model:    a.ModelID,
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return "", fmt.Errorf("llm stream: %w", err)
		}

		// 收集流式响应
		var assistantText string
		var toolCalls []provider.ToolCall
		var finishReason string
		for event := range streamCh {
			switch event.Type {
			case "text/delta":
				assistantText += event.Text
			case "tool_call":
				if event.ToolCall != nil {
					toolCalls = append(toolCalls, *event.ToolCall)
				}
			case "finish":
				finishReason = event.FinishReason
			case "error":
				return "", fmt.Errorf("stream error: %w", event.Err)
			}
		}

		// 没有工具调用，返回最终文本
		if len(toolCalls) == 0 || finishReason == "stop" {
			return assistantText, nil
		}

		// 有工具调用：执行工具并继续循环
		// 添加 assistant 消息（含 tool_calls）
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   assistantText,
			ToolCalls: toolCalls,
		})

		// 执行每个工具调用
		for _, tc := range toolCalls {
			result := a.executeTool(ctx, tc)
			// 添加 tool 结果消息
			messages = append(messages, provider.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}

	return "", fmt.Errorf("max tool call rounds (%d) exceeded", maxRounds)
}

// getToolDefinitions 返回当前 agent 可用的工具定义。
func (a *AgentRuntime) getToolDefinitions() []provider.ToolDefinition {
	allTools := []provider.ToolDefinition{
		{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "read",
				Description: "读取文件内容或列出目录",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "文件或目录路径"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "write",
				Description: "写文件，自动创建父目录",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "文件路径"},
						"content": map[string]any{"type": "string", "description": "文件内容"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "edit",
				Description: "通过字符串替换编辑文件",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":        map[string]any{"type": "string", "description": "文件路径"},
						"old_string":  map[string]any{"type": "string", "description": "要替换的文本"},
						"new_string":  map[string]any{"type": "string", "description": "替换后的文本"},
						"replace_all": map[string]any{"type": "boolean", "description": "是否替换所有出现"},
					},
					"required": []string{"path", "old_string", "new_string"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "task",
				Description: "调用子代理执行任务",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"subagent_name": map[string]any{"type": "string", "description": "子代理名称"},
						"prompt":        map[string]any{"type": "string", "description": "任务描述"},
					},
					"required": []string{"subagent_name", "prompt"},
				},
			},
		},
		{
			Type: "function",
			Function: struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				Parameters  map[string]any `json:"parameters"`
			}{
				Name:        "bash",
				Description: "执行 shell 命令并返回 stdout/stderr/exit_code（Unix 用 sh，Windows 用 cmd）",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command":     map[string]any{"type": "string", "description": "要执行的 shell 命令"},
						"workdir":     map[string]any{"type": "string", "description": "工作目录（可选，默认当前目录）"},
						"timeout_sec": map[string]any{"type": "integer", "description": "超时秒数（可选，默认 60）"},
					},
					"required": []string{"command"},
				},
			},
		},
	}

	// 应用工具启用映射：nil/空=继承全部；非空时按 map 控制，未列出=禁用
	if len(a.Tools) > 0 {
		filtered := make([]provider.ToolDefinition, 0, len(allTools))
		for _, t := range allTools {
			if a.Tools[t.Function.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered
	}
	return allTools
}

// executeTool 执行工具调用，返回结果文本。
func (a *AgentRuntime) executeTool(ctx context.Context, tc provider.ToolCall) string {
	name := tc.Function.Name
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return fmt.Sprintf("error: parse arguments: %v", err)
	}

	switch name {
	case "read":
		input := tool.ReadInput{Path: getStringArg(args, "path")}
		result, err := tool.Read(input)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		if result.IsDir {
			return fmt.Sprintf("directory entries: %v", result.Entries)
		}
		if result.ImageBase64 != "" {
			return fmt.Sprintf("image file (%s), base64 length: %d", result.MIME, len(result.ImageBase64))
		}
		return result.Content
	case "write":
		input := tool.WriteInput{
			Path:    getStringArg(args, "path"),
			Content: getStringArg(args, "content"),
		}
		result, err := tool.Write(input)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return fmt.Sprintf("wrote %d bytes to %s (created=%v)", result.Bytes, result.Path, result.Created)
	case "edit":
		input := tool.EditInput{
			Path:       getStringArg(args, "path"),
			OldString:  getStringArg(args, "old_string"),
			NewString:  getStringArg(args, "new_string"),
			ReplaceAll: getBoolArg(args, "replace_all"),
		}
		result, err := tool.Edit(input)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return fmt.Sprintf("edited %s, %d replacements", result.Path, result.Replacements)
	case "task":
		// task_budget 检查：单个 agent 调用子代理的次数预算
		if a.taskBudget > 0 && a.taskUsed >= a.taskBudget {
			return fmt.Sprintf("error: task budget %d exhausted", a.taskBudget)
		}
		input := tool.TaskInput{
			SubagentName: getStringArg(args, "subagent_name"),
			Prompt:       getStringArg(args, "prompt"),
			Depth:        a.depth, // 注入调用者深度，供 SubagentManager 累积
		}
		// ctx 透传：子 agent 跟随父 agent 生命周期，父 agent cancel 时子 agent 自动中止
		result, err := tool.Task(ctx, input)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		a.taskUsed++
		return result.Output
	case "bash":
		input := tool.BashInput{
			Command:    getStringArg(args, "command"),
			WorkDir:    getStringArg(args, "workdir"),
			TimeoutSec: getIntArg(args, "timeout_sec"),
		}
		result, err := tool.Bash(input)
		if err != nil {
			return fmt.Sprintf("error: %v", err)
		}
		return fmt.Sprintf("exit_code=%d\nstdout:\n%s\nstderr:\n%s", result.ExitCode, result.Stdout, result.Stderr)
	default:
		return fmt.Sprintf("error: unknown tool %s", name)
	}
}

// getStringArg 从参数 map 获取字符串值。
func getStringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// getBoolArg 从参数 map 获取布尔值。
func getBoolArg(args map[string]any, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

// getIntArg 从参数 map 获取整数值。
func getIntArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
