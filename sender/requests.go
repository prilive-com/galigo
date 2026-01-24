package sender

import (
	"github.com/prilive-com/galigo/tg"
)

// SendMessageRequest represents a request to send a text message.
type SendMessageRequest struct {
	ChatID                tg.ChatID   `json:"chat_id"`
	Text                  string      `json:"text"`
	ParseMode             tg.ParseMode `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool        `json:"disable_web_page_preview,omitempty"`
	DisableNotification   bool        `json:"disable_notification,omitempty"`
	ProtectContent        bool        `json:"protect_content,omitempty"`
	ReplyToMessageID      int         `json:"reply_to_message_id,omitempty"`
	ReplyMarkup           any         `json:"reply_markup,omitempty"`
}

// SendPhotoRequest represents a request to send a photo.
type SendPhotoRequest struct {
	ChatID              tg.ChatID   `json:"chat_id"`
	Photo               string      `json:"photo"` // URL, file_id, or file path
	Caption             string      `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool        `json:"disable_notification,omitempty"`
	ProtectContent      bool        `json:"protect_content,omitempty"`
	ReplyToMessageID    int         `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any         `json:"reply_markup,omitempty"`
}

// EditMessageTextRequest represents a request to edit message text.
type EditMessageTextRequest struct {
	ChatID                tg.ChatID   `json:"chat_id,omitempty"`
	MessageID             int         `json:"message_id,omitempty"`
	InlineMessageID       string      `json:"inline_message_id,omitempty"`
	Text                  string      `json:"text"`
	ParseMode             tg.ParseMode `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool        `json:"disable_web_page_preview,omitempty"`
	ReplyMarkup           any         `json:"reply_markup,omitempty"`
}

// EditMessageCaptionRequest represents a request to edit message caption.
type EditMessageCaptionRequest struct {
	ChatID          tg.ChatID   `json:"chat_id,omitempty"`
	MessageID       int         `json:"message_id,omitempty"`
	InlineMessageID string      `json:"inline_message_id,omitempty"`
	Caption         string      `json:"caption,omitempty"`
	ParseMode       tg.ParseMode `json:"parse_mode,omitempty"`
	ReplyMarkup     any         `json:"reply_markup,omitempty"`
}

// EditMessageReplyMarkupRequest represents a request to edit message markup.
type EditMessageReplyMarkupRequest struct {
	ChatID          tg.ChatID `json:"chat_id,omitempty"`
	MessageID       int       `json:"message_id,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	ReplyMarkup     any       `json:"reply_markup,omitempty"`
}

// DeleteMessageRequest represents a request to delete a message.
type DeleteMessageRequest struct {
	ChatID    tg.ChatID `json:"chat_id"`
	MessageID int       `json:"message_id"`
}

// ForwardMessageRequest represents a request to forward a message.
type ForwardMessageRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	FromChatID          tg.ChatID `json:"from_chat_id"`
	MessageID           int       `json:"message_id"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
}

// CopyMessageRequest represents a request to copy a message.
type CopyMessageRequest struct {
	ChatID              tg.ChatID   `json:"chat_id"`
	FromChatID          tg.ChatID   `json:"from_chat_id"`
	MessageID           int         `json:"message_id"`
	Caption             string      `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool        `json:"disable_notification,omitempty"`
	ProtectContent      bool        `json:"protect_content,omitempty"`
	ReplyToMessageID    int         `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any         `json:"reply_markup,omitempty"`
}

// AnswerCallbackQueryRequest represents a request to answer a callback query.
type AnswerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
	URL             string `json:"url,omitempty"`
	CacheTime       int    `json:"cache_time,omitempty"`
}

// MessageResult represents the result of sending a message.
type MessageResult struct {
	MessageID int `json:"message_id"`
}

// Response represents a generic API response.
type Response struct {
	OK          bool   `json:"ok"`
	Result      any    `json:"result,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}
