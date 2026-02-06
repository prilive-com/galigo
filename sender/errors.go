package sender

import (
	"github.com/prilive-com/galigo/tg"
)

// Type aliases for backward compatibility.
// These types are identical to the canonical types in tg package.

// APIError represents an error response from Telegram API.
//
// Deprecated: Use tg.APIError instead. Will be removed in v2.0.
type APIError = tg.APIError

// ValidationError represents a validation error.
//
// Deprecated: Use tg.ValidationError instead. Will be removed in v2.0.
type ValidationError = tg.ValidationError

// ResponseParameters contains information about why a request was unsuccessful.
//
// Deprecated: Use tg.ResponseParameters instead. Will be removed in v2.0.
type ResponseParameters = tg.ResponseParameters

// Sentinel error aliases for backward compatibility.
//
// Deprecated: Use tg.Err* instead. Will be removed in v2.0.
var (
	// API errors
	ErrUnauthorized    = tg.ErrUnauthorized
	ErrForbidden       = tg.ErrForbidden
	ErrNotFound        = tg.ErrNotFound
	ErrTooManyRequests = tg.ErrTooManyRequests

	// Message errors
	ErrMessageNotFound      = tg.ErrMessageNotFound
	ErrMessageNotModified   = tg.ErrMessageNotModified
	ErrMessageCantBeEdited  = tg.ErrMessageCantBeEdited
	ErrMessageCantBeDeleted = tg.ErrMessageCantBeDeleted
	ErrMessageTooOld        = tg.ErrMessageTooOld

	// Chat/User errors
	ErrBotBlocked      = tg.ErrBotBlocked
	ErrBotKicked       = tg.ErrBotKicked
	ErrChatNotFound    = tg.ErrChatNotFound
	ErrUserDeactivated = tg.ErrUserDeactivated
	ErrNoRights        = tg.ErrNoRights

	// Callback errors
	ErrCallbackExpired     = tg.ErrCallbackExpired
	ErrInvalidCallbackData = tg.ErrInvalidCallbackData

	// Client errors
	ErrRateLimited      = tg.ErrRateLimited
	ErrCircuitOpen      = tg.ErrCircuitOpen
	ErrMaxRetries       = tg.ErrMaxRetries
	ErrResponseTooLarge = tg.ErrResponseTooLarge

	// Validation errors
	ErrInvalidToken  = tg.ErrInvalidToken
	ErrPathTraversal = tg.ErrPathTraversal
)

// NewAPIError creates an APIError with automatic sentinel detection.
//
// Deprecated: Use tg.NewAPIError instead. Will be removed in v2.0.
var NewAPIError = tg.NewAPIError

// NewAPIErrorWithRetry creates an APIError with retry information.
//
// Deprecated: Use tg.NewAPIErrorWithRetry instead. Will be removed in v2.0.
var NewAPIErrorWithRetry = tg.NewAPIErrorWithRetry

// NewValidationError creates a new ValidationError.
//
// Deprecated: Use tg.NewValidationError instead. Will be removed in v2.0.
var NewValidationError = tg.NewValidationError
