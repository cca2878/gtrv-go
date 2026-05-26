package gtrv

import (
	"context"

	"github.com/cca2878/gtrv-go/internal/remote"
)

// HTTPClient 定义了发送 HTTP 请求所需的接口。
// 外部调用者可以注入满足此接口的自定义 HTTP 客户端（例如带有代理、超时或标准库的 *http.Client）。
type HTTPClient = remote.HTTPClient

// Validator 暴露给外部调用的统一验证契约接口。
type Validator interface {
	Validate(ctx context.Context) (*ValidationResult, error)
}

// ValidationResult 包含验证成功后的详细信息。
type ValidationResult = remote.ValidationResult

// 定义公开可用的错误，外部可以通过 errors.Is 判断具体的错误类型
var (
	// ErrCaptchaFailed 表示远程验证码解析遇到通用错误或服务端明确返回失败
	ErrCaptchaFailed = remote.ErrCaptchaFailed

	// ErrQueueTooLong 表示排队人数过多（>= 35）导致的验证码解析失败
	ErrQueueTooLong = remote.ErrQueueTooLong

	// ErrMaxRetriesExceeded 表示超过最大轮询轮数限制仍未完成验证
	ErrMaxRetriesExceeded = remote.ErrMaxRetriesExceeded
)

// NewRemoteValidator 创建一个远程验证器实例，通过依赖注入接受 HTTP 客户端。
// 如果 client 为 nil，将默认使用 http.DefaultClient。
func NewRemoteValidator(client HTTPClient) Validator {
	return remote.NewRemoteValidator(client)
}
