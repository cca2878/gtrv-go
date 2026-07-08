package gtrv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type mockClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func createJSONResponse(statusCode int, data any) (*http.Response, error) {
	bodyBytes, _ := json.Marshal(data)
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}, nil
}

func createStringResponse(statusCode int, text string) (*http.Response, error) {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Body:       io.NopCloser(strings.NewReader(text)),
	}, nil
}

// 辅助函数：创建一个免等待睡眠的 Validator
func newTestValidator(client HTTPClient) Validator {
	v := NewRemoteValidator(client)
	if rv, ok := v.(*RemoteValidator); ok {
		// 同包测试直接替换 sleep，使轮询免于实际等待。
		rv.sleep = func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		}
	}
	return v
}

// 1. 成功测试：info 为 JSON 对象，并含有 validate 字段
func TestRemoteValidator_Success_Object(t *testing.T) {
	callCount := 0
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-uuid-123",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-uuid-123") {
				return createJSONResponse(http.StatusOK, map[string]any{
					"info": map[string]string{
						"challenge":  "test-challenge",
						"validate":   "test-validate",
						"gt_user_id": "test-userid",
						"gt":         "test-gt",
					},
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	res, err := v.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.Challenge != "test-challenge" {
		t.Errorf("expected challenge test-challenge, got %s", res.Challenge)
	}
	if res.Validate != "test-validate" {
		t.Errorf("expected validate test-validate, got %s", res.Validate)
	}
	if res.GtUserId != "test-userid" {
		t.Errorf("expected gt_user_id test-userid, got %s", res.GtUserId)
	}
	if res.Gt != "test-gt" {
		t.Errorf("expected gt test-gt, got %s", res.Gt)
	}

	if callCount != 2 {
		t.Errorf("expected 2 http calls (1 renew, 1 check), got %d", callCount)
	}
}

// 2. 成功测试：info 为包含 validate 字段的 JSON 字符串
func TestRemoteValidator_Success_String(t *testing.T) {
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-uuid-456",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-uuid-456") {
				infoJSON := `{"challenge":"str-challenge","validate":"str-validate","gt_user_id":"str-userid","gt":"str-gt"}`
				return createJSONResponse(http.StatusOK, map[string]string{
					"info": infoJSON,
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	res, err := v.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.Challenge != "str-challenge" || res.Validate != "str-validate" || res.GtUserId != "str-userid" || res.Gt != "str-gt" {
		t.Errorf("unexpected validation result: %+v", res)
	}
}

// 3. 成功测试：返回 queue_num 延迟重试，第二次 check 成功
func TestRemoteValidator_Success_WithQueue(t *testing.T) {
	callCount := 0
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-queue-uuid",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-queue-uuid") {
				if callCount == 2 {
					// 第一次 check：返回排队中
					return createJSONResponse(http.StatusOK, map[string]int{
						"queue_num": 5,
					})
				}
				// 第二次 check：返回成功
				return createJSONResponse(http.StatusOK, map[string]any{
					"info": map[string]string{
						"challenge": "q-challenge",
						"validate":  "q-validate",
					},
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	res, err := v.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.Validate != "q-validate" {
		t.Errorf("expected validation result q-validate, got %s", res.Validate)
	}

	if callCount != 3 {
		t.Errorf("expected 3 http calls (1 renew, 2 checks), got %d", callCount)
	}
}

// 4. 成功测试：返回 "in running" 状态，第二次成功
func TestRemoteValidator_Success_WithInRunning(t *testing.T) {
	callCount := 0
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			callCount++
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-running-uuid",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-running-uuid") {
				if callCount == 2 {
					// 第一次 check: running中
					return createJSONResponse(http.StatusOK, map[string]string{
						"info": "in running",
					})
				}
				// 第二次 check: 成功
				return createJSONResponse(http.StatusOK, map[string]any{
					"info": map[string]string{
						"challenge": "r-challenge",
						"validate":  "r-validate",
					},
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	res, err := v.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.Validate != "r-validate" {
		t.Errorf("expected validation result r-validate, got %s", res.Validate)
	}
}

// 5. 失败测试：queue_num >= 35 导致验证失败
func TestRemoteValidator_Failure_QueueTooLarge(t *testing.T) {
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-uuid",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-uuid") {
				return createJSONResponse(http.StatusOK, map[string]int{
					"queue_num": 35,
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	_, err := v.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrQueueTooLong) {
		t.Errorf("expected ErrQueueTooLong error, got: %v", err)
	}
}

// 6. 失败测试：服务端明确返回 fail 或 url invalid
func TestRemoteValidator_Failure_ErrorStatus(t *testing.T) {
	for _, status := range []string{"fail", "url invalid"} {
		t.Run(status, func(t *testing.T) {
			client := &mockClient{
				doFunc: func(req *http.Request) (*http.Response, error) {
					if strings.Contains(req.URL.Path, "/geetest_renew") {
						return createJSONResponse(http.StatusOK, map[string]string{
							"uuid": "test-uuid",
						})
					}
					if strings.Contains(req.URL.Path, "/check/test-uuid") {
						return createJSONResponse(http.StatusOK, map[string]string{
							"info": status,
						})
					}
					return createStringResponse(http.StatusNotFound, "not found")
				},
			}

			v := newTestValidator(client)
			_, err := v.Validate(context.Background())
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !errors.Is(err, ErrCaptchaFailed) {
				t.Errorf("expected ErrCaptchaFailed error, got: %v", err)
			}
		})
	}
}

// 7. 失败测试：轮询次数超过上限
func TestRemoteValidator_Failure_Timeout(t *testing.T) {
	callCount := 0
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{
					"uuid": "test-uuid",
				})
			}
			if strings.Contains(req.URL.Path, "/check/test-uuid") {
				callCount++
				return createJSONResponse(http.StatusOK, map[string]string{
					"info": "in running",
				})
			}
			return createStringResponse(http.StatusNotFound, "not found")
		},
	}

	v := newTestValidator(client)
	_, err := v.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error due to excessive retries, got nil")
	}

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded error, got: %v", err)
	}

	// 默认轮询上限 maxRounds=6，循环条件 ccnt < 6 → 恰好发起 6 次 check 请求。
	if callCount != 6 {
		t.Errorf("expected 6 check calls, got %d", callCount)
	}
}

// 8. 失败测试：Context 提前取消
func TestRemoteValidator_Failure_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return createJSONResponse(http.StatusOK, map[string]string{
				"uuid": "test-uuid",
			})
		},
	}

	v := newTestValidator(client)
	_, err := v.Validate(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context canceled error, got: %v", err)
	}
}

// 9. nil option 应被安全忽略，不 panic（review #6）。
func TestNilOptionIgnored(t *testing.T) {
	var nilOpt Option
	v := NewRemoteValidator(&mockClient{}, nilOpt, WithMaxRounds(3))
	if v == nil {
		t.Fatal("构造应成功")
	}
}

// 10. WithMaxRounds(n) 应恰好轮询 n 次（诚实语义，review #1）。
func TestWithMaxRoundsHonest(t *testing.T) {
	checks := 0
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/geetest_renew") {
				return createJSONResponse(http.StatusOK, map[string]string{"uuid": "u"})
			}
			checks++
			return createJSONResponse(http.StatusOK, map[string]string{"info": "in running"})
		},
	}
	v := NewRemoteValidator(client, WithMaxRounds(2))
	v.(*RemoteValidator).sleep = func(context.Context, time.Duration) error { return nil }

	_, err := v.Validate(context.Background())
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Fatalf("应超轮询上限: %v", err)
	}
	if checks != 2 {
		t.Errorf("WithMaxRounds(2) 应恰好 2 次 check, got %d", checks)
	}
}

// 11. WithBaseURL 去除尾斜杠，避免 // 畸形路径（review #2）。
func TestWithBaseURLTrimsTrailingSlash(t *testing.T) {
	var renewURL string
	client := &mockClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "geetest_renew") {
				renewURL = req.URL.String()
				return createJSONResponse(http.StatusOK, map[string]string{"uuid": "u"})
			}
			return createJSONResponse(http.StatusOK, map[string]any{
				"info": map[string]string{"validate": "v"},
			})
		},
	}
	v := NewRemoteValidator(client, WithBaseURL("https://host.example/"))
	v.(*RemoteValidator).sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := v.Validate(context.Background()); err != nil {
		t.Fatalf("Validate 失败: %v", err)
	}
	if renewURL != "https://host.example/geetest_renew" {
		t.Errorf("尾斜杠未去除，renewURL=%q", renewURL)
	}
}

// 12. 具体失败哨兵从属于 ErrCaptchaFailed（层级，review）。
func TestSpecificErrorsAreCaptchaFailed(t *testing.T) {
	if !errors.Is(ErrQueueTooLong, ErrCaptchaFailed) || !errors.Is(ErrMaxRetriesExceeded, ErrCaptchaFailed) {
		t.Error("ErrQueueTooLong/ErrMaxRetriesExceeded 应 errors.Is ErrCaptchaFailed")
	}
}

// 13. 传输层失败（状态码/网络）归入 ErrCaptchaFailed，且底层错误仍可 errors.Is（review）。
func TestTransportErrorsWrapCaptchaFailed(t *testing.T) {
	// renew 返回非 2xx 状态
	statusClient := &mockClient{doFunc: func(req *http.Request) (*http.Response, error) {
		return createStringResponse(http.StatusInternalServerError, "boom")
	}}
	if _, err := newTestValidator(statusClient).Validate(context.Background()); !errors.Is(err, ErrCaptchaFailed) {
		t.Errorf("状态码失败应归入 ErrCaptchaFailed, got %v", err)
	}

	// renew 网络错误：底层 err 应仍可被 errors.Is 命中
	netErr := errors.New("dial tcp: connection refused")
	netClient := &mockClient{doFunc: func(req *http.Request) (*http.Response, error) {
		return nil, netErr
	}}
	_, err := newTestValidator(netClient).Validate(context.Background())
	if !errors.Is(err, ErrCaptchaFailed) {
		t.Errorf("网络失败应归入 ErrCaptchaFailed, got %v", err)
	}
	if !errors.Is(err, netErr) {
		t.Errorf("底层网络错误应保留可 errors.Is, got %v", err)
	}
}

// 14. 上下文取消不应被包裹成 ErrCaptchaFailed。
func TestContextCancelNotCaptchaFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &mockClient{doFunc: func(req *http.Request) (*http.Response, error) {
		return createJSONResponse(http.StatusOK, map[string]string{"uuid": "u"})
	}}
	_, err := newTestValidator(client).Validate(ctx)
	if errors.Is(err, ErrCaptchaFailed) {
		t.Errorf("ctx 取消不应归入 ErrCaptchaFailed, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("应为 context.Canceled, got %v", err)
	}
}
