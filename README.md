# gtrv-go

A zero-dependency, highly testable Go library for solving Gt captchas remotely. 

It is a clean port of the `remoteValidator` mechanism from [autopcr](https://github.com/cc004/autopcr).

## Installation

```bash
go get github.com/cca2878/gtrv-go
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cca2878/gtrv-go"
)

func main() {
	// Pass any custom HTTPClient, or nil for http.DefaultClient
	validator := gtrv.NewRemoteValidator(nil)

	result, err := validator.Validate(context.Background())
	if err != nil {
		log.Fatalf("Validation failed: %v", err)
	}

	fmt.Printf("Success! Challenge: %s, Validate: %s\n", result.Challenge, result.Validate)
}
```

## Error Handling

Errors can be cleanly handled using standard `errors.Is`:
* `ErrCaptchaFailed`: General remote/captcha error.
* `ErrQueueTooLong`: Remote queue length limit exceeded.
* `ErrMaxRetriesExceeded`: Polling limit reached.

## Testing

```bash
go test -v ./...
```

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPLv3).

See the full text in the `LICENSE` file or at https://www.gnu.org/licenses/agpl-3.0.html