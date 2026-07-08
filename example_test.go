package gtrv_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cca2878/gtrv-go"
)

// ExampleNewRemoteValidator 演示如何初始化并运行远程验证器。
//
// 该示例仅用于文档编译校验，不会在 go test 中实际联网执行（无 Output 指令）。
func ExampleNewRemoteValidator() {
	// 注入自定义 *http.Client（可携带代理 / 共享底层 transport / 自定义超时）；
	// 传 nil 则默认 http.DefaultClient。可选项覆盖求解服务参数。
	validator := gtrv.NewRemoteValidator(
		&http.Client{Timeout: 10 * time.Second},
		gtrv.WithMaxRounds(8),
	)

	// 用带超时的 context 限制整个验证码求解时长。
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := validator.Validate(ctx)
	if err != nil {
		// 可按需区分具体错误：
		switch {
		case errors.Is(err, gtrv.ErrQueueTooLong):
			fmt.Println("排队过长，稍后重试")
		case errors.Is(err, gtrv.ErrMaxRetriesExceeded):
			fmt.Println("轮询超限")
		case errors.Is(err, gtrv.ErrCaptchaFailed):
			fmt.Println("求解失败")
		}
		return
	}

	fmt.Println(result.Validate)
}
