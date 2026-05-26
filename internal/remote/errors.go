package remote

import "errors"

var (
	// ErrCaptchaFailed 表示远程验证码解析遇到通用错误或服务端明确返回失败
	ErrCaptchaFailed = errors.New("captcha failed")

	// ErrQueueTooLong 表示排队人数过多（>= 35）导致的验证码解析失败
	ErrQueueTooLong = errors.New("captcha failed: queue is too long")

	// ErrMaxRetriesExceeded 表示超过最大轮询轮数限制仍未完成验证
	ErrMaxRetriesExceeded = errors.New("captcha failed: max retries exceeded")
)
