package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// SendLivePhotoRequest represents a sendLivePhoto request (Bot API 10.0).
//
// LivePhoto is the video and Photo the static photo; pass each as a file id or
// an upload. The spec says sending by URL is currently unsupported, and that the
// video must be at most 10 seconds and 10 MB; the client checks neither (the
// server does). On 2026-11-05 the server accepted a URL pair and sent a plain
// photo without live_photo.
type SendLivePhotoRequest struct {
	BusinessConnectionID  string              `json:"business_connection_id,omitempty"`
	ChatID                tg.ChatID           `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	DirectMessagesTopicID int                 `json:"direct_messages_topic_id,omitempty"`
	LivePhoto             InputFile           `json:"live_photo"`
	Photo                 InputFile           `json:"photo"`
	Caption               string              `json:"caption,omitempty"`
	ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
	CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
	HasSpoiler            bool                `json:"has_spoiler,omitempty"`
	DisableNotification   bool                `json:"disable_notification,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	AllowPaidBroadcast    bool                `json:"allow_paid_broadcast,omitempty"`
	MessageEffectID       string              `json:"message_effect_id,omitempty"`
	ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           any                 `json:"reply_markup,omitempty"`
}

// SendLivePhoto sends a live photo. It is not retried: a resend could post the
// live photo twice.
func (c *Client) SendLivePhoto(ctx context.Context, req SendLivePhotoRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.LivePhoto.IsEmpty() {
		return nil, tg.NewValidationError("live_photo", "required")
	}
	if req.Photo.IsEmpty() {
		return nil, tg.NewValidationError("photo", "required")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendLivePhoto", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}
