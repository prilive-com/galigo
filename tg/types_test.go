package tg_test

import (
	"testing"

	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
)

// ==================== Message.MessageSig ====================

func TestMessage_MessageSig_Valid(t *testing.T) {
	msg := &tg.Message{
		MessageID: 123,
		Chat:      &tg.Chat{ID: 456},
	}

	msgID, chatID := msg.MessageSig()
	assert.Equal(t, "123", msgID)
	assert.Equal(t, int64(456), chatID)
}

func TestMessage_MessageSig_NilMessage(t *testing.T) {
	var msg *tg.Message

	msgID, chatID := msg.MessageSig()
	assert.Equal(t, "", msgID)
	assert.Equal(t, int64(0), chatID)
}

func TestMessage_MessageSig_NilChat(t *testing.T) {
	msg := &tg.Message{
		MessageID: 123,
		Chat:      nil,
	}

	msgID, chatID := msg.MessageSig()
	assert.Equal(t, "123", msgID)
	assert.Equal(t, int64(0), chatID)
}

// ==================== StoredMessage.MessageSig ====================

func TestStoredMessage_MessageSig(t *testing.T) {
	stored := tg.StoredMessage{
		MsgID:  789,
		ChatID: 12345,
	}

	msgID, chatID := stored.MessageSig()
	assert.Equal(t, "789", msgID)
	assert.Equal(t, int64(12345), chatID)
}

func TestStoredMessage_ImplementsEditable(t *testing.T) {
	var _ tg.Editable = tg.StoredMessage{}
}

// ==================== InlineMessage.MessageSig ====================

func TestInlineMessage_MessageSig(t *testing.T) {
	inline := tg.InlineMessage{
		InlineMessageID: "inline_msg_123",
	}

	msgID, chatID := inline.MessageSig()
	assert.Equal(t, "inline_msg_123", msgID)
	assert.Equal(t, int64(0), chatID) // Inline messages have no chat ID
}

func TestInlineMessage_ImplementsEditable(t *testing.T) {
	var _ tg.Editable = tg.InlineMessage{}
}

// ==================== CallbackQuery.MessageSig ====================

func TestCallbackQuery_MessageSig_WithInlineMessageID(t *testing.T) {
	cb := &tg.CallbackQuery{
		ID:              "cb_123",
		InlineMessageID: "inline_456",
	}

	msgID, chatID := cb.MessageSig()
	assert.Equal(t, "inline_456", msgID)
	assert.Equal(t, int64(0), chatID)
}

func TestCallbackQuery_MessageSig_WithMessage(t *testing.T) {
	cb := &tg.CallbackQuery{
		ID: "cb_123",
		Message: &tg.Message{
			MessageID: 789,
			Chat:      &tg.Chat{ID: 100},
		},
	}

	msgID, chatID := cb.MessageSig()
	assert.Equal(t, "789", msgID)
	assert.Equal(t, int64(100), chatID)
}

func TestCallbackQuery_MessageSig_Nil(t *testing.T) {
	var cb *tg.CallbackQuery

	msgID, chatID := cb.MessageSig()
	assert.Equal(t, "", msgID)
	assert.Equal(t, int64(0), chatID)
}

func TestCallbackQuery_MessageSig_NoMessageNoInline(t *testing.T) {
	cb := &tg.CallbackQuery{
		ID: "cb_123",
	}

	msgID, chatID := cb.MessageSig()
	assert.Equal(t, "", msgID)
	assert.Equal(t, int64(0), chatID)
}

// ==================== Editable Interface Compliance ====================

func TestEditableInterface_AllTypesImplement(t *testing.T) {
	// Compile-time verification that types implement Editable
	editables := []tg.Editable{
		&tg.Message{},
		tg.StoredMessage{},
		tg.InlineMessage{},
		&tg.CallbackQuery{},
	}

	// All should be usable as Editable
	for _, e := range editables {
		msgID, chatID := e.MessageSig()
		_ = msgID
		_ = chatID
	}
}
