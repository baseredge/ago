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
