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

	"github.com/cca2878/gtrv-go/internal/remote"
)

type mockClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func createJSONResponse(statusCode int, data interface{}) (*http.Response, error) {
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
	if rv, ok := v.(*remote.RemoteValidator); ok {
		rv.SetSleepFunc(func(ctx context.Context, d time.Duration) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		})
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
				return createJSONResponse(http.StatusOK, map[string]interface{}{
					"info": map[string]string{
						"challenge":  "test-challenge",
						"validate":   "test-validate",
						"gt_user_id": "test-userid",
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
	if res.GTUserID != "test-userid" {
		t.Errorf("expected gt_user_id test-userid, got %s", res.GTUserID)
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
				infoJSON := `{"challenge":"str-challenge","validate":"str-validate","gt_user_id":"str-userid"}`
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

	if res.Challenge != "str-challenge" || res.Validate != "str-validate" || res.GTUserID != "str-userid" {
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
				return createJSONResponse(http.StatusOK, map[string]interface{}{
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
				return createJSONResponse(http.StatusOK, map[string]interface{}{
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

	// 轮询上限为 5 次（ccnt: 0 -> 1 -> 2 -> 3 -> 4 -> 5 -> 6 (退出循环)）
	// 所以一共应该有 6 次 check 请求被发起（ccnt 每次循环最开始进行自增 1，且判断 ccnt <= up(5) 时进行循环）
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
