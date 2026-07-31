package provider

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ago/internal/base"
	"ago/internal/config"
)

// getAPIKeyFromConfig 从配置的 provider.Options.apiKey 或 Env 环境变量获取 apiKey。
// 三个 provider 共用的逻辑，避免重复实现。
func getAPIKeyFromConfig(cfg *config.Config, providerID string) string {
	if pcfg, ok := cfg.Provider[providerID]; ok {
		// 优先 options.apiKey
		if pcfg.Options != nil {
			if k, ok := pcfg.Options["apiKey"].(string); ok && k != "" {
				return k
			}
		}
		// 其次环境变量
		for _, env := range pcfg.Env {
			if v := config.ResolveEnvPlaceholder("{env:" + env + "}"); v != "" && !strings.HasPrefix(v, "{env:") {
				return v
			}
		}
	}
	return ""
}

// listModelsFromConfig 从配置的 provider.Models 返回模型列表。
// 三个 provider 共用的逻辑。
func listModelsFromConfig(cfg *config.Config, providerID string) map[string]string {
	models := make(map[string]string)
	if pcfg, ok := cfg.Provider[providerID]; ok {
		for id, m := range pcfg.Models {
			name := m.Name
			if name == "" {
				name = id
			}
			models[id] = name
		}
	}
	return models
}

// wrapHTTPError 将 HTTP 错误响应包装为 ProviderError。
// providerTag 用于日志标识（如 "openai" / "anthropic"）。
// 两个 provider 共用的逻辑，避免重复实现。
func wrapHTTPError(resp *http.Response, providerTag string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	// 错误体可能含敏感信息，日志只截断前 200 字符
	msg := truncateForLog(string(body), 200)
	pe := &base.ProviderError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, msg)}
	switch resp.StatusCode {
	case 429:
		pe.Cause = base.ErrRateLimited
	case 401, 403:
		pe.Cause = base.ErrUnauthorized
	case 500, 502, 503, 504:
		pe.Cause = base.ErrGatewayUnavailable
	}
	base.Errorf("provider %s request failed: HTTP %d", providerTag, resp.StatusCode)
	return pe
}

// truncateForLog 截断字符串用于日志输出，避免泄漏过多错误体内容。
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// sharedHTTPClient 是共享 HTTP client，避免每个 provider 实例化。
// 流式请求不能用 client 级 Timeout（会中断流），改由调用方通过 context 控制单次请求 deadline。
var sharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	},
}

// httpClient 返回共享 HTTP client。
func httpClient() *http.Client {
	return sharedHTTPClient
}
