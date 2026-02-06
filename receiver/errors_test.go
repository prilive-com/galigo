package receiver_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/receiver"
)

// ==================== WebhookError ====================

func TestWebhookError_Error_WithWrappedError(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &receiver.WebhookError{
		Code:    400,
		Message: "bad request",
		Err:     cause,
	}

	msg := err.Error()

	assert.Contains(t, msg, "400")
	assert.Contains(t, msg, "bad request")
	assert.Contains(t, msg, "underlying cause")
}

func TestWebhookError_Error_WithoutWrappedError(t *testing.T) {
	err := &receiver.WebhookError{
		Code:    403,
		Message: "forbidden",
		Err:     nil,
	}

	msg := err.Error()

	assert.Contains(t, msg, "403")
	assert.Contains(t, msg, "forbidden")
}

func TestWebhookError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &receiver.WebhookError{
		Code:    500,
		Message: "internal error",
		Err:     cause,
	}

	unwrapped := err.Unwrap()

	assert.Equal(t, cause, unwrapped)
	assert.True(t, errors.Is(err, cause))
}

func TestWebhookError_Unwrap_Nil(t *testing.T) {
	err := &receiver.WebhookError{
		Code:    404,
		Message: "not found",
		Err:     nil,
	}

	unwrapped := err.Unwrap()

	assert.Nil(t, unwrapped)
}

// ==================== APIError ====================

func TestAPIError_Error_WithWrappedError(t *testing.T) {
	cause := errors.New("network failure")
	err := &receiver.APIError{
		Code:        502,
		Description: "Bad Gateway",
		Err:         cause,
	}

	msg := err.Error()

	assert.Contains(t, msg, "502")
	assert.Contains(t, msg, "Bad Gateway")
	assert.Contains(t, msg, "network failure")
}

func TestAPIError_Error_WithoutWrappedError(t *testing.T) {
	err := &receiver.APIError{
		Code:        401,
		Description: "Unauthorized",
		Err:         nil,
	}

	msg := err.Error()

	assert.Contains(t, msg, "401")
	assert.Contains(t, msg, "Unauthorized")
}

func TestAPIError_Unwrap(t *testing.T) {
	cause := errors.New("connection timeout")
	err := &receiver.APIError{
		Code:        504,
		Description: "Gateway Timeout",
		Err:         cause,
	}

	unwrapped := err.Unwrap()

	assert.Equal(t, cause, unwrapped)
	assert.True(t, errors.Is(err, cause))
}

func TestAPIError_Unwrap_Nil(t *testing.T) {
	err := &receiver.APIError{
		Code:        429,
		Description: "Too Many Requests",
		Err:         nil,
	}

	unwrapped := err.Unwrap()

	assert.Nil(t, unwrapped)
}

// ==================== Sentinel Errors ====================

func TestSentinelErrors_AreDistinct(t *testing.T) {
	sentinels := []error{
		receiver.ErrAlreadyRunning,
		receiver.ErrNotRunning,
		receiver.ErrTokenRequired,
		receiver.ErrWebhookURLRequired,
		receiver.ErrTLSRequired,
		receiver.ErrForbidden,
		receiver.ErrUnauthorized,
		receiver.ErrMethodNotAllowed,
		receiver.ErrChannelBlocked,
		receiver.ErrRateLimited,
	}

	// Verify all sentinels are distinct
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j {
				assert.False(t, errors.Is(a, b), "sentinel %d should not match sentinel %d", i, j)
			}
		}
	}
}

func TestSentinelErrors_CanBeMatched(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
	}{
		{"ErrAlreadyRunning", receiver.ErrAlreadyRunning, receiver.ErrAlreadyRunning},
		{"ErrNotRunning", receiver.ErrNotRunning, receiver.ErrNotRunning},
		{"ErrChannelBlocked", receiver.ErrChannelBlocked, receiver.ErrChannelBlocked},
		{"ErrRateLimited", receiver.ErrRateLimited, receiver.ErrRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, errors.Is(tt.err, tt.sentinel))
		})
	}
}

func TestSentinelErrors_HaveDescriptiveMessages(t *testing.T) {
	tests := []struct {
		err      error
		contains string
	}{
		{receiver.ErrAlreadyRunning, "already running"},
		{receiver.ErrNotRunning, "not running"},
		{receiver.ErrTokenRequired, "token required"},
		{receiver.ErrChannelBlocked, "channel"},
		{receiver.ErrRateLimited, "rate"},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			assert.Contains(t, tt.err.Error(), tt.contains)
		})
	}
}

// ==================== ErrorAs ====================

func TestAPIError_ErrorAs(t *testing.T) {
	err := &receiver.APIError{
		Code:        400,
		Description: "Bad Request",
	}

	var apiErr *receiver.APIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
	assert.Equal(t, "Bad Request", apiErr.Description)
}

func TestWebhookError_ErrorAs(t *testing.T) {
	err := &receiver.WebhookError{
		Code:    413,
		Message: "payload too large",
	}

	var webhookErr *receiver.WebhookError
	assert.True(t, errors.As(err, &webhookErr))
	assert.Equal(t, 413, webhookErr.Code)
	assert.Equal(t, "payload too large", webhookErr.Message)
}
