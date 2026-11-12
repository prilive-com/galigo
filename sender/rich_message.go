package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// SendRichMessageRequest represents a sendRichMessage request (Bot API 10.1).
type SendRichMessageRequest struct {
	BusinessConnectionID  string              `json:"business_connection_id,omitempty"`
	ChatID                tg.ChatID           `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	DirectMessagesTopicID int                 `json:"direct_messages_topic_id,omitempty"`
	RichMessage           tg.InputRichMessage `json:"rich_message"`
	DisableNotification   bool                `json:"disable_notification,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	AllowPaidBroadcast    bool                `json:"allow_paid_broadcast,omitempty"`
	MessageEffectID       string              `json:"message_effect_id,omitempty"`
	ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           any                 `json:"reply_markup,omitempty"`

	// Bot API 9.2: channel direct messages chats and suggested posts (suggested
	// posts are refused in any other chat).
	SuggestedPostParameters *tg.SuggestedPostParameters `json:"suggested_post_parameters,omitempty"`
}

// SendRichMessage sends a rich message (Bot API 10.1). It is not retried: a resend could post the message twice.
func (c *Client) SendRichMessage(ctx context.Context, req SendRichMessageRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if err := validateInputRichMessage(req.RichMessage); err != nil {
		return nil, err
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendRichMessage", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendRichMessageDraftRequest represents a sendRichMessageDraft request (Bot API 10.1).
type SendRichMessageDraftRequest struct {
	ChatID          tg.ChatID           `json:"chat_id"` // validated integer-only (private chats)
	MessageThreadID int                 `json:"message_thread_id,omitempty"`
	DraftID         int                 `json:"draft_id"`
	RichMessage     tg.InputRichMessage `json:"rich_message"`
}

// SendRichMessageDraft streams a partial rich message to a user (Bot API 10.1). ChatID must be an integer (private
// chats only) and DraftID non-zero; drafts with the same DraftID are animated. The draft is a 30-second preview: send
// the final message with SendRichMessage. It is not retried.
func (c *Client) SendRichMessageDraft(ctx context.Context, req SendRichMessageDraftRequest) error {
	switch v := req.ChatID.(type) {
	case int64:
		if v == 0 {
			return tg.NewValidationError("chat_id", "must be non-zero")
		}
	case int:
		if v == 0 {
			return tg.NewValidationError("chat_id", "must be non-zero")
		}
	case nil:
		return tg.NewValidationError("chat_id", "required")
	default:
		return tg.NewValidationError("chat_id", "must be integer for sendRichMessageDraft (private chats only)")
	}
	if req.DraftID == 0 {
		return tg.NewValidationError("draft_id", "must be non-zero")
	}
	if err := validateInputRichMessage(req.RichMessage); err != nil {
		return err
	}

	return c.callJSON(ctx, "sendRichMessageDraft", req, nil, extractChatID(req.ChatID))
}

// validateInputRichMessage checks the spec rule "exactly one of the fields html, markdown, or blocks" (Bot API 10.2).
// An empty string or an empty blocks slice counts as not used; it is not sent.
func validateInputRichMessage(m tg.InputRichMessage) error {
	used := 0
	if m.HTML != "" {
		used++
	}
	if m.Markdown != "" {
		used++
	}
	if len(m.Blocks) > 0 {
		used++
	}
	switch {
	case used == 0:
		return tg.NewValidationError("rich_message", "html, markdown or blocks required")
	case used > 1:
		return tg.NewValidationError("rich_message", "use only one of html, markdown, blocks")
	}
	return nil
}
