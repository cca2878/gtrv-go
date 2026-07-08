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

// Option 定制远程验证器（求解服务地址、UA、轮询参数等）。
type Option = remote.Option

// 求解器可调参数（覆盖默认值）。
var (
	// WithBaseURL 覆盖求解服务的基础地址（默认 pcrd.tencentbot.top）。
	WithBaseURL = remote.WithBaseURL
	// WithUserAgent 覆盖请求 User-Agent。
	WithUserAgent = remote.WithUserAgent
	// WithMaxRounds 覆盖轮询上限轮数（默认 5）。
	WithMaxRounds = remote.WithMaxRounds
	// WithRunningPollInterval 覆盖 "in running" 状态的等待间隔（默认 8s）。
	WithRunningPollInterval = remote.WithRunningPollInterval
)

// NewRemoteValidator 创建一个远程验证器实例，通过依赖注入接受 HTTP 客户端。
// 如果 client 为 nil，将默认使用 http.DefaultClient。可选 opts 覆盖求解服务参数。
func NewRemoteValidator(client HTTPClient, opts ...Option) Validator {
	return remote.NewRemoteValidator(client, opts...)
}
