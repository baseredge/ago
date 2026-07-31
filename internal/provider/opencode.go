package provider

import (
	"context"
	"strings"

	"ago/internal/config"
)

// opencodeProviderBaseURL 是 opencode provider（Zen 网关）的默认 baseURL。
const opencodeProviderBaseURL = "https://opencode.ai/zen/v1"

// opencodeProviderAPIKeyFree 是免费模式的硬编码 apiKey，对齐原版逻辑。
// 参考原版 packages/opencode/src/provider/provider.ts 第 199 行：`options: ok ? {} : { apiKey: "public" }`
const opencodeProviderAPIKeyFree = "public"

// OpencodeProvider 是 opencode provider（对应 Zen 网关），双模式鉴权。
// 复用 OpenAI 兼容协议（/chat/completions 端点），仅覆盖走该端点的 Zen 模型。
//
// 实现说明：Go 不支持对父类内部调用的私有方法做动态分派，
// 因此通过注入 apiKeyFn（函数字段）实现覆盖，而非嵌入+方法重写。
type OpencodeProvider struct {
	*OpenAICompatibleProvider
}

// NewOpencodeProvider 创建 opencode provider。
// baseURL 默认 https://opencode.ai/zen/v1，可在 opencode.json 中覆盖。
func NewOpencodeProvider(cfg *config.Config) *OpencodeProvider {
	baseURL := opencodeProviderBaseURL
	if pcfg, ok := cfg.Provider["opencode"]; ok && pcfg.Options != nil {
		if u, ok := pcfg.Options["baseURL"].(string); ok && u != "" {
			baseURL = u
		}
	}
	inner := NewOpenAICompatibleProvider(cfg, "opencode", baseURL)
	// 注入双模式鉴权逻辑（覆盖默认 apiKeyFn）
	inner.apiKeyFn = opencodeAPIKeyFn(cfg)
	return &OpencodeProvider{OpenAICompatibleProvider: inner}
}

// opencodeAPIKeyFn 返回 opencode provider 的 apiKey 获取函数（双模式鉴权）。
// 对齐原版 packages/opencode/src/provider/provider.ts 第 179-201 行 opencode provider loader：
//   - 有 apiKey/env var 时 → 付费模式，所有模型可用
//   - 无 apiKey 时 → 免费模式，硬编码 apiKey="public"，仅暴露 cost.input=0 的免费模型
func opencodeAPIKeyFn(cfg *config.Config) func() string {
	return func() string {
		if pcfg, ok := cfg.Provider["opencode"]; ok {
			// 1. options.apiKey
			if pcfg.Options != nil {
				if k, ok := pcfg.Options["apiKey"].(string); ok && k != "" {
					return k
				}
			}
			// 2. 环境变量
			for _, env := range pcfg.Env {
				if v := config.ResolveEnvPlaceholder("{env:" + env + "}"); v != "" && !strings.HasPrefix(v, "{env:") {
					return v
				}
			}
		}
		// 3. 免费模式：硬编码 apiKey="public"
		return opencodeProviderAPIKeyFree
	}
}

// isFreeMode 判断是否为免费模式（无付费 key）。
func (p *OpencodeProvider) isFreeMode() bool {
	if pcfg, ok := p.cfg.Provider["opencode"]; ok {
		if pcfg.Options != nil {
			if k, ok := pcfg.Options["apiKey"].(string); ok && k != "" {
				return false
			}
		}
		for _, env := range pcfg.Env {
			if v := config.ResolveEnvPlaceholder("{env:" + env + "}"); v != "" && !strings.HasPrefix(v, "{env:") {
				return false
			}
		}
	}
	return true
}

// ListModels 覆盖父类，免费模式下过滤掉非免费模型（cost.input != 0）。
// 对齐原版第 190-195 行：`if (value.cost.input === 0) continue; delete input.models[key];`
func (p *OpencodeProvider) ListModels(ctx context.Context) (map[string]string, error) {
	models := make(map[string]string)
	if pcfg, ok := p.cfg.Provider["opencode"]; ok {
		for id, m := range pcfg.Models {
			// 免费模式下过滤付费模型
			if p.isFreeMode() {
				if m.Cost == nil || m.Cost.Input != 0 {
					continue
				}
			}
			name := m.Name
			if name == "" {
				name = id
			}
			models[id] = name
		}
	}
	return models, nil
}

// GetAPIKeyForTest 返回当前 apiKey（仅用于测试，暴露 apiKeyFn 逻辑）。
func (p *OpencodeProvider) GetAPIKeyForTest() string {
	return p.apiKeyFn()
}
