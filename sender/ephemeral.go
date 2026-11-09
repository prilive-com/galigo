package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// EditEphemeralMessageTextRequest represents an editEphemeralMessageText request (Bot API 10.2).
type EditEphemeralMessageTextRequest struct {
	ChatID             tg.ChatID                `json:"chat_id"`
	ReceiverUserID     int64                    `json:"receiver_user_id"`
	EphemeralMessageID int                      `json:"ephemeral_message_id"`
	Text               string                   `json:"text"`
	ParseMode          tg.ParseMode             `json:"parse_mode,omitempty"`
	Entities           []tg.MessageEntity       `json:"entities,omitempty"`
	LinkPreviewOptions *tg.LinkPreviewOptions   `json:"link_preview_options,omitempty"`
	ReplyMarkup        *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// EditEphemeralMessageMediaRequest represents an editEphemeralMessageMedia request (Bot API 10.2).
// A Media with an uploaded file (InputFile with a Reader or Source) is sent as a file part. A file_id or URL works
// with Bot API 10.2; an upload needs a 10.3 server.
type EditEphemeralMessageMediaRequest struct {
	ChatID             tg.ChatID                `json:"chat_id"`
	ReceiverUserID     int64                    `json:"receiver_user_id"`
	EphemeralMessageID int                      `json:"ephemeral_message_id"`
	Media              InputMedia               `json:"media"`
	ReplyMarkup        *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// EditEphemeralMessageCaptionRequest represents an editEphemeralMessageCaption request (Bot API 10.2).
type EditEphemeralMessageCaptionRequest struct {
	ChatID             tg.ChatID                `json:"chat_id"`
	ReceiverUserID     int64                    `json:"receiver_user_id"`
	EphemeralMessageID int                      `json:"ephemeral_message_id"`
	Caption            string                   `json:"caption,omitempty"`
	ParseMode          tg.ParseMode             `json:"parse_mode,omitempty"`
	CaptionEntities    []tg.MessageEntity       `json:"caption_entities,omitempty"`
	ReplyMarkup        *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// EditEphemeralMessageReplyMarkupRequest represents an editEphemeralMessageReplyMarkup request (Bot API 10.2).
type EditEphemeralMessageReplyMarkupRequest struct {
	ChatID             tg.ChatID                `json:"chat_id"`
	ReceiverUserID     int64                    `json:"receiver_user_id"`
	EphemeralMessageID int                      `json:"ephemeral_message_id"`
	ReplyMarkup        *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// DeleteEphemeralMessageRequest represents a deleteEphemeralMessage request (Bot API 10.2).
type DeleteEphemeralMessageRequest struct {
	ChatID             tg.ChatID `json:"chat_id"`
	ReceiverUserID     int64     `json:"receiver_user_id"`
	EphemeralMessageID int       `json:"ephemeral_message_id"`
}

// validateEphemeralTarget checks the three identifiers every ephemeral message method needs.
// The spec gives no sign rule for ephemeral_message_id, so only 0 (unset) is refused.
func validateEphemeralTarget(chatID tg.ChatID, receiverUserID int64, ephemeralMessageID int) error {
	if err := validateChatID(chatID); err != nil {
		return err
	}
	if receiverUserID <= 0 {
		return tg.NewValidationError("receiver_user_id", "must be positive")
	}
	if ephemeralMessageID == 0 {
		return tg.NewValidationError("ephemeral_message_id", "required")
	}
	return nil
}

// EditEphemeralMessageText edits the text of an ephemeral message (Bot API 10.2); see
// tg.Message.EphemeralMessageID. It is not retried.
func (c *Client) EditEphemeralMessageText(ctx context.Context, req EditEphemeralMessageTextRequest) error {
	if err := validateEphemeralTarget(req.ChatID, req.ReceiverUserID, req.EphemeralMessageID); err != nil {
		return err
	}
	if err := req.LinkPreviewOptions.Validate(); err != nil {
		return err
	}
	return c.callJSON(ctx, "editEphemeralMessageText", req, nil, extractChatID(req.ChatID))
}

// EditEphemeralMessageMedia edits the media of an ephemeral message (Bot API 10.2). It is sent as
// multipart/form-data. It is not retried.
func (c *Client) EditEphemeralMessageMedia(ctx context.Context, req EditEphemeralMessageMediaRequest) error {
	if err := validateEphemeralTarget(req.ChatID, req.ReceiverUserID, req.EphemeralMessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "editEphemeralMessageMedia", req, nil, extractChatID(req.ChatID))
}

// EditEphemeralMessageCaption edits the caption of an ephemeral message (Bot API 10.2). It is not retried.
func (c *Client) EditEphemeralMessageCaption(ctx context.Context, req EditEphemeralMessageCaptionRequest) error {
	if err := validateEphemeralTarget(req.ChatID, req.ReceiverUserID, req.EphemeralMessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "editEphemeralMessageCaption", req, nil, extractChatID(req.ChatID))
}

// EditEphemeralMessageReplyMarkup edits the inline keyboard of an ephemeral message (Bot API 10.2).
// It is not retried.
func (c *Client) EditEphemeralMessageReplyMarkup(ctx context.Context, req EditEphemeralMessageReplyMarkupRequest) error {
	if err := validateEphemeralTarget(req.ChatID, req.ReceiverUserID, req.EphemeralMessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "editEphemeralMessageReplyMarkup", req, nil, extractChatID(req.ChatID))
}

// DeleteEphemeralMessage deletes an ephemeral message (Bot API 10.2). It is not retried.
func (c *Client) DeleteEphemeralMessage(ctx context.Context, req DeleteEphemeralMessageRequest) error {
	if err := validateEphemeralTarget(req.ChatID, req.ReceiverUserID, req.EphemeralMessageID); err != nil {
		return err
	}
	return c.callJSON(ctx, "deleteEphemeralMessage", req, nil, extractChatID(req.ChatID))
}
