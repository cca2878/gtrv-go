# gtrv-go

> 极验（GeeTest）Gt 验证码的**远程求解**库——零依赖、可测，把 challenge 委托给远程服务解出 validate。

![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](./LICENSE)

[autopcr](https://github.com/cc004/autopcr) 中 `remoteValidator` 机制的干净 Go 移植。**不做本地推理**：把待解
验证码委托给远程求解服务、排队轮询直到出结果，返回 `(challenge, validate)`。与姊妹库
[gtlv-go](https://github.com/cca2878/gtlv-go)（**本地** wasm 推理求解）互为两条路线；
[bsdkv3-go](https://github.com/cca2878/bsdkv3-go) 经 `WithClientValidator` 消费本库暴露的 `Validator`。

## 用法

```bash
go get github.com/cca2878/gtrv-go
```

```go
import "github.com/cca2878/gtrv-go"

validator := gtrv.NewRemoteValidator(nil) // client 传 nil → http.DefaultClient

res, err := validator.Validate(context.Background())
if err != nil { log.Fatalf("验证失败: %v", err) }

fmt.Printf("challenge=%s validate=%s\n", res.Challenge, res.Validate)
```

作为 [bsdkv3-go](https://github.com/cca2878/bsdkv3-go) 登录时的验证码求解器注入：

```go
bsdkv3.NewClient(ctx, bsdkv3.AppkeyPcr,
    bsdkv3.WithClientValidator(gtrv.NewRemoteValidator(nil)),
)
```

## 选项

`NewRemoteValidator(client, opts...)`：

| Option | 作用 |
|--------|------|
| `WithBaseURL(u)` | 远程求解服务地址 |
| `WithUserAgent(ua)` | 请求 UA |
| `WithMaxRounds(n)` | 轮询最大轮数（达上限 → `ErrMaxRetriesExceeded`） |
| `WithRunningPollInterval(d)` | 轮询间隔 |

`client` 接受任意实现 `gtrv.HTTPClient` 的类型（便于注入自定义传输 / 打桩测试）。

## 错误处理

标准 `errors.Is` 判定：

- `ErrCaptchaFailed` —— 远程 / 验证码通用错误。
- `ErrQueueTooLong` —— 远程队列长度超限。
- `ErrMaxRetriesExceeded` —— 轮询达上限仍未出结果。

## 开发

需要 Go 1.25+。零第三方依赖，`Validate` 走 `HTTPClient` 接口，故可 httptest 完全离线单测。

```bash
go test -v ./...
```

CI（`.github/workflows/ci.yml`，job `Go (fmt · vet · test)`）：gofmt / vet / `go mod verify` / build / test。
分支模型 dev/main（开发提 dev、PR 合 main），提交英文 `<type>: <describe>`。

## 许可证

**[AGPL-3.0](./LICENSE)**。本库是 [autopcr](https://github.com/cc004/autopcr) `remoteValidator` 的移植。
