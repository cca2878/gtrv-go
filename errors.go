package gtrv

import (
	"errors"
	"fmt"
)

// ErrCaptchaFailed 是验证码求解失败的通用哨兵：任何求解阶段的失败——网络错误、
// 非 2xx 状态、响应解析失败、服务端明确失败、非预期响应——都可用 errors.Is 命中它。
// （上下文取消/超时除外，按原样返回 context.Canceled / context.DeadlineExceeded。）
var ErrCaptchaFailed = errors.New("captcha failed")

var (
	// ErrQueueTooLong 表示排队人数过多（>= 35）。是 ErrCaptchaFailed 的细分。
	ErrQueueTooLong = fmt.Errorf("%w: queue is too long", ErrCaptchaFailed)

	// ErrMaxRetriesExceeded 表示超过最大轮询轮数限制仍未完成验证。是 ErrCaptchaFailed 的细分。
	ErrMaxRetriesExceeded = fmt.Errorf("%w: max retries exceeded", ErrCaptchaFailed)
)
