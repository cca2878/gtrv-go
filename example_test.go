package gtrv_test

import (
	"context"
	"fmt"
	"time"

	"github.com/cca2878/gtrv-go"
)

// ExampleNewRemoteValidator demonstrates how to initialize and run the remote validator.
func ExampleNewRemoteValidator() {
	// Initialize the Gt remote validator.
	// You can pass nil to default to http.DefaultClient,
	// or pass a customized *http.Client for proxy or custom timeout settings.
	validator := gtrv.NewRemoteValidator(nil)

	// Create a context with timeout to safely limit the captcha solving duration.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute the validation.
	result, err := validator.Validate(ctx)
	if err != nil {
		// In a real environment, you might want to retry or handle the specific errors:
		// errors.Is(err, gtrv.ErrCaptchaFailed), gtrv.ErrQueueTooLong, etc.
		fmt.Println("validation process completed")
		return
	}

	if result != nil {
		fmt.Println("validation process completed")
	}

	// Output:
	// validation process completed
}
