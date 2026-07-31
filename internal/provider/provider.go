// Package provider 实现 LLM Provider 子系统。
// 提供 Provider 接口与 OpenAI 兼容、Anthropic Messages、opencode（Zen 网关）三种实现。
package provider

import (
	"context"

	"ago/internal/config"
)

// Message 表示对话消息，对齐原版 LLMRequest 的核心结构。
type Message struct {
	Role       string     `json:"role"`                   // system/user/assistant/tool
	Content    string     `json:"content,omitempty"`      // 文本内容
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // assistant 消息的工具调用
	ToolCallID string     `json:"tool_call_id,omitempty"` // tool 消息关联的调用 ID
}

// ToolCall 表示 assistant 消息中的工具调用。
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // 固定 "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON 字符串
	} `json:"function"`
}

// ToolDefinition 工具定义，对齐原版 ToolDefinition。
type ToolDefinition struct {
	Type     string `json:"type"` // 固定 "function"
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"` // JSON Schema
	} `json:"function"`
}

// StreamEvent 是流式响应事件。
type StreamEvent struct {
	// Type 事件类型：text/delta（文本增量）、tool_call（工具调用）、finish（结束）、usage（用量）、error
	Type string `json:"type"`

	// Text 文本增量（type=text/delta 时）
	Text string `json:"text,omitempty"`

	// ToolCall 工具调用（type=tool_call 时）
	ToolCall *ToolCall `json:"tool_call,omitempty"`

	// FinishReason 结束原因（type=finish 时）：stop/length/tool_calls
	FinishReason string `json:"finish_reason,omitempty"`

	// Usage 用量（type=usage 时）
	Usage *Usage `json:"usage,omitempty"`

	// Err 错误（type=error 时）
	Err error `json:"-"`
}

// Usage 表示 token 用量。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CompleteRequest 是 LLM 调用请求。
type CompleteRequest struct {
	Model    string           // 模型 ID（不含 provider 前缀）
	Messages []Message        // 对话消息
	Tools    []ToolDefinition // 可用工具
}

// Provider 是 LLM Provider 接口，对齐原版抽象。
type Provider interface {
	// Init 初始化 provider（校验配置、建立连接池等）
	Init(ctx context.Context) error

	// StreamComplete 流式调用 LLM，通过 channel 返回事件。
	// channel 关闭表示流结束。
	StreamComplete(ctx context.Context, req CompleteRequest) (<-chan StreamEvent, error)

	// ListModels 返回可用模型列表（provider 内部模型 ID -> 显示名）
	ListModels(ctx context.Context) (map[string]string, error)
}

// Factory 是 provider 工厂，根据配置创建 provider 实例。
type Factory struct {
	cfg *config.Config
}

// NewFactory 创建工厂。
func NewFactory(cfg *config.Config) *Factory {
	return &Factory{cfg: cfg}
}

// GetProvider 根据 "provider/model" 格式的模型 ID 返回对应 provider 实例。
func (f *Factory) GetProvider(modelID string) (Provider, string, error) {
	providerID, modelIDAfterProvider := config.ParseModelID(modelID)
	if providerID == "" {
		return nil, "", &UnknownProviderError{ModelID: modelID}
	}
	p, err := f.createProvider(providerID)
	if err != nil {
		return nil, "", err
	}
	return p, modelIDAfterProvider, nil
}

// createProvider 根据 provider ID 创建 provider 实例。
func (f *Factory) createProvider(providerID string) (Provider, error) {
	switch providerID {
	case "opencode":
		return NewOpencodeProvider(f.cfg), nil
	case "openai":
		return NewOpenAICompatibleProvider(f.cfg, providerID, "https://api.openai.com/v1"), nil
	case "anthropic":
		return NewAnthropicProvider(f.cfg), nil
	default:
		// 从配置的自定义 provider 查找
		if pcfg, ok := f.cfg.Provider[providerID]; ok {
			npm := pcfg.Npm
			if pcfg.API != nil && pcfg.API.Npm != "" {
				npm = pcfg.API.Npm
			}
			baseURL := ""
			if pcfg.Options != nil {
				if u, ok := pcfg.Options["baseURL"].(string); ok {
					baseURL = u
				}
			}
			switch npm {
			case "@ai-sdk/anthropic":
				return NewAnthropicProvider(f.cfg), nil
			default:
				// 默认走 OpenAI 兼容协议（含 @ai-sdk/openai-compatible/@ai-sdk/openai）
				return NewOpenAICompatibleProvider(f.cfg, providerID, baseURL), nil
			}
		}
		return nil, &UnknownProviderError{ProviderID: providerID}
	}
}

// UnknownProviderError 未知 provider 错误。
type UnknownProviderError struct {
	ProviderID string
	ModelID    string
}

func (e *UnknownProviderError) Error() string {
	if e.ModelID != "" {
		return "unknown provider for model: " + e.ModelID
	}
	return "unknown provider: " + e.ProviderID
}
