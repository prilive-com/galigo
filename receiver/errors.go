package receiver

import (
	"errors"
	"fmt"
)

// Sentinel errors
var (
	ErrAlreadyRunning     = errors.New("galigo/receiver: already running")
	ErrNotRunning         = errors.New("galigo/receiver: not running")
	ErrTokenRequired      = errors.New("galigo/receiver: bot token required")
	ErrWebhookURLRequired = errors.New("galigo/receiver: webhook URL required for auto-registration")
	ErrTLSRequired        = errors.New("galigo/receiver: TLS cert and key required for webhook")

	// Webhook errors
	ErrForbidden        = errors.New("galigo/receiver: forbidden")
	ErrUnauthorized     = errors.New("galigo/receiver: unauthorized")
	ErrMethodNotAllowed = errors.New("galigo/receiver: method not allowed")
	ErrChannelBlocked   = errors.New("galigo/receiver: updates channel full")
	ErrRateLimited      = errors.New("galigo/receiver: rate limited")
)

// WebhookError represents an HTTP error response.
type WebhookError struct {
	Code    int
	Message string
	Err     error
}

func (e *WebhookError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("webhook error %d: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("webhook error %d: %s", e.Code, e.Message)
}

func (e *WebhookError) Unwrap() error {
	return e.Err
}

// APIError represents a Telegram API error.
type APIError struct {
	Code        int
	Description string
	Err         error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("telegram API error %d: %s: %v", e.Code, e.Description, e.Err)
	}
	return fmt.Sprintf("telegram API error %d: %s", e.Code, e.Description)
}

func (e *APIError) Unwrap() error {
	return e.Err
}
