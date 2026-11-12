package sender

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prilive-com/galigo/tg"
)

// InputPaidMedia is one item of sendPaidMedia's media list (Bot API 7.6; the live
// photo kind since 10.0). Use one of the constructors: NewInputPaidMediaPhoto,
// NewInputPaidMediaVideo, NewInputPaidMediaLivePhoto.
type InputPaidMedia struct {
	// Type is "photo", "video" or "live_photo".
	Type string

	// Media is the file to send: the photo, the video, or a live photo's video
	// (FileID, URL or an upload; the spec says a live photo cannot be sent by URL).
	// For a live photo, Media.Photo is its static photo (required; set by
	// NewInputPaidMediaLivePhoto). A set Media.Photo is sent as "photo" whatever
	// the Type; only a live photo uses it.
	Media InputFile

	// The fields below are for a video (optional). Each is sent when it is set,
	// whatever the Type; the client does not check the kind.

	// Thumbnail of the video. The spec says it is ignored unless the video is
	// uploaded, and that a thumbnail can only be uploaded as a new file.
	Thumbnail *InputFile

	// Cover for the video in the message (Bot API 8.3).
	Cover *InputFile

	// StartTimestamp is the start timestamp for the video in the message (Bot API 8.3).
	StartTimestamp int

	// Width, Height and Duration (in seconds) of the video.
	Width    int
	Height   int
	Duration int

	// SupportsStreaming is true if the uploaded video is suitable for streaming.
	SupportsStreaming bool
}

// NewInputPaidMediaPhoto creates an InputPaidMedia of type "photo".
func NewInputPaidMediaPhoto(media InputFile) InputPaidMedia {
	return InputPaidMedia{Type: "photo", Media: media}
}

// NewInputPaidMediaVideo creates an InputPaidMedia of type "video". Set the
// video fields on the result if needed.
func NewInputPaidMediaVideo(media InputFile) InputPaidMedia {
	return InputPaidMedia{Type: "video", Media: media}
}

// NewInputPaidMediaLivePhoto creates an InputPaidMedia of type "live_photo"
// (Bot API 10.0): video is the live photo's video and photo its static photo.
func NewInputPaidMediaLivePhoto(video, photo InputFile) InputPaidMedia {
	return InputPaidMedia{Type: "live_photo", Media: video.WithPhoto(photo)}
}

// SendPaidMediaRequest represents a sendPaidMedia request (Bot API 7.6).
//
// StarCount is the price in Telegram Stars (the spec allows 1-25000) and Media
// holds up to 10 items; the client checks only that both are set and that each
// item has its files (the server checks the limits). The request is always sent as multipart/form-data. The
// spec's suggested_post_parameters is not supported yet (B-48c).
type SendPaidMediaRequest struct {
	BusinessConnectionID  string              `json:"business_connection_id,omitempty"` // 7.9
	ChatID                tg.ChatID           `json:"chat_id"`
	MessageThreadID       int                 `json:"message_thread_id,omitempty"`
	DirectMessagesTopicID int                 `json:"direct_messages_topic_id,omitempty"`
	StarCount             int                 `json:"star_count"`
	Media                 []InputPaidMedia    `json:"media"`
	Payload               string              `json:"payload,omitempty"` // 7.10: comes back in TransactionPartnerUser and Update.PurchasedPaidMedia
	Caption               string              `json:"caption,omitempty"`
	ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
	CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
	DisableNotification   bool                `json:"disable_notification,omitempty"`
	ProtectContent        bool                `json:"protect_content,omitempty"`
	AllowPaidBroadcast    bool                `json:"allow_paid_broadcast,omitempty"`
	ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
	ReplyMarkup           any                 `json:"reply_markup,omitempty"`
}

// SendPaidMedia sends paid media (Bot API 7.6). It is not retried: a resend could
// post the media twice.
func (c *Client) SendPaidMedia(ctx context.Context, req SendPaidMediaRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.StarCount <= 0 {
		return nil, tg.NewValidationError("star_count", "must be positive")
	}
	if len(req.Media) == 0 {
		return nil, tg.NewValidationError("media", "required")
	}
	if err := validatePaidMediaItems(req.Media); err != nil {
		return nil, err
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendPaidMedia", req, &result, extractChatID(req.ChatID)); err != nil {
		return nil, err
	}
	return &result, nil
}

// validatePaidMediaItems checks each item's files before the request, so a bad
// item is a ValidationError and never reaches the rate limiters or the circuit
// breaker (the builder would also refuse it, but there it counts as a failure).
func validatePaidMediaItems(list []InputPaidMedia) error {
	for i, m := range list {
		switch {
		case m.Media.IsEmpty():
			return tg.NewValidationError("media", fmt.Sprintf("item %d: media is required", i))
		case m.Media.Photo == nil && m.Type == "live_photo":
			return tg.NewValidationError("media", fmt.Sprintf("item %d: live_photo requires Media.Photo", i))
		case m.Media.Photo != nil && m.Media.Photo.IsEmpty():
			return tg.NewValidationError("media", fmt.Sprintf("item %d: photo is empty", i))
		case m.Thumbnail != nil && m.Thumbnail.IsEmpty():
			return tg.NewValidationError("media", fmt.Sprintf("item %d: thumbnail is empty", i))
		case m.Cover != nil && m.Cover.IsEmpty():
			return tg.NewValidationError("media", fmt.Sprintf("item %d: cover is empty", i))
		}
	}
	return nil
}

// handlePaidMedia writes sendPaidMedia's media list: one JSON item per
// InputPaidMedia, in list order. Each upload (the media, a live photo's static
// photo, a video's thumbnail and cover, in that order) becomes an attach://fileN
// file part; a file id or URL is written as given.
func handlePaidMedia(req *MultipartRequest, fieldName string, list []InputPaidMedia, attachIdx *int) error {
	items := make([]map[string]any, 0, len(list))
	for i, m := range list {
		item := map[string]any{"type": m.Type}
		ref, err := attachRef(req, m.Media, attachIdx)
		if err != nil {
			return fmt.Errorf("item %d: media: %w", i, err)
		}
		item["media"] = ref

		if m.Media.Photo != nil {
			if item["photo"], err = attachRef(req, *m.Media.Photo, attachIdx); err != nil {
				return fmt.Errorf("item %d: photo: %w", i, err)
			}
		} else if m.Type == "live_photo" {
			return fmt.Errorf("item %d: live_photo requires Media.Photo", i)
		}
		if m.Thumbnail != nil {
			if item["thumbnail"], err = attachRef(req, *m.Thumbnail, attachIdx); err != nil {
				return fmt.Errorf("item %d: thumbnail: %w", i, err)
			}
		}
		if m.Cover != nil {
			if item["cover"], err = attachRef(req, *m.Cover, attachIdx); err != nil {
				return fmt.Errorf("item %d: cover: %w", i, err)
			}
		}
		for _, f := range []struct {
			key string
			val int
		}{{"start_timestamp", m.StartTimestamp}, {"width", m.Width}, {"height", m.Height}, {"duration", m.Duration}} {
			if f.val != 0 {
				item[f.key] = f.val
			}
		}
		if m.SupportsStreaming {
			item["supports_streaming"] = true
		}
		items = append(items, item)
	}

	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("JSON marshal paid media: %w", err)
	}
	req.Params[fieldName] = string(data)
	return nil
}
