package sender

import (
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// validateChatID validates a ChatID value.
// Returns nil if valid, error if invalid.
func validateChatID(id tg.ChatID) error {
	if id == nil {
		return fmt.Errorf("galigo: chat_id is required")
	}
	switch v := id.(type) {
	case int64:
		if v == 0 {
			return fmt.Errorf("galigo: chat_id cannot be zero")
		}
		return nil
	case int:
		if v == 0 {
			return fmt.Errorf("galigo: chat_id cannot be zero")
		}
		return nil
	case string:
		if v == "" {
			return fmt.Errorf("galigo: chat_id cannot be empty string")
		}
		return nil
	default:
		return fmt.Errorf("galigo: chat_id must be int64, int, or string, got %T", id)
	}
}

// validateUserID validates a user ID.
func validateUserID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("galigo: user_id must be positive, got %d", id)
	}
	return nil
}

// validateMessageID validates a message ID.
func validateMessageID(id int) error {
	if id <= 0 {
		return fmt.Errorf("galigo: message_id must be positive, got %d", id)
	}
	return nil
}

// validateThreadID validates a forum topic thread ID.
func validateThreadID(id int) error {
	if id <= 0 {
		return fmt.Errorf("galigo: message_thread_id must be positive, got %d", id)
	}
	return nil
}

// validateMessageIDs validates a slice of message IDs for bulk operations.
func validateMessageIDs(ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("galigo: message_ids cannot be empty")
	}
	if len(ids) > 100 {
		return fmt.Errorf("galigo: message_ids cannot exceed 100 messages, got %d", len(ids))
	}
	for i, id := range ids {
		if id <= 0 {
			return fmt.Errorf("galigo: message_ids[%d] must be positive, got %d", i, id)
		}
	}
	return nil
}
