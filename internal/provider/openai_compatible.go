package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ago/internal/config"
)

// OpenAICompatibleProvider 是 OpenAI 兼容协议实现，覆盖 OpenAI/OpenRouter/DeepSeek/GLM/Kimi/MiniMax/Grok 等。
// 参考 packages/llm/src/protocols/openai-compatible-chat.ts + openai-chat.ts。
type OpenAICompatibleProvider struct {
	cfg        *config.Config
	providerID string
	baseURL    string
	// apiKeyFn 返回 apiKey 的函数，可被调用方注入覆盖。
	// 默认实现从 cfg.Provider[providerID].Options.apiKey 或 Env 读取。
	// OpencodeProvider 通过注入此函数实现双模式鉴权（避免 Go 方法覆盖不生效的问题）。
	apiKeyFn func() string
}

// NewOpenAICompatibleProvider 创建 OpenAI 兼容 provider。
// baseURL 为空时默认 https://api.openai.com/v1。
func NewOpenAICompatibleProvider(cfg *config.Config, providerID, baseURL string) *OpenAICompatibleProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	p := &OpenAICompatibleProvider{cfg: cfg, providerID: providerID, baseURL: baseURL}
	p.apiKeyFn = p.defaultGetAPIKey
	return p
}

// Init 校验配置。
func (p *OpenAICompatibleProvider) Init(ctx context.Context) error {
	return nil
}

// ListModels 返回配置中声明的模型列表。
func (p *OpenAICompatibleProvider) ListModels(ctx context.Context) (map[string]string, error) {
	return listModelsFromConfig(p.cfg, p.providerID), nil
}

// defaultGetAPIKey 从配置或环境变量获取 apiKey（默认实现）。
func (p *OpenAICompatibleProvider) defaultGetAPIKey() string {
	return getAPIKeyFromConfig(p.cfg, p.providerID)
}

// StreamComplete 调用 /chat/completions 流式接口。
func (p *OpenAICompatibleProvider) StreamComplete(ctx context.Context, req CompleteRequest) (<-chan StreamEvent, error) {
	apiKey := p.apiKeyFn()

	// 构造请求体
	body := p.buildRequestBody(req)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	// 非 2xx 错误处理
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, wrapHTTPError(resp, p.providerID)
	}

	// 流式解析
	ch := make(chan StreamEvent, 32)
	go p.parseSSEStream(resp.Body, ch)
	return ch, nil
}

// buildRequestBody 构造 OpenAI chat/completions 请求体。
func (p *OpenAICompatibleProvider) buildRequestBody(req CompleteRequest) map[string]any {
	body := map[string]any{
		"model":  req.Model,
		"stream": true,
	}
	// 消息
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			// 防御性补全：确保 Type="function"，避免某些 provider 拒绝空 type
			calls := make([]ToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				if tc.Type == "" {
					tc.Type = "function"
				}
				calls[i] = tc
			}
			msg["tool_calls"] = calls
		}
		msgs = append(msgs, msg)
	}
	body["messages"] = msgs
	// 工具
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Function.Name,
					"description": t.Function.Description,
					"parameters":  t.Function.Parameters,
				},
			})
		}
		body["tools"] = tools
	}
	return body
}

// parseSSEStream 解析 SSE 流式响应。
// 格式：每行 "data: <json>\n\n"，最后 "data: [DONE]"。
//
// OpenAI 流式 tool_calls 协议：同一 tool_call 按 index 分多 chunk 传：
//   - 首 chunk: {index, id, type:"function", function:{name, arguments:""}}
//   - 后续 chunk: {index, function:{arguments:"增量片段"}}
//
// 必须按 index 累积，流结束时统一发出完整 ToolCall，否则会产出 Type="" 的碎片。
func (p *OpenAICompatibleProvider) parseSSEStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 最大 1MB 行

	// 按 index 累积 tool_call 增量
	type toolBuilder struct {
		tc      ToolCall
		hasName bool
	}
	builders := map[int]*toolBuilder{}
	var builderIndices []int // 保持 index 出现顺序

	flushToolCalls := func() {
		for _, idx := range builderIndices {
			b := builders[idx]
			// 防御性补全：Type 空则填 "function"（OpenAI 标准）
			if b.tc.Type == "" {
				b.tc.Type = "function"
			}
			// 只发有 name 的有效 tool_call（过滤无意义碎片）
			if b.hasName {
				tc := b.tc
				ch <- StreamEvent{Type: "tool_call", ToolCall: &tc}
			}
		}
		builders = map[int]*toolBuilder{}
		builderIndices = nil
	}

	finished := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			if !finished {
				flushToolCalls()
				ch <- StreamEvent{Type: "finish", FinishReason: "stop"}
			}
			return
		}
		var chunk openaiChatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- StreamEvent{Type: "error", Err: fmt.Errorf("parse chunk: %w", err)}
			return
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				ch <- StreamEvent{Type: "text/delta", Text: choice.Delta.Content}
			}
			// 累积 tool_call 增量（按 index 合并）
			for _, stc := range choice.Delta.ToolCalls {
				b, ok := builders[stc.Index]
				if !ok {
					b = &toolBuilder{}
					builders[stc.Index] = b
					builderIndices = append(builderIndices, stc.Index)
				}
				if stc.ID != "" {
					b.tc.ID = stc.ID
				}
				if stc.Type != "" {
					b.tc.Type = stc.Type
				}
				if stc.Function.Name != "" {
					b.tc.Function.Name = stc.Function.Name
					b.hasName = true
				}
				if stc.Function.Arguments != "" {
					b.tc.Function.Arguments += stc.Function.Arguments
				}
			}
			if choice.FinishReason != "" && !finished {
				flushToolCalls()
				ch <- StreamEvent{Type: "finish", FinishReason: choice.FinishReason}
				finished = true
			}
		}
		if chunk.Usage != nil {
			ch <- StreamEvent{Type: "usage", Usage: &Usage{
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			}}
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Err: fmt.Errorf("read stream: %w", err)}
		return
	}
	// 流未显式 finish 时兜底
	if !finished {
		flushToolCalls()
		ch <- StreamEvent{Type: "finish", FinishReason: "stop"}
	}
}

// openaiStreamToolCall 是 OpenAI 流式响应中的 tool_call 增量结构（带 index）。
type openaiStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// openaiChatChunk 是 OpenAI chat/completions 流式 chunk 结构。
type openaiChatChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string                 `json:"role,omitempty"`
			Content   string                 `json:"content,omitempty"`
			ToolCalls []openaiStreamToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}
