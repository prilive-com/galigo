package sender

import (
	"github.com/prilive-com/galigo/tg"
)

// SendMessageRequest represents a request to send a text message.
type SendMessageRequest struct {
	ChatID              tg.ChatID              `json:"chat_id"`
	Text                string                 `json:"text"`
	ParseMode           tg.ParseMode           `json:"parse_mode,omitempty"`
	LinkPreviewOptions  *tg.LinkPreviewOptions `json:"link_preview_options,omitempty"`
	DisableNotification bool                   `json:"disable_notification,omitempty"`
	ProtectContent      bool                   `json:"protect_content,omitempty"`
	ReplyToMessageID    int                    `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any                    `json:"reply_markup,omitempty"`

	// Deprecated: Use LinkPreviewOptions.IsDisabled instead.
	DisableWebPagePreview bool `json:"disable_web_page_preview,omitempty"`
}

// SendPhotoRequest represents a request to send a photo.
type SendPhotoRequest struct {
	ChatID              tg.ChatID    `json:"chat_id"`
	Photo               InputFile    `json:"photo"` // file_id, URL, or upload
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
}

// EditMessageTextRequest represents a request to edit message text.
type EditMessageTextRequest struct {
	ChatID             tg.ChatID              `json:"chat_id,omitempty"`
	MessageID          int                    `json:"message_id,omitempty"`
	InlineMessageID    string                 `json:"inline_message_id,omitempty"`
	Text               string                 `json:"text"`
	ParseMode          tg.ParseMode           `json:"parse_mode,omitempty"`
	LinkPreviewOptions *tg.LinkPreviewOptions `json:"link_preview_options,omitempty"`
	ReplyMarkup        any                    `json:"reply_markup,omitempty"`

	// Deprecated: Use LinkPreviewOptions.IsDisabled instead.
	DisableWebPagePreview bool `json:"disable_web_page_preview,omitempty"`
}

// EditMessageCaptionRequest represents a request to edit message caption.
type EditMessageCaptionRequest struct {
	ChatID          tg.ChatID    `json:"chat_id,omitempty"`
	MessageID       int          `json:"message_id,omitempty"`
	InlineMessageID string       `json:"inline_message_id,omitempty"`
	Caption         string       `json:"caption,omitempty"`
	ParseMode       tg.ParseMode `json:"parse_mode,omitempty"`
	ReplyMarkup     any          `json:"reply_markup,omitempty"`
}

// EditMessageReplyMarkupRequest represents a request to edit message markup.
type EditMessageReplyMarkupRequest struct {
	ChatID          tg.ChatID `json:"chat_id,omitempty"`
	MessageID       int       `json:"message_id,omitempty"`
	InlineMessageID string    `json:"inline_message_id,omitempty"`
	ReplyMarkup     any       `json:"reply_markup,omitempty"`
}

// EditMessageMediaRequest represents a request to edit message media.
type EditMessageMediaRequest struct {
	ChatID          tg.ChatID  `json:"chat_id,omitempty"`
	MessageID       int        `json:"message_id,omitempty"`
	InlineMessageID string     `json:"inline_message_id,omitempty"`
	Media           InputMedia `json:"media"`
	ReplyMarkup     any        `json:"reply_markup,omitempty"`
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
	ChatID              tg.ChatID    `json:"chat_id"`
	FromChatID          tg.ChatID    `json:"from_chat_id"`
	MessageID           int          `json:"message_id"`
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
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

// ================== Bot Identity Methods ==================

// GetMeRequest represents a request to get bot info.
type GetMeRequest struct{}

// LogOutRequest represents a request to log out the bot.
type LogOutRequest struct{}

// CloseRequest represents a request to close the bot.
type CloseRequest struct{}

// ================== Media Methods ==================

// SendDocumentRequest represents a request to send a document.
type SendDocumentRequest struct {
	ChatID                      tg.ChatID    `json:"chat_id"`
	Document                    InputFile    `json:"document"`
	Thumbnail                   *InputFile   `json:"thumbnail,omitempty"`
	Caption                     string       `json:"caption,omitempty"`
	ParseMode                   tg.ParseMode `json:"parse_mode,omitempty"`
	DisableContentTypeDetection bool         `json:"disable_content_type_detection,omitempty"`
	DisableNotification         bool         `json:"disable_notification,omitempty"`
	ProtectContent              bool         `json:"protect_content,omitempty"`
	ReplyToMessageID            int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup                 any          `json:"reply_markup,omitempty"`
}

// SendVideoRequest represents a request to send a video.
type SendVideoRequest struct {
	ChatID              tg.ChatID    `json:"chat_id"`
	Video               InputFile    `json:"video"`
	Thumbnail           *InputFile   `json:"thumbnail,omitempty"`
	Duration            int          `json:"duration,omitempty"`
	Width               int          `json:"width,omitempty"`
	Height              int          `json:"height,omitempty"`
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	SupportsStreaming   bool         `json:"supports_streaming,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
}

// SendAudioRequest represents a request to send an audio file.
type SendAudioRequest struct {
	ChatID              tg.ChatID    `json:"chat_id"`
	Audio               InputFile    `json:"audio"`
	Thumbnail           *InputFile   `json:"thumbnail,omitempty"`
	Duration            int          `json:"duration,omitempty"`
	Performer           string       `json:"performer,omitempty"`
	Title               string       `json:"title,omitempty"`
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
}

// SendVoiceRequest represents a request to send a voice message.
type SendVoiceRequest struct {
	ChatID              tg.ChatID    `json:"chat_id"`
	Voice               InputFile    `json:"voice"`
	Duration            int          `json:"duration,omitempty"`
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
}

// SendAnimationRequest represents a request to send an animation.
type SendAnimationRequest struct {
	ChatID              tg.ChatID    `json:"chat_id"`
	Animation           InputFile    `json:"animation"`
	Thumbnail           *InputFile   `json:"thumbnail,omitempty"`
	Duration            int          `json:"duration,omitempty"`
	Width               int          `json:"width,omitempty"`
	Height              int          `json:"height,omitempty"`
	Caption             string       `json:"caption,omitempty"`
	ParseMode           tg.ParseMode `json:"parse_mode,omitempty"`
	DisableNotification bool         `json:"disable_notification,omitempty"`
	ProtectContent      bool         `json:"protect_content,omitempty"`
	ReplyToMessageID    int          `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any          `json:"reply_markup,omitempty"`
}

// SendVideoNoteRequest represents a request to send a video note.
type SendVideoNoteRequest struct {
	ChatID              tg.ChatID  `json:"chat_id"`
	VideoNote           InputFile  `json:"video_note"`
	Thumbnail           *InputFile `json:"thumbnail,omitempty"`
	Duration            int        `json:"duration,omitempty"`
	Length              int        `json:"length,omitempty"`
	DisableNotification bool       `json:"disable_notification,omitempty"`
	ProtectContent      bool       `json:"protect_content,omitempty"`
	ReplyToMessageID    int        `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any        `json:"reply_markup,omitempty"`
}

// SendStickerRequest represents a request to send a sticker.
type SendStickerRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	Sticker             InputFile `json:"sticker"`
	Emoji               string    `json:"emoji,omitempty"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
	ReplyToMessageID    int       `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any       `json:"reply_markup,omitempty"`
}

// SendMediaGroupRequest represents a request to send a media group.
type SendMediaGroupRequest struct {
	ChatID              tg.ChatID   `json:"chat_id"`
	Media               []InputFile `json:"media"`
	DisableNotification bool        `json:"disable_notification,omitempty"`
	ProtectContent      bool        `json:"protect_content,omitempty"`
	ReplyToMessageID    int         `json:"reply_to_message_id,omitempty"`
}

// ================== Utility Methods ==================

// GetFileRequest represents a request to get file info.
type GetFileRequest struct {
	FileID string `json:"file_id"`
}

// SendChatActionRequest represents a request to send a chat action.
type SendChatActionRequest struct {
	ChatID          tg.ChatID `json:"chat_id"`
	Action          string    `json:"action"`
	MessageThreadID int       `json:"message_thread_id,omitempty"`
}

// GetUserProfilePhotosRequest represents a request to get user profile photos.
type GetUserProfilePhotosRequest struct {
	UserID int64 `json:"user_id"`
	Offset int   `json:"offset,omitempty"`
	Limit  int   `json:"limit,omitempty"`
}

// ================== Location/Contact Methods ==================

// SendLocationRequest represents a request to send a location.
type SendLocationRequest struct {
	ChatID               tg.ChatID `json:"chat_id"`
	Latitude             float64   `json:"latitude"`
	Longitude            float64   `json:"longitude"`
	HorizontalAccuracy   float64   `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int       `json:"live_period,omitempty"`
	Heading              int       `json:"heading,omitempty"`
	ProximityAlertRadius int       `json:"proximity_alert_radius,omitempty"`
	DisableNotification  bool      `json:"disable_notification,omitempty"`
	ProtectContent       bool      `json:"protect_content,omitempty"`
	ReplyToMessageID     int       `json:"reply_to_message_id,omitempty"`
	ReplyMarkup          any       `json:"reply_markup,omitempty"`
}

// SendVenueRequest represents a request to send a venue.
type SendVenueRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	Latitude            float64   `json:"latitude"`
	Longitude           float64   `json:"longitude"`
	Title               string    `json:"title"`
	Address             string    `json:"address"`
	FoursquareID        string    `json:"foursquare_id,omitempty"`
	FoursquareType      string    `json:"foursquare_type,omitempty"`
	GooglePlaceID       string    `json:"google_place_id,omitempty"`
	GooglePlaceType     string    `json:"google_place_type,omitempty"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
	ReplyToMessageID    int       `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any       `json:"reply_markup,omitempty"`
}

// SendContactRequest represents a request to send a contact.
type SendContactRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	PhoneNumber         string    `json:"phone_number"`
	FirstName           string    `json:"first_name"`
	LastName            string    `json:"last_name,omitempty"`
	Vcard               string    `json:"vcard,omitempty"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
	ReplyToMessageID    int       `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any       `json:"reply_markup,omitempty"`
}

// SendDiceRequest represents a request to send a dice.
type SendDiceRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	Emoji               string    `json:"emoji,omitempty"` // Default: dice emoji
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
	ReplyToMessageID    int       `json:"reply_to_message_id,omitempty"`
	ReplyMarkup         any       `json:"reply_markup,omitempty"`
}

// ================== Bulk Operations ==================

// ForwardMessagesRequest represents a request to forward multiple messages.
type ForwardMessagesRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	FromChatID          tg.ChatID `json:"from_chat_id"`
	MessageIDs          []int     `json:"message_ids"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
}

// CopyMessagesRequest represents a request to copy multiple messages.
type CopyMessagesRequest struct {
	ChatID              tg.ChatID `json:"chat_id"`
	FromChatID          tg.ChatID `json:"from_chat_id"`
	MessageIDs          []int     `json:"message_ids"`
	DisableNotification bool      `json:"disable_notification,omitempty"`
	ProtectContent      bool      `json:"protect_content,omitempty"`
	RemoveCaption       bool      `json:"remove_caption,omitempty"`
}

// DeleteMessagesRequest represents a request to delete multiple messages.
type DeleteMessagesRequest struct {
	ChatID     tg.ChatID `json:"chat_id"`
	MessageIDs []int     `json:"message_ids"`
}

// SetMessageReactionRequest represents a request to set a message reaction.
type SetMessageReactionRequest struct {
	ChatID    tg.ChatID      `json:"chat_id"`
	MessageID int            `json:"message_id"`
	Reaction  []ReactionType `json:"reaction,omitempty"`
	IsBig     bool           `json:"is_big,omitempty"`
}

// ReactionType represents a reaction type.
type ReactionType struct {
	Type        string `json:"type"` // "emoji" or "custom_emoji"
	Emoji       string `json:"emoji,omitempty"`
	CustomEmoji string `json:"custom_emoji_id,omitempty"`
}
