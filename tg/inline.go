package tg

import "encoding/json"

// --- InlineQueryResult Union (Partial Implementation) ---
//
// Telegram has 20+ InlineQueryResult types. We implement commonly-used ones
// and provide InlineQueryResultUnknown as a forward-compatible fallback.

// InlineQueryResult represents one result of an inline query.
type InlineQueryResult interface {
	inlineQueryResultTag()
	GetType() string
}

// InlineQueryResultArticle represents a link to an article or web page.
type InlineQueryResultArticle struct {
	Type                string                `json:"type"` // Always "article"
	ID                  string                `json:"id"`
	Title               string                `json:"title"`
	InputMessageContent InputMessageContent   `json:"input_message_content"`
	ReplyMarkup         *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	URL                 string                `json:"url,omitempty"`
	HideURL             bool                  `json:"hide_url,omitempty"`
	Description         string                `json:"description,omitempty"`
	ThumbnailURL        string                `json:"thumbnail_url,omitempty"`
	ThumbnailWidth      int                   `json:"thumbnail_width,omitempty"`
	ThumbnailHeight     int                   `json:"thumbnail_height,omitempty"`
}

func (InlineQueryResultArticle) inlineQueryResultTag() {}
func (InlineQueryResultArticle) GetType() string       { return "article" }

// InlineQueryResultPhoto represents a link to a photo.
type InlineQueryResultPhoto struct {
	Type                  string                `json:"type"` // Always "photo"
	ID                    string                `json:"id"`
	PhotoURL              string                `json:"photo_url"`
	ThumbnailURL          string                `json:"thumbnail_url"`
	PhotoWidth            int                   `json:"photo_width,omitempty"`
	PhotoHeight           int                   `json:"photo_height,omitempty"`
	Title                 string                `json:"title,omitempty"`
	Description           string                `json:"description,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	ParseMode             string                `json:"parse_mode,omitempty"`
	CaptionEntities       []MessageEntity       `json:"caption_entities,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultPhoto) inlineQueryResultTag() {}
func (InlineQueryResultPhoto) GetType() string       { return "photo" }

// InlineQueryResultDocument represents a link to a file.
type InlineQueryResultDocument struct {
	Type                string                `json:"type"` // Always "document"
	ID                  string                `json:"id"`
	Title               string                `json:"title"`
	DocumentURL         string                `json:"document_url"`
	MimeType            string                `json:"mime_type"`
	Caption             string                `json:"caption,omitempty"`
	ParseMode           string                `json:"parse_mode,omitempty"`
	CaptionEntities     []MessageEntity       `json:"caption_entities,omitempty"`
	Description         string                `json:"description,omitempty"`
	ReplyMarkup         *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent InputMessageContent   `json:"input_message_content,omitempty"`
	ThumbnailURL        string                `json:"thumbnail_url,omitempty"`
	ThumbnailWidth      int                   `json:"thumbnail_width,omitempty"`
	ThumbnailHeight     int                   `json:"thumbnail_height,omitempty"`
}

func (InlineQueryResultDocument) inlineQueryResultTag() {}
func (InlineQueryResultDocument) GetType() string       { return "document" }

// InlineQueryResultUnknown is a fallback for unknown/future result types.
type InlineQueryResultUnknown struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (InlineQueryResultUnknown) inlineQueryResultTag() {}
func (r InlineQueryResultUnknown) GetType() string     { return r.Type }

// --- InputMessageContent ---

// InputMessageContent represents the content of a message to be sent
// as a result of an inline query.
type InputMessageContent interface {
	inputMessageContentTag()
}

// InputTextMessageContent represents text content for an inline query result.
type InputTextMessageContent struct {
	MessageText        string              `json:"message_text"`
	ParseMode          string              `json:"parse_mode,omitempty"`
	Entities           []MessageEntity     `json:"entities,omitempty"`
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

func (InputTextMessageContent) inputMessageContentTag() {}

// InputLocationMessageContent represents location content for an inline query result.
type InputLocationMessageContent struct {
	Latitude             float64 `json:"latitude"`
	Longitude            float64 `json:"longitude"`
	HorizontalAccuracy   float64 `json:"horizontal_accuracy,omitempty"`
	LivePeriod           int     `json:"live_period,omitempty"`
	Heading              int     `json:"heading,omitempty"`
	ProximityAlertRadius int     `json:"proximity_alert_radius,omitempty"`
}

func (InputLocationMessageContent) inputMessageContentTag() {}

// InputVenueMessageContent represents venue content for an inline query result.
type InputVenueMessageContent struct {
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	Title           string  `json:"title"`
	Address         string  `json:"address"`
	FoursquareID    string  `json:"foursquare_id,omitempty"`
	FoursquareType  string  `json:"foursquare_type,omitempty"`
	GooglePlaceID   string  `json:"google_place_id,omitempty"`
	GooglePlaceType string  `json:"google_place_type,omitempty"`
}

func (InputVenueMessageContent) inputMessageContentTag() {}

// InputContactMessageContent represents contact content for an inline query result.
type InputContactMessageContent struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	VCard       string `json:"vcard,omitempty"`
}

func (InputContactMessageContent) inputMessageContentTag() {}

// InputInvoiceMessageContent represents invoice content for an inline query result.
type InputInvoiceMessageContent struct {
	Title                     string         `json:"title"`
	Description               string         `json:"description"`
	Payload                   string         `json:"payload"`
	ProviderToken             string         `json:"provider_token,omitempty"`
	Currency                  string         `json:"currency"`
	Prices                    []LabeledPrice `json:"prices"`
	MaxTipAmount              int            `json:"max_tip_amount,omitempty"`
	SuggestedTipAmounts       []int          `json:"suggested_tip_amounts,omitempty"`
	ProviderData              string         `json:"provider_data,omitempty"`
	PhotoURL                  string         `json:"photo_url,omitempty"`
	PhotoSize                 int            `json:"photo_size,omitempty"`
	PhotoWidth                int            `json:"photo_width,omitempty"`
	PhotoHeight               int            `json:"photo_height,omitempty"`
	NeedName                  bool           `json:"need_name,omitempty"`
	NeedPhoneNumber           bool           `json:"need_phone_number,omitempty"`
	NeedEmail                 bool           `json:"need_email,omitempty"`
	NeedShippingAddress       bool           `json:"need_shipping_address,omitempty"`
	SendPhoneNumberToProvider bool           `json:"send_phone_number_to_provider,omitempty"`
	SendEmailToProvider       bool           `json:"send_email_to_provider,omitempty"`
	IsFlexible                bool           `json:"is_flexible,omitempty"`
}

func (InputInvoiceMessageContent) inputMessageContentTag() {}

// --- Other Inline Types ---

// InlineQueryResultsButton represents a button above inline query results.
type InlineQueryResultsButton struct {
	Text           string      `json:"text"`
	WebApp         *WebAppInfo `json:"web_app,omitempty"`
	StartParameter string      `json:"start_parameter,omitempty"`
}

// SentWebAppMessage describes an inline message sent by a Web App.
type SentWebAppMessage struct {
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// PreparedInlineMessage represents a prepared inline message.
type PreparedInlineMessage struct {
	ID             string `json:"id"`
	ExpirationDate int64  `json:"expiration_date"`
}
