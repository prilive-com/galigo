package sender

import (
	"context"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// ================== Inline & Other Request Types ==================

// AnswerInlineQueryRequest represents an answerInlineQuery request.
type AnswerInlineQueryRequest struct {
	InlineQueryID string                       `json:"inline_query_id"`
	Results       []tg.InlineQueryResult       `json:"results"`
	CacheTime     int                          `json:"cache_time,omitempty"`
	IsPersonal    bool                         `json:"is_personal,omitempty"`
	NextOffset    string                       `json:"next_offset,omitempty"`
	Button        *tg.InlineQueryResultsButton `json:"button,omitempty"`
}

// AnswerWebAppQueryRequest represents an answerWebAppQuery request.
type AnswerWebAppQueryRequest struct {
	WebAppQueryID string               `json:"web_app_query_id"`
	Result        tg.InlineQueryResult `json:"result"`
}

// AnswerGuestQueryRequest represents an answerGuestQuery request (Bot API 10.0).
type AnswerGuestQueryRequest struct {
	GuestQueryID string               `json:"guest_query_id"`
	Result       tg.InlineQueryResult `json:"result"`
}

// SendChecklistRequest represents a sendChecklist request.
type SendChecklistRequest struct {
	ChatID              tg.ChatID           `json:"chat_id"`
	Checklist           tg.InputChecklist   `json:"checklist"`
	MessageThreadID     int                 `json:"message_thread_id,omitempty"`
	DisableNotification bool                `json:"disable_notification,omitempty"`
	ProtectContent      bool                `json:"protect_content,omitempty"`
	ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
}

// EditMessageChecklistRequest represents an editMessageChecklist request.
type EditMessageChecklistRequest struct {
	ChatID    tg.ChatID         `json:"chat_id"`
	MessageID int               `json:"message_id"`
	Checklist tg.InputChecklist `json:"checklist"`
}

// ================== Inline & Other Methods ==================

// AnswerInlineQuery sends answers to an inline query.
func (c *Client) AnswerInlineQuery(ctx context.Context, req AnswerInlineQueryRequest) error {
	if req.InlineQueryID == "" {
		return tg.NewValidationError("inline_query_id", "required")
	}
	if len(req.Results) == 0 {
		return tg.NewValidationError("results", "at least one result required")
	}

	return c.callJSON(ctx, "answerInlineQuery", req, nil)
}

// AnswerWebAppQuery sets the result of an interaction with a Web App.
func (c *Client) AnswerWebAppQuery(ctx context.Context, req AnswerWebAppQueryRequest) (*tg.SentWebAppMessage, error) {
	if req.WebAppQueryID == "" {
		return nil, tg.NewValidationError("web_app_query_id", "required")
	}
	if req.Result == nil {
		return nil, tg.NewValidationError("result", "required")
	}

	var result tg.SentWebAppMessage
	if err := c.callJSON(ctx, "answerWebAppQuery", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AnswerGuestQuery sends a message in response to a guest query (Bot API 10.0); see tg.Message.GuestQueryID.
// It is not retried: a repeat could post the message twice. An empty inline_message_id in the reply (a null result
// included) is an error. Any error other than a 4xx *tg.APIError or a *tg.ValidationError (an empty
// inline_message_id and a 5xx *tg.APIError included) may come after the message was sent: do not repeat the call
// blindly.
func (c *Client) AnswerGuestQuery(ctx context.Context, req AnswerGuestQueryRequest) (*tg.SentGuestMessage, error) {
	if req.GuestQueryID == "" {
		return nil, tg.NewValidationError("guest_query_id", "required")
	}
	if req.Result == nil {
		return nil, tg.NewValidationError("result", "required")
	}

	var result tg.SentGuestMessage
	if err := c.callJSON(ctx, "answerGuestQuery", req, &result); err != nil {
		return nil, err
	}
	if result.InlineMessageID == "" {
		return nil, fmt.Errorf("galigo: answerGuestQuery: empty inline_message_id in response")
	}
	return &result, nil
}

// SendChecklist sends a checklist message.
func (c *Client) SendChecklist(ctx context.Context, req SendChecklistRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.Checklist.Title == "" {
		return nil, tg.NewValidationError("checklist.title", "required")
	}
	if len(req.Checklist.Tasks) == 0 {
		return nil, tg.NewValidationError("checklist.tasks", "at least one task required")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendChecklist", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditMessageChecklist edits a checklist message.
func (c *Client) EditMessageChecklist(ctx context.Context, req EditMessageChecklistRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.MessageID <= 0 {
		return nil, tg.NewValidationError("message_id", "must be positive")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "editMessageChecklist", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}
