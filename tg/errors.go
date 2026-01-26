package tg

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors - use with errors.Is()
var (
	// API errors
	ErrUnauthorized    = errors.New("galigo: unauthorized (invalid token)")
	ErrForbidden       = errors.New("galigo: forbidden")
	ErrNotFound        = errors.New("galigo: not found")
	ErrTooManyRequests = errors.New("galigo: too many requests")

	// Message errors
	ErrMessageNotFound      = errors.New("galigo: message not found")
	ErrMessageNotModified   = errors.New("galigo: message not modified")
	ErrMessageCantBeEdited  = errors.New("galigo: message can't be edited")
	ErrMessageCantBeDeleted = errors.New("galigo: message can't be deleted")
	ErrMessageTooOld        = errors.New("galigo: message too old")

	// Chat/User errors
	ErrBotBlocked      = errors.New("galigo: bot blocked by user")
	ErrBotKicked       = errors.New("galigo: bot kicked from chat")
	ErrChatNotFound    = errors.New("galigo: chat not found")
	ErrUserDeactivated = errors.New("galigo: user deactivated")
	ErrNoRights        = errors.New("galigo: not enough rights")

	// Callback errors
	ErrCallbackExpired     = errors.New("galigo: callback query expired")
	ErrInvalidCallbackData = errors.New("galigo: invalid callback data")

	// Client errors
	ErrRateLimited      = errors.New("galigo: rate limit exceeded")
	ErrCircuitOpen      = errors.New("galigo: circuit breaker open")
	ErrMaxRetries       = errors.New("galigo: max retries exceeded")
	ErrResponseTooLarge = errors.New("galigo: response too large")

	// Validation errors
	ErrInvalidToken  = errors.New("galigo: invalid bot token format")
	ErrPathTraversal = errors.New("galigo: path traversal attempt")
	ErrInvalidConfig = errors.New("galigo: invalid configuration")
)

// ResponseParameters contains information about why a request was unsuccessful.
type ResponseParameters struct {
	MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
	RetryAfter      int   `json:"retry_after,omitempty"`
}

// APIError represents an error response from Telegram API.
// Use errors.As() to extract details, errors.Is() to match sentinels.
type APIError struct {
	Code        int
	Description string
	RetryAfter  time.Duration
	Method      string              // API method that failed
	Parameters  *ResponseParameters // Additional response parameters
	cause       error               // Underlying sentinel for errors.Is()
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("galigo: %s failed: %s (code=%d, retry_after=%s)",
			e.Method, e.Description, e.Code, e.RetryAfter)
	}
	return fmt.Sprintf("galigo: %s failed: %s (code=%d)", e.Method, e.Description, e.Code)
}

// Unwrap returns the underlying sentinel error for errors.Is() support.
func (e *APIError) Unwrap() error { return e.cause }

// IsRetryable returns true if the error is temporary and may succeed on retry.
func (e *APIError) IsRetryable() bool {
	return e.Code == 429 || (e.Code >= 500 && e.Code <= 504)
}

// NewAPIError creates an APIError with automatic sentinel detection.
func NewAPIError(method string, code int, description string) *APIError {
	return &APIError{
		Code:        code,
		Description: description,
		Method:      method,
		cause:       DetectSentinel(code, description),
	}
}

// NewAPIErrorWithRetry creates an APIError with retry information.
func NewAPIErrorWithRetry(method string, code int, description string, retryAfter time.Duration) *APIError {
	return &APIError{
		Code:        code,
		Description: description,
		Method:      method,
		RetryAfter:  retryAfter,
		cause:       DetectSentinel(code, description),
	}
}

// DetectSentinel maps Telegram error codes/descriptions to sentinel errors.
// Description-based detection is prioritized over HTTP status codes for more specific errors.
func DetectSentinel(code int, desc string) error {
	// Check description first for specific error messages
	descLower := strings.ToLower(desc)
	switch {
	case strings.Contains(descLower, "message is not modified"):
		return ErrMessageNotModified
	case strings.Contains(descLower, "message to edit not found"),
		strings.Contains(descLower, "message to delete not found"),
		strings.Contains(descLower, "message not found"):
		return ErrMessageNotFound
	case strings.Contains(descLower, "message can't be edited"):
		return ErrMessageCantBeEdited
	case strings.Contains(descLower, "message can't be deleted"):
		return ErrMessageCantBeDeleted
	case strings.Contains(descLower, "message is too old"):
		return ErrMessageTooOld
	case strings.Contains(descLower, "bot was blocked"):
		return ErrBotBlocked
	case strings.Contains(descLower, "bot was kicked"):
		return ErrBotKicked
	case strings.Contains(descLower, "chat not found"):
		return ErrChatNotFound
	case strings.Contains(descLower, "user is deactivated"):
		return ErrUserDeactivated
	case strings.Contains(descLower, "not enough rights"):
		return ErrNoRights
	case strings.Contains(descLower, "query is too old"):
		return ErrCallbackExpired
	case strings.Contains(descLower, "button_data_invalid"):
		return ErrInvalidCallbackData
	}

	// Fall back to generic HTTP status code sentinels
	switch code {
	case 401:
		return ErrUnauthorized
	case 403:
		return ErrForbidden
	case 404:
		return ErrNotFound
	case 429:
		return ErrTooManyRequests
	}

	return nil
}

// ValidationError represents a request validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("galigo: validation: %s - %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// ConfigError represents a configuration error.
type ConfigError struct {
	Key     string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("galigo: config: %s - %s", e.Key, e.Message)
}

// NewConfigError creates a new ConfigError.
func NewConfigError(key, message string) *ConfigError {
	return &ConfigError{Key: key, Message: message}
}
