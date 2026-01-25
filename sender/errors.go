package sender

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors
var (
	// API errors
	ErrUnauthorized    = errors.New("galigo/sender: unauthorized")
	ErrForbidden       = errors.New("galigo/sender: forbidden")
	ErrNotFound        = errors.New("galigo/sender: not found")
	ErrTooManyRequests = errors.New("galigo/sender: too many requests")

	// Message errors
	ErrMessageNotFound      = errors.New("galigo/sender: message not found")
	ErrMessageNotModified   = errors.New("galigo/sender: message not modified")
	ErrMessageCantBeEdited  = errors.New("galigo/sender: message can't be edited")
	ErrMessageCantBeDeleted = errors.New("galigo/sender: message can't be deleted")
	ErrMessageTooOld        = errors.New("galigo/sender: message too old")

	// Chat/User errors
	ErrBotBlocked      = errors.New("galigo/sender: bot blocked by user")
	ErrBotKicked       = errors.New("galigo/sender: bot kicked from chat")
	ErrChatNotFound    = errors.New("galigo/sender: chat not found")
	ErrUserDeactivated = errors.New("galigo/sender: user deactivated")
	ErrNoRights        = errors.New("galigo/sender: not enough rights")

	// Callback errors
	ErrCallbackExpired     = errors.New("galigo/sender: callback query expired")
	ErrInvalidCallbackData = errors.New("galigo/sender: invalid callback data")

	// Client errors
	ErrRateLimited      = errors.New("galigo/sender: rate limited")
	ErrCircuitOpen      = errors.New("galigo/sender: circuit breaker open")
	ErrMaxRetries       = errors.New("galigo/sender: max retries exceeded")
	ErrResponseTooLarge = errors.New("galigo/sender: response too large")

	// Validation errors
	ErrInvalidToken = errors.New("galigo/sender: invalid token")
	ErrPathTraversal = errors.New("galigo/sender: path traversal attempt")
)

// APIError represents an error response from Telegram API.
type APIError struct {
	Code        int
	Description string
	RetryAfter  time.Duration
	Method      string
	cause       error
}

func (e *APIError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("galigo/sender: %s failed: %s (code=%d, retry_after=%s)",
			e.Method, e.Description, e.Code, e.RetryAfter)
	}
	return fmt.Sprintf("galigo/sender: %s failed: %s (code=%d)", e.Method, e.Description, e.Code)
}

// Unwrap returns the underlying sentinel error.
func (e *APIError) Unwrap() error { return e.cause }

// IsRetryable returns true if the error may succeed on retry.
func (e *APIError) IsRetryable() bool {
	return e.Code == 429 || (e.Code >= 500 && e.Code <= 504)
}

// NewAPIError creates an APIError with automatic sentinel detection.
func NewAPIError(method string, code int, description string) *APIError {
	return &APIError{
		Code:        code,
		Description: description,
		Method:      method,
		cause:       detectSentinel(code, description),
	}
}

// NewAPIErrorWithRetry creates an APIError with retry information.
func NewAPIErrorWithRetry(method string, code int, description string, retryAfter time.Duration) *APIError {
	return &APIError{
		Code:        code,
		Description: description,
		Method:      method,
		RetryAfter:  retryAfter,
		cause:       detectSentinel(code, description),
	}
}

// detectSentinel maps Telegram error codes/descriptions to sentinel errors.
// Description-based detection is prioritized over HTTP status codes for more specific errors.
func detectSentinel(code int, desc string) error {
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

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("galigo/sender: validation: %s - %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}
