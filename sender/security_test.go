package sender

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/internal/scrub"
	"github.com/prilive-com/galigo/tg"
)

func TestNoTokenInErrors(t *testing.T) {
	token := tg.SecretToken("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ")

	// Simulate a DNS error containing the token in the URL
	origErr := fmt.Errorf(`Get "https://api.telegram.org/bot%s/getMe": dial tcp: no such host`, token.Value())
	scrubbed := scrub.TokenFromError(origErr, token)

	assert.NotContains(t, scrubbed.Error(), token.Value())
	assert.NotContains(t, scrubbed.Error(), "ABCdef")
	assert.Contains(t, scrubbed.Error(), "[REDACTED]")
}

func TestScrubTokenFromError_Nil(t *testing.T) {
	result := scrub.TokenFromError(nil, "sometoken")
	assert.Nil(t, result)
}

func TestScrubTokenFromError_NoToken(t *testing.T) {
	token := tg.SecretToken("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ")
	origErr := fmt.Errorf("connection refused")
	result := scrub.TokenFromError(origErr, token)
	// Should return original error unchanged
	assert.Equal(t, origErr, result)
}

func TestScrubTokenFromError_PreservesUnwrap(t *testing.T) {
	token := tg.SecretToken("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ")
	inner := fmt.Errorf("inner error")
	origErr := fmt.Errorf(`Get "https://api.telegram.org/bot%s/getMe": %w`, token.Value(), inner)
	scrubbed := scrub.TokenFromError(origErr, token)

	// Scrubbed message should not contain token
	assert.NotContains(t, scrubbed.Error(), token.Value())

	// Unwrap chain should be preserved
	assert.True(t, errors.Is(scrubbed, inner))
}

func TestBreakerSuccess_400IsSuccess(t *testing.T) {
	// 400 Bad Request should NOT trip the breaker
	err := tg.NewAPIError("sendMessage", 400, "Bad Request: chat not found")
	assert.True(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_403IsSuccess(t *testing.T) {
	// 403 Forbidden should NOT trip the breaker
	err := tg.NewAPIError("sendMessage", 403, "Forbidden: bot was blocked by the user")
	assert.True(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_404IsSuccess(t *testing.T) {
	// 404 Not Found should NOT trip the breaker
	err := tg.NewAPIError("sendMessage", 404, "Not Found")
	assert.True(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_429IsSuccess(t *testing.T) {
	// 429 Too Many Requests should NOT trip the breaker — handle via retry_after
	err := tg.NewAPIError("sendMessage", 429, "Too Many Requests: retry after 30")
	assert.True(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_500IsFailure(t *testing.T) {
	// 500 Internal Server Error SHOULD trip the breaker
	err := tg.NewAPIError("sendMessage", 500, "Internal Server Error")
	assert.False(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_NilIsSuccess(t *testing.T) {
	assert.True(t, isBreakerSuccess(nil))
}

func TestBreakerSuccess_NetworkErrorIsFailure(t *testing.T) {
	err := fmt.Errorf("dial tcp: connection refused")
	assert.False(t, isBreakerSuccess(err))
}

func TestBreakerSuccess_ContextCancelIsSuccess(t *testing.T) {
	// Context cancellation should NOT trip the breaker
	assert.True(t, isBreakerSuccess(context.Canceled))
	assert.True(t, isBreakerSuccess(context.DeadlineExceeded))
}
