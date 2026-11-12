package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ApproveSuggestedPostRequest represents an approveSuggestedPost request (Bot API 9.2).
type ApproveSuggestedPostRequest struct {
	ChatID    int64 `json:"chat_id"`    // the channel direct messages chat (an integer id only)
	MessageID int   `json:"message_id"` // the suggested post
	// SendDate is when the post is to be published (Unix time); 0 leaves it out, for a post that already has a
	// date. The server checks it: not more than 2678400 seconds ahead (the spec calls it 30 days).
	SendDate int64 `json:"send_date,omitempty"`
}

// DeclineSuggestedPostRequest represents a declineSuggestedPost request (Bot API 9.2).
type DeclineSuggestedPostRequest struct {
	ChatID    int64 `json:"chat_id"`    // the channel direct messages chat (an integer id only)
	MessageID int   `json:"message_id"` // the suggested post
	// Comment is for the creator of the post; "" leaves it out. It is sent as given; the server checks its
	// length (0-128 characters).
	Comment string `json:"comment,omitempty"`
}

// ApproveSuggestedPost approves a suggested post in a channel direct messages chat (Bot API 9.2); see
// tg.Message.SuggestedPostInfo. The bot needs the can_post_messages right in the channel. It is not retried.
func (c *Client) ApproveSuggestedPost(ctx context.Context, req ApproveSuggestedPostRequest) error {
	if err := validateSuggestedPostIDs(req.ChatID, req.MessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "approveSuggestedPost", req, nil, extractChatID(req.ChatID))
}

// DeclineSuggestedPost declines a suggested post in a channel direct messages chat (Bot API 9.2); see
// tg.Message.SuggestedPostInfo. The bot needs the can_manage_direct_messages right in the channel. It is not
// retried.
func (c *Client) DeclineSuggestedPost(ctx context.Context, req DeclineSuggestedPostRequest) error {
	if err := validateSuggestedPostIDs(req.ChatID, req.MessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "declineSuggestedPost", req, nil, extractChatID(req.ChatID))
}

// validateSuggestedPostIDs checks the ids of approveSuggestedPost and declineSuggestedPost.
func validateSuggestedPostIDs(chatID int64, messageID int) error {
	if chatID == 0 {
		return tg.NewValidationError("chat_id", "required")
	}
	if messageID <= 0 {
		return tg.NewValidationError("message_id", "must be positive")
	}
	return nil
}
