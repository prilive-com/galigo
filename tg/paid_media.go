package tg

// PaidMediaInfo describes the paid media added to a message (Bot API 7.6).
type PaidMediaInfo struct {
	StarCount int         `json:"star_count"` // the number of Telegram Stars that must be paid to buy access to the media
	PaidMedia []PaidMedia `json:"paid_media"`
}

// PaidMedia describes paid media (Bot API 7.6). It is one of the spec's PaidMediaLivePhoto ("live_photo", 10.0),
// PaidMediaPhoto ("photo"), PaidMediaPreview ("preview") and PaidMediaVideo ("video"); Type says which, and only
// that variant's fields are set.
type PaidMedia struct {
	Type      string      `json:"type"`                 // "live_photo", "photo", "preview" or "video"
	LivePhoto *LivePhoto  `json:"live_photo,omitempty"` // "live_photo"
	Photo     []PhotoSize `json:"photo,omitempty"`      // "photo"
	Width     int         `json:"width,omitempty"`      // "preview": media width as defined by the sender
	Height    int         `json:"height,omitempty"`     // "preview": media height as defined by the sender
	Duration  int         `json:"duration,omitempty"`   // "preview": duration in seconds as defined by the sender
	Video     *Video      `json:"video,omitempty"`      // "video"
}

// PaidMediaPurchased is a paid media purchase (Bot API 7.10): a user bought paid media with a non-empty payload that
// the bot sent in a non-channel chat.
type PaidMediaPurchased struct {
	From             User   `json:"from"`               // the user who bought the media
	PaidMediaPayload string `json:"paid_media_payload"` // the bot-specified paid media payload
}
