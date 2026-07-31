package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"ago/internal/config"
)

// TestParseCoreFields 测试核心字段解析
func TestParseCoreFields(t *testing.T) {
	jsonData := []byte(`{
		"model": "opencode/deepseek-v4-flash-free",
		"provider": {
			"opencode": {
				"name": "OpenCode Zen",
				"options": {
					"baseURL": "https://opencode.ai/zen/v1",
					"apiKey": "test-key"
				},
				"models": {
					"deepseek-v4-flash-free": {
						"name": "DeepSeek V4 Flash Free",
						"cost": { "input": 0, "output": 0 }
					}
				}
			}
		},
		"agents": {
			"research": {
				"model": "opencode/deepseek-v4-flash-free",
				"system": "你是调研助手",
				"mode": "subagent"
			}
		}
	}`)

	cfg, err := config.Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Model != "opencode/deepseek-v4-flash-free" {
		t.Errorf("Model = %q, want %q", cfg.Model, "opencode/deepseek-v4-flash-free")
	}

	if len(cfg.Provider) != 1 {
		t.Fatalf("Provider count = %d, want 1", len(cfg.Provider))
	}
	pc, ok := cfg.Provider["opencode"]
	if !ok {
		t.Fatal("provider 'opencode' not found")
	}
	if pc.Name != "OpenCode Zen" {
		t.Errorf("Provider name = %q, want %q", pc.Name, "OpenCode Zen")
	}
	if pc.Options["apiKey"] != "test-key" {
		t.Errorf("apiKey = %v, want %q", pc.Options["apiKey"], "test-key")
	}

	if len(cfg.Agents) != 1 {
		t.Fatalf("Agents count = %d, want 1", len(cfg.Agents))
	}
	ac, ok := cfg.Agents["research"]
	if !ok {
		t.Fatal("agent 'research' not found")
	}
	if ac.System != "你是调研助手" {
		t.Errorf("Agent system = %q, want %q", ac.System, "你是调研助手")
	}
}

// TestParseCompatibleFields 测试兼容字段忽略不报错
func TestParseCompatibleFields(t *testing.T) {
	jsonData := []byte(`{
		"model": "opencode/test",
		"shell": { "bin": "bash" },
		"permissions": { "edit": "allow" },
		"mcp": { "server1": {} },
		"lsp": { "gopls": {} },
		"formatter": { "go": "gofmt" },
		"watcher": { "enabled": true },
		"snapshots": { "enabled": true }
	}`)

	cfg, err := config.Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Model != "opencode/test" {
		t.Errorf("Model = %q, want %q", cfg.Model, "opencode/test")
	}
	// 兼容字段应该被接收（RawMessage 不为空）但不影响核心逻辑
	if cfg.Shell == nil {
		t.Error("Shell field should be received as RawMessage")
	}
}

// TestParseModelID 测试模型 ID 解析
func TestParseModelID(t *testing.T) {
	tests := []struct {
		input        string
		wantProvider string
		wantModel    string
	}{
		{"opencode/deepseek-v4-flash-free", "opencode", "deepseek-v4-flash-free"},
		{"openai/gpt-4o", "openai", "gpt-4o"},
		{"anthropic/claude-sonnet-5", "anthropic", "claude-sonnet-5"},
		{"no-provider", "", "no-provider"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p, m := config.ParseModelID(tt.input)
			if p != tt.wantProvider {
				t.Errorf("providerID = %q, want %q", p, tt.wantProvider)
			}
			if m != tt.wantModel {
				t.Errorf("modelID = %q, want %q", m, tt.wantModel)
			}
		})
	}
}

// TestResolveEnvPlaceholder 测试环境变量占位符解析
func TestResolveEnvPlaceholder(t *testing.T) {
	os.Setenv("TEST_API_KEY", "my-secret-key")
	defer os.Unsetenv("TEST_API_KEY")

	tests := []struct {
		input string
		want  string
	}{
		{"{env:TEST_API_KEY}", "my-secret-key"},
		{"bearer {env:TEST_API_KEY}", "bearer my-secret-key"},
		{"{env:NOT_SET_VAR}", "{env:NOT_SET_VAR}"}, // 未设置保留原样
		{"plain-text", "plain-text"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := config.ResolveEnvPlaceholder(tt.input)
			if got != tt.want {
				t.Errorf("ResolveEnvPlaceholder(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLoadFromFixture 测试从 fixture 文件加载
func TestLoadFromFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "test-config.json")
	cfg, err := config.Load(fixturePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Model != "opencode/test-model" {
		t.Errorf("Model = %q, want %q", cfg.Model, "opencode/test-model")
	}
}

// TestParseAgentSteps 测试 agent 的 steps / maxSteps 字段解析与回退
func TestParseAgentSteps(t *testing.T) {
	jsonData := []byte(`{
		"model": "opencode/test",
		"subagent_depth": 2,
		"agent": {
			"with_steps":    { "model": "opencode/test", "steps": 5 },
			"with_maxsteps": { "model": "opencode/test", "maxSteps": 7 },
			"with_both":     { "model": "opencode/test", "steps": 3, "maxSteps": 9 },
			"with_neither":  { "model": "opencode/test" }
		}
	}`)

	cfg, err := config.Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.SubagentDepth != 2 {
		t.Errorf("SubagentDepth = %d, want 2", cfg.SubagentDepth)
	}
	tests := []struct {
		name     string
		fallback int
		want     int
	}{
		{"with_steps", 99, 5},
		{"with_maxsteps", 99, 7},
		{"with_both", 99, 3},      // steps 优先于 maxSteps
		{"with_neither", 99, 99},  // 回退到 fallback
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.Agents[tt.name].ResolveSteps(tt.fallback)
			if got != tt.want {
				t.Errorf("ResolveSteps(%d) = %d, want %d", tt.fallback, got, tt.want)
			}
		})
	}
}

// TestParseAgentFieldAlias 测试 agent/agents 字段名兼容
func TestParseAgentFieldAlias(t *testing.T) {
	// 原版标准字段名 "agent"（单数）
	t.Run("standard_agent", func(t *testing.T) {
		jsonData := []byte(`{
			"model": "opencode/test",
			"agent": {
				"research": { "model": "opencode/test", "system": "via agent" }
			}
		}`)
		cfg, err := config.Parse(jsonData)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(cfg.Agents) != 1 {
			t.Fatalf("Agents count = %d, want 1", len(cfg.Agents))
		}
		if cfg.Agents["research"].System != "via agent" {
			t.Errorf("System = %q, want %q", cfg.Agents["research"].System, "via agent")
		}
	})

	// 旧别名 "agents"（复数）
	t.Run("alias_agents", func(t *testing.T) {
		jsonData := []byte(`{
			"model": "opencode/test",
			"agents": {
				"research": { "model": "opencode/test", "system": "via agents" }
			}
		}`)
		cfg, err := config.Parse(jsonData)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(cfg.Agents) != 1 {
			t.Fatalf("Agents count = %d, want 1", len(cfg.Agents))
		}
		if cfg.Agents["research"].System != "via agents" {
			t.Errorf("System = %q, want %q", cfg.Agents["research"].System, "via agents")
		}
	})

	// "agent" 优先于 "agents"
	t.Run("agent_takes_priority", func(t *testing.T) {
		jsonData := []byte(`{
			"model": "opencode/test",
			"agent":  { "a1": { "model": "opencode/test" } },
			"agents": { "a2": { "model": "opencode/test" } }
		}`)
		cfg, err := config.Parse(jsonData)
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		if len(cfg.Agents) != 1 {
			t.Fatalf("Agents count = %d, want 1 (agent should take priority)", len(cfg.Agents))
		}
		if _, ok := cfg.Agents["a1"]; !ok {
			t.Error("expected 'a1' from 'agent' field, not found")
		}
	})
}

// TestParseTaskBudget 测试 task_budget 字段解析
func TestParseTaskBudget(t *testing.T) {
	jsonData := []byte(`{
		"model": "opencode/test",
		"agent": {
			"limited":   { "model": "opencode/test", "task_budget": 3 },
			"unlimited": { "model": "opencode/test" }
		}
	}`)

	cfg, err := config.Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Agents["limited"].TaskBudget != 3 {
		t.Errorf("limited.TaskBudget = %d, want 3", cfg.Agents["limited"].TaskBudget)
	}
	if cfg.Agents["unlimited"].TaskBudget != 0 {
		t.Errorf("unlimited.TaskBudget = %d, want 0", cfg.Agents["unlimited"].TaskBudget)
	}
}
