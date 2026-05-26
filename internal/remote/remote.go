package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient 定义了发送 HTTP 请求所需的接口。
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ValidationResult 包含验证成功后的详细信息。
type ValidationResult struct {
	Challenge string `json:"challenge"`
	Validate  string `json:"validate"`
	GTUserID  string `json:"gt_user_id,omitempty"`
}

// RemoteValidator 结构体具体实现了验证码解析逻辑。
type RemoteValidator struct {
	client HTTPClient
	sleep  func(ctx context.Context, d time.Duration) error
}

const (
	baseURL   = "https://pcrd.tencentbot.top"
	userAgent = "autopcr/1.0.0"
)

// NewRemoteValidator 创建一个远程验证器实例。
func NewRemoteValidator(client HTTPClient) *RemoteValidator {
	if client == nil {
		client = http.DefaultClient
	}
	return &RemoteValidator{
		client: client,
		sleep: func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		},
	}
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
	renewURL := baseURL + "/geetest_renew"
	req, err := http.NewRequestWithContext(ctx, "GET", renewURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create renew request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

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
	up := 5

	for ccnt <= up {
		// 检查 context 是否已被取消
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ccnt++
		checkURL := fmt.Sprintf("%s/check/%s", baseURL, uuid)
		checkReq, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create check request: %w", err)
		}
		checkReq.Header.Set("Content-Type", "application/json")
		checkReq.Header.Set("User-Agent", userAgent)

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

		// info 可能是字符串，也可能是包含验证信息的对象
		var infoStr string
		var infoMap map[string]interface{}

		isString := json.Unmarshal(infoRaw, &infoStr) == nil
		isMap := json.Unmarshal(infoRaw, &infoMap) == nil

		if isString {
			if infoStr == "fail" || infoStr == "url invalid" {
				return nil, fmt.Errorf("%w: server returned '%s'", ErrCaptchaFailed, infoStr)
			}
			if infoStr == "in running" {
				if err := v.sleep(ctx, 8*time.Second); err != nil {
					return nil, err
				}
				continue
			}

			// 有些情况下，info 以字符串形式存放了序列化的 JSON 结果
			var result ValidationResult
			if err := json.Unmarshal([]byte(infoStr), &result); err == nil && result.Validate != "" {
				return &result, nil
			}

			// 如果不是 JSON，但包含了 validate 关键字，提取或包裹返回
			if strings.Contains(infoStr, "validate") {
				return &ValidationResult{
					Validate: infoStr,
				}, nil
			}
		}

		if isMap {
			// 如果 info 是一个 JSON 对象，检查是否包含 "validate" 字段
			if val, ok := infoMap["validate"]; ok {
				var result ValidationResult
				if err := json.Unmarshal(infoRaw, &result); err == nil && result.Validate != "" {
					return &result, nil
				}
				// 备用：手动进行字段映射
				if vStr, ok := val.(string); ok {
					res := &ValidationResult{Validate: vStr}
					if ch, ok := infoMap["challenge"].(string); ok {
						res.Challenge = ch
					}
					if uid, ok := infoMap["gt_user_id"].(string); ok {
						res.GTUserID = uid
					}
					return res, nil
				}
			}
		}

		// 既不是预期的状态，也没有 validate 信息
		return nil, fmt.Errorf("%w: unexpected info format/value: %s", ErrCaptchaFailed, string(infoRaw))
	}

	return nil, ErrMaxRetriesExceeded
}
