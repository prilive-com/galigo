package sender

import (
	"context"

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

// SendChecklistRequest represents a sendChecklist request.
type SendChecklistRequest struct {
	ChatID              tg.ChatID           `json:"chat_id"`
	Checklist           tg.InputChecklist   `json:"checklist"`
	MessageThreadID     int                 `json:"message_thread_id,omitempty"`
	DisableNotification bool                `json:"disable_notification,omitempty"`
	ProtectContent      bool                `json:"protect_content,omitempty"`
	ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
}

// EditChecklistRequest represents an editChecklist request.
type EditChecklistRequest struct {
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

// EditChecklist edits a checklist message.
func (c *Client) EditChecklist(ctx context.Context, req EditChecklistRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.MessageID <= 0 {
		return nil, tg.NewValidationError("message_id", "must be positive")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "editChecklist", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}
