package provider_test

import (
	"testing"

	"ago/internal/config"
	"ago/internal/provider"
)

// TestOpencodeProviderFreeMode 测试 opencode provider 免费模式鉴权
func TestOpencodeProviderFreeMode(t *testing.T) {
	cfg := &config.Config{
		Provider: map[string]config.ProviderConfig{
			"opencode": {
				Name: "OpenCode Zen",
				Options: map[string]any{
					"baseURL": "https://opencode.ai/zen/v1",
				},
				Models: map[string]config.ModelConfig{
					"free-model": {
						Name: "Free Model",
						Cost: &config.ModelCost{Input: 0, Output: 0},
					},
					"paid-model": {
						Name: "Paid Model",
						Cost: &config.ModelCost{Input: 0.01, Output: 0.02},
					},
				},
			},
		},
	}

	p := provider.NewOpencodeProvider(cfg)

	// 测试免费模式：未配置 apiKey，应该返回 "public"
	if apiKey := p.GetAPIKeyForTest(); apiKey != "public" {
		t.Errorf("free mode apiKey = %q, want %q", apiKey, "public")
	}

	// 测试免费模式过滤付费模型
	models, err := p.ListModels(nil)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if _, exists := models["free-model"]; !exists {
		t.Error("free-model should be available in free mode")
	}
	if _, exists := models["paid-model"]; exists {
		t.Error("paid-model should be filtered out in free mode")
	}
}

// TestOpencodeProviderPaidMode 测试 opencode provider 付费模式鉴权
func TestOpencodeProviderPaidMode(t *testing.T) {
	cfg := &config.Config{
		Provider: map[string]config.ProviderConfig{
			"opencode": {
				Name: "OpenCode Zen",
				Options: map[string]any{
					"baseURL": "https://opencode.ai/zen/v1",
					"apiKey":  "sk-user-paid-key",
				},
				Models: map[string]config.ModelConfig{
					"free-model": {
						Name: "Free Model",
						Cost: &config.ModelCost{Input: 0, Output: 0},
					},
					"paid-model": {
						Name: "Paid Model",
						Cost: &config.ModelCost{Input: 0.01, Output: 0.02},
					},
				},
			},
		},
	}

	p := provider.NewOpencodeProvider(cfg)

	// 测试付费模式：配置了 apiKey，应该返回用户 key
	if apiKey := p.GetAPIKeyForTest(); apiKey != "sk-user-paid-key" {
		t.Errorf("paid mode apiKey = %q, want %q", apiKey, "sk-user-paid-key")
	}

	// 测试付费模式不过滤模型
	models, err := p.ListModels(nil)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if _, exists := models["free-model"]; !exists {
		t.Error("free-model should be available in paid mode")
	}
	if _, exists := models["paid-model"]; !exists {
		t.Error("paid-model should be available in paid mode")
	}
}

// TestFactoryUnknownProvider 测试工厂未知 provider 报错
func TestFactoryUnknownProvider(t *testing.T) {
	cfg := &config.Config{}
	factory := provider.NewFactory(cfg)

	_, _, err := factory.GetProvider("unknown/model")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

// TestFactoryGetProvider 测试工厂创建 provider
func TestFactoryGetProvider(t *testing.T) {
	cfg := &config.Config{
		Provider: map[string]config.ProviderConfig{
			"custom-openai": {
				Name: "Custom",
				Options: map[string]any{
					"baseURL": "https://custom.api.com/v1",
				},
			},
		},
	}
	factory := provider.NewFactory(cfg)

	// opencode provider
	p, modelID, err := factory.GetProvider("opencode/test-model")
	if err != nil {
		t.Fatalf("GetProvider opencode failed: %v", err)
	}
	if modelID != "test-model" {
		t.Errorf("modelID = %q, want %q", modelID, "test-model")
	}
	if p == nil {
		t.Error("provider should not be nil")
	}

	// 自定义 OpenAI 兼容 provider
	p2, modelID2, err := factory.GetProvider("custom-openai/gpt-4")
	if err != nil {
		t.Fatalf("GetProvider custom failed: %v", err)
	}
	if modelID2 != "gpt-4" {
		t.Errorf("modelID = %q, want %q", modelID2, "gpt-4")
	}
	if p2 == nil {
		t.Error("provider should not be nil")
	}
}
