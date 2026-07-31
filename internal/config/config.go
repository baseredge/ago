// Package config 解析 opencode.json 配置文件，格式兼容原版。
// 极简版只解析 model/provider/agents 三个核心字段，其他字段用 json.RawMessage 接收后忽略。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// Config 是 opencode.json 解析后的配置结构。
type Config struct {
	// Model 默认模型 ID，格式 provider/model（如 opencode/deepseek-v4-flash-free）
	Model string `json:"model,omitempty"`

	// Provider 自定义 provider map，key 是 provider id（如 opencode/openai/anthropic）
	Provider map[string]ProviderConfig `json:"provider,omitempty"`

	// Agents 子代理 map，key 是 agent id（如 research/build）
	Agents map[string]AgentConfig `json:"agents,omitempty"`

	// 兼容字段：用 RawMessage 接收后忽略，保持加载不报错
	Schema       json.RawMessage `json:"$schema,omitempty"`
	Shell        json.RawMessage `json:"shell,omitempty"`
	DefaultAgent json.RawMessage `json:"default_agent,omitempty"`
	Autoupdate   json.RawMessage `json:"autoupdate,omitempty"`
	Share        json.RawMessage `json:"share,omitempty"`
	Enterprise   json.RawMessage `json:"enterprise,omitempty"`
	Username     json.RawMessage `json:"username,omitempty"`
	Permissions  json.RawMessage `json:"permissions,omitempty"`
	Snapshots    json.RawMessage `json:"snapshots,omitempty"`
	Watcher      json.RawMessage `json:"watcher,omitempty"`
	Formatter    json.RawMessage `json:"formatter,omitempty"`
	Lsp          json.RawMessage `json:"lsp,omitempty"`
	Attachments  json.RawMessage `json:"attachments,omitempty"`
	ToolOutput   json.RawMessage `json:"tool_output,omitempty"`
	Mcp          json.RawMessage `json:"mcp,omitempty"`
	Compaction   json.RawMessage `json:"compaction,omitempty"`
	Skills       json.RawMessage `json:"skills,omitempty"`
	Commands     json.RawMessage `json:"commands,omitempty"`
	Instructions json.RawMessage `json:"instructions,omitempty"`
	References   json.RawMessage `json:"references,omitempty"`
	SmallModel   json.RawMessage `json:"small_model,omitempty"`
}

// ProviderConfig 是单个 provider 的配置，对齐原版 packages/core/src/config/provider.ts。
type ProviderConfig struct {
	// Name provider 显示名
	Name string `json:"name,omitempty"`

	// Env 环境变量名列表（用于查找 apiKey）
	Env []string `json:"env,omitempty"`

	// Options provider 选项（apiKey/baseURL/headers 等）
	Options map[string]any `json:"options,omitempty"`

	// Npm 对应原版 @ai-sdk/* 包名，用于协议识别
	Npm string `json:"npm,omitempty"`

	// API provider API 信息
	API *ProviderAPI `json:"api,omitempty"`

	// Models 模型列表
	Models map[string]ModelConfig `json:"models,omitempty"`

	// Whitelist 模型白名单
	Whitelist []string `json:"whitelist,omitempty"`

	// Blacklist 模型黑名单
	Blacklist []string `json:"blacklist,omitempty"`
}

// ProviderAPI 是 provider 的 API 信息，对齐原版 ProviderApiInfo。
type ProviderAPI struct {
	Type string `json:"type,omitempty"` // aisdk 等
	URL  string `json:"url,omitempty"`
	Npm  string `json:"npm,omitempty"` // @ai-sdk/openai-compatible 等
}

// ModelConfig 是单个模型的配置，对齐原版 Model schema 的核心字段。
type ModelConfig struct {
	// ID 模型 API ID（可能与 key 不同）
	ID string `json:"id,omitempty"`

	// Name 模型显示名
	Name string `json:"name,omitempty"`

	// Cost 模型费用（免费模型 cost.input=0）
	Cost *ModelCost `json:"cost,omitempty"`

	// Limit 上下文/输出限制
	Limit *ModelLimit `json:"limit,omitempty"`

	// Status 模型状态（active/deprecated/alpha）
	Status string `json:"status,omitempty"`

	// Provider 模型级 provider 覆盖
	Provider *ModelProvider `json:"provider,omitempty"`
}

// ModelCost 模型费用，用于判断免费/付费。
type ModelCost struct {
	Input  float64 `json:"input,omitempty"`
	Output float64 `json:"output,omitempty"`
}

// ModelLimit 模型上下文/输出限制。
type ModelLimit struct {
	Context int `json:"context,omitempty"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output,omitempty"`
}

// ModelProvider 模型级 provider 覆盖。
type ModelProvider struct {
	API string `json:"api,omitempty"`
	Npm string `json:"npm,omitempty"`
	URL string `json:"url,omitempty"`
}

// AgentConfig 是子代理配置，对齐原版 packages/core/src/config/agent.ts。
type AgentConfig struct {
	// Model 子代理使用的模型 ID（格式 provider/model）
	Model string `json:"model,omitempty"`

	// System 子代理系统提示词
	System string `json:"system,omitempty"`

	// Description 子代理描述（供主代理选择时参考）
	Description string `json:"description,omitempty"`

	// Mode 子代理模式：subagent/primary/all
	Mode string `json:"mode,omitempty"`

	// Hidden 是否隐藏（不在选择列表显示）
	Hidden bool `json:"hidden,omitempty"`

	// Tools 工具白名单（空表示继承全部）
	Tools []string `json:"tools,omitempty"`
}

// envPlaceholderRe 匹配 {env:VAR} 占位符。
var envPlaceholderRe = regexp.MustCompile(`\{env:([A-Z_][A-Z0-9_]*)\}`)

// Load 从文件加载配置。
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return Parse(data)
}

// Parse 解析配置 JSON。
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// ResolveEnvPlaceholder 解析字符串中的 {env:VAR} 占位符，返回替换后的值。
// 若环境变量未设置，保留原占位符。
func ResolveEnvPlaceholder(s string) string {
	return envPlaceholderRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := envPlaceholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		if val := os.Getenv(sub[1]); val != "" {
			return val
		}
		return match
	})
}

// ParseModelID 解析 "provider/model" 格式的模型 ID，返回 providerID 和 modelID。
func ParseModelID(modelID string) (providerID, modelIDAfterProvider string) {
	for i := 0; i < len(modelID); i++ {
		if modelID[i] == '/' {
			return modelID[:i], modelID[i+1:]
		}
	}
	return "", modelID
}
