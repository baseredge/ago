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

// anthropicBaseURL 是 Anthropic API 默认 baseURL。
const anthropicBaseURL = "https://api.anthropic.com"

// anthropicVersion 是 Anthropic API 版本。
const anthropicVersion = "2023-06-01"

// AnthropicProvider 是 Anthropic Messages 协议实现。
// 参考 packages/llm/src/protocols/anthropic-messages.ts。
type AnthropicProvider struct {
	cfg     *config.Config
	baseURL string
}

// NewAnthropicProvider 创建 Anthropic provider。
func NewAnthropicProvider(cfg *config.Config) *AnthropicProvider {
	baseURL := anthropicBaseURL
	if pcfg, ok := cfg.Provider["anthropic"]; ok && pcfg.Options != nil {
		if u, ok := pcfg.Options["baseURL"].(string); ok && u != "" {
			baseURL = u
		}
	}
	return &AnthropicProvider{cfg: cfg, baseURL: baseURL}
}

// Init 校验配置。
func (p *AnthropicProvider) Init(ctx context.Context) error {
	return nil
}

// ListModels 返回配置中声明的模型列表。
func (p *AnthropicProvider) ListModels(ctx context.Context) (map[string]string, error) {
	return listModelsFromConfig(p.cfg, "anthropic"), nil
}

// getAPIKey 从配置或环境变量获取 apiKey。
func (p *AnthropicProvider) getAPIKey() string {
	if k := getAPIKeyFromConfig(p.cfg, "anthropic"); k != "" {
		return k
	}
	// 默认环境变量 fallback
	if v := config.ResolveEnvPlaceholder("{env:ANTHROPIC_API_KEY}"); v != "" && !strings.HasPrefix(v, "{env:") {
		return v
	}
	return ""
}

// StreamComplete 调用 /v1/messages 流式接口。
func (p *AnthropicProvider) StreamComplete(ctx context.Context, req CompleteRequest) (<-chan StreamEvent, error) {
	apiKey := p.getAPIKey()

	body := p.buildRequestBody(req)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	if apiKey != "" {
		httpReq.Header.Set("x-api-key", apiKey)
	}

	resp, err := httpClient().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, wrapHTTPError(resp, "anthropic")
	}

	ch := make(chan StreamEvent, 32)
	go p.parseSSEStream(resp.Body, ch)
	return ch, nil
}

// buildRequestBody 构造 Anthropic messages 请求体。
func (p *AnthropicProvider) buildRequestBody(req CompleteRequest) map[string]any {
	body := map[string]any{
		"model":      req.Model,
		"stream":     true,
		"max_tokens": 8192, // Anthropic 必填字段，默认值
	}
	// 分离 system 消息（Anthropic 单独传 system）
	var systemPrompt string
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			if systemPrompt != "" {
				systemPrompt += "\n"
			}
			systemPrompt += m.Content
			continue
		}
		// Anthropic 不支持 role=tool，需转为 user + tool_result content block
		if m.Role == "tool" {
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     m.Content,
					},
				},
			})
			continue
		}
		// assistant 消息含 tool_calls 需转为 content blocks
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			content := []map[string]any{}
			if m.Content != "" {
				content = append(content, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args any
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
				content = append(content, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": args,
				})
			}
			msgs = append(msgs, map[string]any{"role": "assistant", "content": content})
			continue
		}
		msgs = append(msgs, map[string]any{"role": m.Role, "content": m.Content})
	}
	if systemPrompt != "" {
		body["system"] = systemPrompt
	}
	body["messages"] = msgs

	// 工具
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": t.Function.Parameters,
			})
		}
		body["tools"] = tools
	}
	return body
}

// anthropicToolBuilder 累积 Anthropic 流式工具调用的增量片段。
// Anthropic 工具调用分三个事件：
//  1. content_block_start: 带 tool_use 的 id/name（空 input）
//  2. content_block_delta: input_json_delta（partial_json 片段，需拼接）
//  3. content_block_stop: 该 content block 结束，组装完整 ToolCall 发出
type anthropicToolBuilder struct {
	id         string
	name       string
	inputParts []string
}

// parseSSEStream 解析 Anthropic SSE 流。
// 事件类型：message_start/content_block_start/content_block_delta/content_block_stop/message_delta/message_stop。
func (p *AnthropicProvider) parseSSEStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var eventType string
	var curTool anthropicToolBuilder // 当前累积的 tool_use block
	finished := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			eventType = ""
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		switch eventType {
		case "content_block_start":
			var d struct {
				Index        int `json:"index"`
				ContentBlock struct {
					Type string `json:"type"` // text / tool_use
					ID   string `json:"id"`   // tool_use 的 id
					Name string `json:"name"` // tool_use 的 name
					Text string `json:"text"` // text block 的初始文本（通常为空）
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				ch <- StreamEvent{Type: "error", Err: fmt.Errorf("parse content_block_start: %w", err)}
				return
			}
			if d.ContentBlock.Type == "tool_use" {
				// 开始一个新的 tool_use block
				curTool = anthropicToolBuilder{
					id:   d.ContentBlock.ID,
					name: d.ContentBlock.Name,
				}
			} else if d.ContentBlock.Text != "" {
				ch <- StreamEvent{Type: "text/delta", Text: d.ContentBlock.Text}
			}
		case "content_block_delta":
			var d struct {
				Type  string `json:"type"`
				Delta struct {
					Type        string `json:"type"` // text_delta / input_json_delta
					Text        string `json:"text"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				ch <- StreamEvent{Type: "error", Err: fmt.Errorf("parse content_block_delta: %w", err)}
				return
			}
			switch d.Delta.Type {
			case "text_delta":
				if d.Delta.Text != "" {
					ch <- StreamEvent{Type: "text/delta", Text: d.Delta.Text}
				}
			case "input_json_delta":
				// 累积工具入参的 JSON 片段
				curTool.inputParts = append(curTool.inputParts, d.Delta.PartialJSON)
			}
		case "content_block_stop":
			// 若当前 block 是 tool_use，组装完整 ToolCall 发出
			if curTool.id != "" {
				args := strings.Join(curTool.inputParts, "")
				tc := ToolCall{
					ID:   curTool.id,
					Type: "function",
				}
				tc.Function.Name = curTool.name
				tc.Function.Arguments = args
				ch <- StreamEvent{Type: "tool_call", ToolCall: &tc}
				curTool = anthropicToolBuilder{} // 重置
			}
		case "message_delta":
			var d struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &d); err != nil {
				ch <- StreamEvent{Type: "error", Err: fmt.Errorf("parse message_delta: %w", err)}
				return
			}
			if d.Delta.StopReason != "" && !finished {
				reason := d.Delta.StopReason
				if reason == "end_turn" {
					reason = "stop"
				}
				ch <- StreamEvent{Type: "finish", FinishReason: reason}
				finished = true
			}
			if d.Usage.OutputTokens > 0 {
				ch <- StreamEvent{Type: "usage", Usage: &Usage{OutputTokens: d.Usage.OutputTokens}}
			}
		case "message_start":
			var d struct {
				Message struct {
					Usage struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(data), &d); err == nil && d.Message.Usage.InputTokens > 0 {
				ch <- StreamEvent{Type: "usage", Usage: &Usage{InputTokens: d.Message.Usage.InputTokens}}
			}
		case "message_stop":
			if !finished {
				ch <- StreamEvent{Type: "finish", FinishReason: "stop"}
			}
			return
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{Type: "error", Err: fmt.Errorf("read stream: %w", err)}
	}
}
