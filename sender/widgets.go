package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// SendWidgetRequest represents a sendWidget request (Bot API 10.4). galigo checks only that ChatID and HTML are set;
// the HTML size (1-32768 bytes, at most 4096 bytes after zlib), the data sizes (0-1024 bytes) and the HTML itself are
// left to Telegram.
type SendWidgetRequest struct {
	ChatID                tg.ChatID           `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	HTML                  string              `json:"html"`
	Width                 int                 `json:"width,omitempty"`
	Height                int                 `json:"height,omitempty"`
	PublicData            string              `json:"public_data,omitempty"`
	ChatData              string              `json:"chat_data,omitempty"`
	Caption               string              `json:"caption,omitempty"`
	ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
	CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
	AllowQueries          bool                `json:"allow_queries,omitempty"`
	IsShared              bool                `json:"is_shared,omitempty"` // a shared widget; send it again with SendSharedWidget
	DisableNotification   bool                `json:"disable_notification,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	AllowPaidBroadcast    bool                `json:"allow_paid_broadcast,omitempty"`
	MessageEffectID       string              `json:"message_effect_id,omitempty"`
	ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           any                 `json:"reply_markup,omitempty"`
}

// SendSharedWidgetRequest represents a sendSharedWidget request (Bot API 10.4): it sends again a shared widget
// created by the bot. galigo checks only that ChatID and WidgetID are set.
type SendSharedWidgetRequest struct {
	ChatID                tg.ChatID           `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	WidgetID              string              `json:"widget_id"`
	ChatData              string              `json:"chat_data,omitempty"`
	Caption               string              `json:"caption,omitempty"`
	ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
	CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
	DisableNotification   bool                `json:"disable_notification,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	AllowPaidBroadcast    bool                `json:"allow_paid_broadcast,omitempty"`
	MessageEffectID       string              `json:"message_effect_id,omitempty"`
	ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           any                 `json:"reply_markup,omitempty"`
}

// AnswerWidgetQueryRequest represents an answerWidgetQuery request (Bot API 10.4); see tg.WidgetQuery and
// tg.SharedWidgetQuery.
type AnswerWidgetQueryRequest struct {
	WidgetQueryID string `json:"widget_query_id"`
	Data          string `json:"data,omitempty"` // returned to the widget, 0-1024 bytes; "" sends no data
}

// EditMessageWidgetDataRequest represents an editMessageWidgetData request (Bot API 10.4). Set either ChatID and
// MessageID, or InlineMessageID. PublicData and ChatData have three states: nil leaves the value unchanged (the
// key is not sent), a pointer to "" removes it, and any other value sets it.
type EditMessageWidgetDataRequest struct {
	ChatID          tg.ChatID `json:"chat_id,omitempty"`
	MessageID       int       `json:"message_id,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	PublicData      *string   `json:"public_data,omitempty"`
	ChatData        *string   `json:"chat_data,omitempty"`
}

// EditSharedWidgetDataRequest represents an editSharedWidgetData request (Bot API 10.4). PublicData is always sent:
// "" removes the public data. CacheTime 0 leaves out cache_time, so the current value is kept (30 by default); the
// server takes 5-3600, which galigo does not check.
type EditSharedWidgetDataRequest struct {
	WidgetID   string `json:"widget_id"`
	PublicData string `json:"public_data"`
	CacheTime  int    `json:"cache_time,omitempty"`
}

// SendWidget sends a message with an HTML widget (Bot API 10.4). Currently, in the production environment, widgets
// can be sent only to private chats, and sent widgets automatically have protected content and therefore can't be
// forwarded. It is not retried: a repeat could post the widget twice. ChatID is checked first (the plain galigo
// chat_id errors), then HTML must be non-empty (*tg.ValidationError).
func (c *Client) SendWidget(ctx context.Context, req SendWidgetRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.HTML == "" {
		return nil, tg.NewValidationError("html", "required")
	}
	var result tg.Message
	if err := c.callJSON(ctx, "sendWidget", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendSharedWidget sends again a shared widget created by the bot (Bot API 10.4). Currently, in the production
// environment, widgets can be sent only to private chats, and sent widgets automatically have protected content and
// therefore can't be forwarded. It is not retried: a repeat could post the widget twice. ChatID is checked first,
// then WidgetID must be non-empty.
func (c *Client) SendSharedWidget(ctx context.Context, req SendSharedWidgetRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.WidgetID == "" {
		return nil, tg.NewValidationError("widget_id", "required")
	}
	var result tg.Message
	if err := c.callJSON(ctx, "sendSharedWidget", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// AnswerWidgetQuery answers a query from a widget (Bot API 10.4), tg.WidgetQuery or tg.SharedWidgetQuery. The query
// must be answered within 15 seconds. It is not retried. An expired or unknown id gives "query is too old ...", which
// tg.DetectSentinel maps to tg.ErrCallbackExpired.
func (c *Client) AnswerWidgetQuery(ctx context.Context, req AnswerWidgetQueryRequest) error {
	if req.WidgetQueryID == "" {
		return tg.NewValidationError("widget_query_id", "required")
	}
	return c.callJSON(ctx, "answerWidgetQuery", req, nil)
}

// EditMessageWidgetData edits the data of a widget in a message sent by the bot or via the bot (Bot API 10.4). It
// is not retried. Set either InlineMessageID, or ChatID and MessageID: with InlineMessageID set, any non-nil ChatID
// (also "", 0 or a typed nil) and any non-zero MessageID is refused, so galigo never guesses which target is meant.
// When both data fields are nil, a successful call changes neither value. A missing message gives "message with
// widget to edit not found", which maps to no sentinel.
func (c *Client) EditMessageWidgetData(ctx context.Context, req EditMessageWidgetDataRequest) error {
	if req.InlineMessageID != "" {
		if req.ChatID != nil || req.MessageID != 0 {
			return tg.NewValidationError("inline_message_id", "cannot be combined with chat_id or message_id")
		}
		return c.callJSON(ctx, "editMessageWidgetData", req, nil)
	}
	if req.ChatID == nil && req.MessageID == 0 {
		return tg.NewValidationError("inline_message_id", "required if chat_id and message_id are not specified")
	}
	if err := validateChatID(req.ChatID); err != nil {
		return err
	}
	if err := validateMessageID(req.MessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "editMessageWidgetData", req, nil, extractChatID(req.ChatID))
}

// EditSharedWidgetData edits the public data of a shared widget created by the bot (Bot API 10.4). A shared widget
// can be edited at most once per second, so it is not retried.
func (c *Client) EditSharedWidgetData(ctx context.Context, req EditSharedWidgetDataRequest) error {
	if req.WidgetID == "" {
		return tg.NewValidationError("widget_id", "required")
	}
	return c.callJSON(ctx, "editSharedWidgetData", req, nil)
}
