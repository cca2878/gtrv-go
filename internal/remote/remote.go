package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient 定义了发送 HTTP 请求所需的接口。
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ValidationResult 包含验证成功后的详细信息。
type ValidationResult struct {
	Challenge string `json:"challenge"`
	Gt        string `json:"gt"`
	GtUserId  string `json:"gt_user_id"`
	Validate  string `json:"validate"`
}

// RemoteValidator 结构体具体实现了验证码解析逻辑。
type RemoteValidator struct {
	client       HTTPClient
	sleep        func(ctx context.Context, d time.Duration) error
	baseURL      string
	userAgent    string
	maxRounds    int           // 轮询上限
	runningDelay time.Duration // "in running" 状态的等待间隔
}

const (
	defaultBaseURL      = "https://pcrd.tencentbot.top"
	defaultUserAgent    = "autopcr/1.0.0"
	defaultMaxRounds    = 5
	defaultRunningDelay = 8 * time.Second
)

// Option 定制远程验证器（求解服务地址、UA、轮询参数等）。
type Option func(*RemoteValidator)

// WithBaseURL 覆盖求解服务的基础地址（默认 pcrd.tencentbot.top）。
func WithBaseURL(u string) Option {
	return func(v *RemoteValidator) {
		if u != "" {
			v.baseURL = u
		}
	}
}

// WithUserAgent 覆盖请求 User-Agent。
func WithUserAgent(ua string) Option {
	return func(v *RemoteValidator) {
		if ua != "" {
			v.userAgent = ua
		}
	}
}

// WithMaxRounds 覆盖轮询上限轮数（默认 5）。
func WithMaxRounds(n int) Option {
	return func(v *RemoteValidator) {
		if n > 0 {
			v.maxRounds = n
		}
	}
}

// WithRunningPollInterval 覆盖 "in running" 状态的等待间隔（默认 8s）。
func WithRunningPollInterval(d time.Duration) Option {
	return func(v *RemoteValidator) {
		if d > 0 {
			v.runningDelay = d
		}
	}
}

// NewRemoteValidator 创建一个远程验证器实例。client 为 nil 时默认 http.DefaultClient。
func NewRemoteValidator(client HTTPClient, opts ...Option) *RemoteValidator {
	if client == nil {
		client = http.DefaultClient
	}
	v := &RemoteValidator{
		client: client,
		sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		},
		baseURL:      defaultBaseURL,
		userAgent:    defaultUserAgent,
		maxRounds:    defaultMaxRounds,
		runningDelay: defaultRunningDelay,
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// SetSleepFunc 允许在测试中自定义 sleep 函数，使轮询过程免于实际等待时间。
func (v *RemoteValidator) SetSleepFunc(fn func(ctx context.Context, d time.Duration) error) {
	if fn != nil {
		v.sleep = fn
	}
}

// Validate 执行验证码解析，基于 Python 的 remoteValidator 逻辑轮询服务端。
func (v *RemoteValidator) Validate(ctx context.Context) (*ValidationResult, error) {
	// 1. 获取 geetest_renew 分配 uuid
	renewURL := v.baseURL + "/geetest_renew"
	req, err := http.NewRequestWithContext(ctx, "GET", renewURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create renew request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", v.userAgent)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("renew request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("renew request returned status %d: %s", resp.StatusCode, resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read renew response body: %w", err)
	}

	var renewResp struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(bodyBytes, &renewResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal renew response: %w", err)
	}

	uuid := renewResp.UUID
	if uuid == "" {
		return nil, fmt.Errorf("received empty uuid from renew request: %w", ErrCaptchaFailed)
	}

	// 2. 轮询 check 接口
	ccnt := 0
	up := v.maxRounds

	for ccnt <= up {
		// 检查 context 是否已被取消
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ccnt++
		checkURL := fmt.Sprintf("%s/check/%s", v.baseURL, uuid)
		checkReq, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create check request: %w", err)
		}
		checkReq.Header.Set("Content-Type", "application/json")
		checkReq.Header.Set("User-Agent", v.userAgent)

		checkResp, err := v.client.Do(checkReq)
		if err != nil {
			return nil, fmt.Errorf("check request failed: %w", err)
		}

		bodyBytes, err = io.ReadAll(checkResp.Body)
		_ = checkResp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read check response body: %w", err)
		}

		if checkResp.StatusCode < 200 || checkResp.StatusCode >= 300 {
			return nil, fmt.Errorf("check request returned status %d: %s", checkResp.StatusCode, checkResp.Status)
		}

		// 解析返回结果，可能包含 queue_num 或者 info 字段
		var rawResult map[string]json.RawMessage
		if err := json.Unmarshal(bodyBytes, &rawResult); err != nil {
			return nil, fmt.Errorf("failed to parse check response JSON: %w", err)
		}

		// 2.1 检查 queue_num 是否存在
		if queueNumRaw, ok := rawResult["queue_num"]; ok {
			var nu int
			if err := json.Unmarshal(queueNumRaw, &nu); err != nil {
				return nil, fmt.Errorf("failed to unmarshal queue_num: %w", err)
			}

			if nu >= 35 {
				return nil, ErrQueueTooLong
			}

			// tim = min(int(nu), 3) * 10
			tim := nu
			if tim > 3 {
				tim = 3
			}
			tim *= 10

			// 延迟等待，期间注意响应 ctx 取消
			if err := v.sleep(ctx, time.Duration(tim)*time.Second); err != nil {
				return nil, err
			}

			if tim >= 40 {
				ccnt += 2
			}
			continue
		}

		// 2.2 如果没有 queue_num，提取 info 并处理
		infoRaw, ok := rawResult["info"]
		if !ok {
			return nil, fmt.Errorf("check response contains neither queue_num nor info: %w", ErrCaptchaFailed)
		}

		// info 可能是字符串（"in running", "fail", "url invalid"），也可能是包含验证信息的 JSON 对象
		var infoStr string
		if err := json.Unmarshal(infoRaw, &infoStr); err == nil {
			if infoStr == "fail" || infoStr == "url invalid" {
				return nil, fmt.Errorf("%w: server returned '%s'", ErrCaptchaFailed, infoStr)
			}
			if infoStr == "in running" {
				if err := v.sleep(ctx, v.runningDelay); err != nil {
					return nil, err
				}
				continue
			}

			// 有些情况下，info 以字符串形式存放了序列化的 JSON 结果
			var result ValidationResult
			if err := json.Unmarshal([]byte(infoStr), &result); err == nil && result.Validate != "" {
				return &result, nil
			}

			// 其余字符串都是非预期状态，直接判失败（不再把整串裸塞进 Validate 返回畸形结果）
			return nil, fmt.Errorf("%w: unexpected info string: %s", ErrCaptchaFailed, infoStr)
		}

		// 如果 info 不是字符串，那么尝试解析为验证成功的 JSON 对象
		var result ValidationResult
		if err := json.Unmarshal(infoRaw, &result); err == nil && result.Validate != "" {
			return &result, nil
		}

		// 既不是预期的状态，也没有 validate 信息
		return nil, fmt.Errorf("%w: unexpected info format/value: %s", ErrCaptchaFailed, string(infoRaw))
	}

	return nil, ErrMaxRetriesExceeded
}
