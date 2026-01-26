package tg_test

import (
	"errors"
	"testing"
	"time"

	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *tg.APIError
		expected string
	}{
		{
			name: "basic error",
			err: &tg.APIError{
				Code:        400,
				Description: "Bad Request",
				Method:      "sendMessage",
			},
			expected: "galigo: sendMessage failed: Bad Request (code=400)",
		},
		{
			name: "error with retry_after",
			err: &tg.APIError{
				Code:        429,
				Description: "Too Many Requests",
				Method:      "sendMessage",
				RetryAfter:  30 * time.Second,
			},
			expected: "galigo: sendMessage failed: Too Many Requests (code=429, retry_after=30s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		retryable bool
	}{
		{"200 ok", 200, false},
		{"400 bad request", 400, false},
		{"401 unauthorized", 401, false},
		{"403 forbidden", 403, false},
		{"404 not found", 404, false},
		{"429 rate limited", 429, true},
		{"500 internal server error", 500, true},
		{"502 bad gateway", 502, true},
		{"503 service unavailable", 503, true},
		{"504 gateway timeout", 504, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &tg.APIError{Code: tt.code}
			assert.Equal(t, tt.retryable, err.IsRetryable())
		})
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	err := tg.NewAPIError("sendMessage", 403, "Forbidden: bot was blocked by the user")
	require.NotNil(t, err)

	// Should unwrap to ErrBotBlocked
	assert.True(t, errors.Is(err, tg.ErrBotBlocked))
}

func TestNewAPIError(t *testing.T) {
	err := tg.NewAPIError("sendMessage", 400, "Bad Request: chat not found")

	assert.Equal(t, 400, err.Code)
	assert.Equal(t, "Bad Request: chat not found", err.Description)
	assert.Equal(t, "sendMessage", err.Method)
	assert.True(t, errors.Is(err, tg.ErrChatNotFound))
}

func TestNewAPIErrorWithRetry(t *testing.T) {
	err := tg.NewAPIErrorWithRetry("sendMessage", 429, "Too Many Requests", 30*time.Second)

	assert.Equal(t, 429, err.Code)
	assert.Equal(t, 30*time.Second, err.RetryAfter)
	assert.True(t, errors.Is(err, tg.ErrTooManyRequests))
}

func TestDetectSentinel(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		desc     string
		expected error
	}{
		// Description-based detection (takes precedence)
		{"message not modified", 400, "Bad Request: message is not modified", tg.ErrMessageNotModified},
		{"message to edit not found", 400, "Bad Request: message to edit not found", tg.ErrMessageNotFound},
		{"message to delete not found", 400, "Bad Request: message to delete not found", tg.ErrMessageNotFound},
		{"message not found generic", 400, "Bad Request: message not found", tg.ErrMessageNotFound},
		{"message can't be edited", 400, "Bad Request: message can't be edited", tg.ErrMessageCantBeEdited},
		{"message can't be deleted", 400, "Bad Request: message can't be deleted", tg.ErrMessageCantBeDeleted},
		{"message too old", 400, "Bad Request: message is too old", tg.ErrMessageTooOld},
		{"bot blocked", 403, "Forbidden: bot was blocked by the user", tg.ErrBotBlocked},
		{"bot kicked", 403, "Forbidden: bot was kicked from the chat", tg.ErrBotKicked},
		{"chat not found", 400, "Bad Request: chat not found", tg.ErrChatNotFound},
		{"user deactivated", 403, "Forbidden: user is deactivated", tg.ErrUserDeactivated},
		{"not enough rights", 400, "Bad Request: not enough rights to send messages", tg.ErrNoRights},
		{"callback expired", 400, "Bad Request: query is too old and response timeout expired", tg.ErrCallbackExpired},
		{"invalid callback data", 400, "Bad Request: BUTTON_DATA_INVALID", tg.ErrInvalidCallbackData},

		// HTTP status code fallbacks
		{"401 unauthorized", 401, "Unauthorized", tg.ErrUnauthorized},
		{"403 forbidden generic", 403, "Forbidden", tg.ErrForbidden},
		{"404 not found generic", 404, "Not Found", tg.ErrNotFound},
		{"429 too many requests", 429, "Too Many Requests", tg.ErrTooManyRequests},

		// No match
		{"unknown error", 500, "Internal Server Error", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tg.DetectSentinel(tt.code, tt.desc)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectSentinel_DescriptionPriority(t *testing.T) {
	// When description contains specific error, it should take precedence over code
	err := tg.DetectSentinel(403, "Forbidden: bot was blocked by the user")
	assert.Equal(t, tg.ErrBotBlocked, err, "description should take precedence over code")

	err = tg.DetectSentinel(404, "Not Found: message to edit not found")
	assert.Equal(t, tg.ErrMessageNotFound, err, "description should take precedence over code")
}

func TestValidationError(t *testing.T) {
	err := tg.NewValidationError("chat_id", "must be non-zero")

	assert.Equal(t, "galigo: validation: chat_id - must be non-zero", err.Error())
	assert.Equal(t, "chat_id", err.Field)
	assert.Equal(t, "must be non-zero", err.Message)
}

func TestConfigError(t *testing.T) {
	err := tg.NewConfigError("timeout", "must be positive")

	assert.Equal(t, "galigo: config: timeout - must be positive", err.Error())
	assert.Equal(t, "timeout", err.Key)
	assert.Equal(t, "must be positive", err.Message)
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	// Verify all sentinel errors are distinct
	sentinels := []error{
		tg.ErrUnauthorized,
		tg.ErrForbidden,
		tg.ErrNotFound,
		tg.ErrTooManyRequests,
		tg.ErrMessageNotFound,
		tg.ErrMessageNotModified,
		tg.ErrMessageCantBeEdited,
		tg.ErrMessageCantBeDeleted,
		tg.ErrMessageTooOld,
		tg.ErrBotBlocked,
		tg.ErrBotKicked,
		tg.ErrChatNotFound,
		tg.ErrUserDeactivated,
		tg.ErrNoRights,
		tg.ErrCallbackExpired,
		tg.ErrInvalidCallbackData,
		tg.ErrRateLimited,
		tg.ErrCircuitOpen,
		tg.ErrMaxRetries,
		tg.ErrResponseTooLarge,
		tg.ErrInvalidToken,
		tg.ErrPathTraversal,
		tg.ErrInvalidConfig,
	}

	for i, err1 := range sentinels {
		for j, err2 := range sentinels {
			if i != j {
				assert.NotEqual(t, err1, err2, "sentinel errors should be distinct: %v and %v", err1, err2)
				assert.False(t, errors.Is(err1, err2), "errors.Is should return false for different sentinels")
			}
		}
	}
}
