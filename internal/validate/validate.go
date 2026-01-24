package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// Error represents a validation error.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("validation: %s - %s", e.Field, e.Message)
}

// New creates a new validation error.
func New(field, message string) *Error {
	return &Error{Field: field, Message: message}
}

// Newf creates a new validation error with formatted message.
func Newf(field, format string, args ...any) *Error {
	return &Error{Field: field, Message: fmt.Sprintf(format, args...)}
}

// Token validates a Telegram bot token format.
// Format: {bot_id}:{secret} where bot_id is numeric.
func Token(token string) error {
	if token == "" {
		return New("token", "cannot be empty")
	}

	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return New("token", "invalid format, expected {bot_id}:{secret}")
	}

	botID := parts[0]
	secret := parts[1]

	// Bot ID must be numeric
	for _, c := range botID {
		if c < '0' || c > '9' {
			return New("token", "bot_id must be numeric")
		}
	}

	// Secret must not be empty
	if secret == "" {
		return New("token", "secret cannot be empty")
	}

	return nil
}

// ChatID validates a chat identifier.
// Valid: int64 (numeric ID) or string starting with @.
func ChatID(chatID any) error {
	if chatID == nil {
		return New("chat_id", "cannot be nil")
	}

	switch v := chatID.(type) {
	case int64:
		if v == 0 {
			return New("chat_id", "cannot be zero")
		}
	case int:
		if v == 0 {
			return New("chat_id", "cannot be zero")
		}
	case string:
		if v == "" {
			return New("chat_id", "cannot be empty")
		}
		if !strings.HasPrefix(v, "@") {
			return New("chat_id", "string chat_id must start with @")
		}
	default:
		return Newf("chat_id", "invalid type %T, expected int64 or string", chatID)
	}

	return nil
}

// Text validates message text.
func Text(text string, maxLen int) error {
	if text == "" {
		return New("text", "cannot be empty")
	}
	if len(text) > maxLen {
		return Newf("text", "exceeds maximum length of %d characters", maxLen)
	}
	return nil
}

// Caption validates media caption.
func Caption(caption string, maxLen int) error {
	if len(caption) > maxLen {
		return Newf("caption", "exceeds maximum length of %d characters", maxLen)
	}
	return nil
}

// CallbackData validates inline keyboard callback data.
func CallbackData(data string, maxLen int) error {
	if data == "" {
		return New("callback_data", "cannot be empty")
	}
	if len(data) > maxLen {
		return Newf("callback_data", "exceeds maximum length of %d bytes", maxLen)
	}
	return nil
}

// URL validates a URL string.
func URL(url string) error {
	if url == "" {
		return New("url", "cannot be empty")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return New("url", "must start with http:// or https://")
	}
	return nil
}

// WebhookURL validates a webhook URL (must be HTTPS).
func WebhookURL(url string) error {
	if url == "" {
		return New("url", "cannot be empty")
	}
	if !strings.HasPrefix(url, "https://") {
		return New("url", "webhook URL must use HTTPS")
	}
	return nil
}

// FileID validates a Telegram file ID.
func FileID(fileID string) error {
	if fileID == "" {
		return New("file_id", "cannot be empty")
	}
	return nil
}

// InlineQueryID validates an inline query ID.
func InlineQueryID(id string) error {
	if id == "" {
		return New("inline_query_id", "cannot be empty")
	}
	return nil
}

// CallbackQueryID validates a callback query ID.
func CallbackQueryID(id string) error {
	if id == "" {
		return New("callback_query_id", "cannot be empty")
	}
	return nil
}

// Username validates a Telegram username.
var usernameRegex = regexp.MustCompile(`^@?[a-zA-Z][a-zA-Z0-9_]{4,31}$`)

func Username(username string) error {
	if username == "" {
		return New("username", "cannot be empty")
	}
	if !usernameRegex.MatchString(username) {
		return New("username", "invalid format (5-32 alphanumeric characters, starting with letter)")
	}
	return nil
}

// Pagination validates pagination parameters.
func Pagination(offset, limit, maxLimit int) error {
	if offset < 0 {
		return New("offset", "cannot be negative")
	}
	if limit < 0 {
		return New("limit", "cannot be negative")
	}
	if limit > maxLimit {
		return Newf("limit", "exceeds maximum of %d", maxLimit)
	}
	return nil
}

// ParseMode validates a parse mode value.
func ParseMode(mode string) error {
	switch mode {
	case "", "HTML", "Markdown", "MarkdownV2":
		return nil
	default:
		return Newf("parse_mode", "invalid value %q, expected HTML, Markdown, or MarkdownV2", mode)
	}
}

// Positive validates that a value is positive.
func Positive(field string, value int) error {
	if value <= 0 {
		return Newf(field, "must be positive, got %d", value)
	}
	return nil
}

// NonNegative validates that a value is non-negative.
func NonNegative(field string, value int) error {
	if value < 0 {
		return Newf(field, "cannot be negative, got %d", value)
	}
	return nil
}

// InRange validates that a value is within a range.
func InRange(field string, value, min, max int) error {
	if value < min || value > max {
		return Newf(field, "must be between %d and %d, got %d", min, max, value)
	}
	return nil
}

// Required validates that a string is not empty.
func Required(field, value string) error {
	if value == "" {
		return Newf(field, "is required")
	}
	return nil
}

// MaxLength validates string length.
func MaxLength(field, value string, max int) error {
	if len(value) > max {
		return Newf(field, "exceeds maximum length of %d", max)
	}
	return nil
}
