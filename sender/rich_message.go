package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// STUB (B-25 10.1d red record): the types and methods exist, with no JSON tags and no behaviour.

// SendRichMessageRequest represents a sendRichMessage request (Bot API 10.1).
type SendRichMessageRequest struct {
	BusinessConnectionID  string
	ChatID                tg.ChatID
	MessageThreadID       int
	DirectMessagesTopicID int
	RichMessage           tg.InputRichMessage
	DisableNotification   bool
	ProtectContent        bool
	AllowPaidBroadcast    bool
	MessageEffectID       string
	ReplyParameters       *tg.ReplyParameters
	ReplyMarkup           any
}

// SendRichMessage sends a rich message (Bot API 10.1). STUB: no request.
func (c *Client) SendRichMessage(ctx context.Context, req SendRichMessageRequest) (*tg.Message, error) {
	return nil, nil
}

// SendRichMessageDraftRequest represents a sendRichMessageDraft request (Bot API 10.1).
type SendRichMessageDraftRequest struct {
	ChatID          tg.ChatID
	MessageThreadID int
	DraftID         int
	RichMessage     tg.InputRichMessage
}

// SendRichMessageDraft streams a partial rich message (Bot API 10.1). STUB: no request.
func (c *Client) SendRichMessageDraft(ctx context.Context, req SendRichMessageDraftRequest) error {
	return nil
}

// validateInputRichMessage STUB: accepts everything.
func validateInputRichMessage(m tg.InputRichMessage) error {
	return nil
}
