package base

import "errors"

// 统一错误类型，用于 provider 错误识别与降级处理。

// ErrRateLimited 表示触发 429 限流。
var ErrRateLimited = errors.New("rate limited (429)")

// ErrGatewayUnavailable 表示网关不可用（5xx）。
var ErrGatewayUnavailable = errors.New("gateway unavailable")

// ErrNetworkTimeout 表示网络超时。
var ErrNetworkTimeout = errors.New("network timeout")

// ErrUnauthorized 表示鉴权失败（401/403）。
var ErrUnauthorized = errors.New("unauthorized")

// ProviderError 包装 provider 调用错误，附带状态码便于分类。
type ProviderError struct {
	StatusCode int
	Message    string
	Cause      error
}

func (e *ProviderError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Cause
}

// IsRateLimited 判断错误是否为限流。
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var pe *ProviderError
	if errors.As(err, &pe) && pe.StatusCode == 429 {
		return true
	}
	return false
}
