package sender

// InputMedia represents media content for editMessageMedia, sendMediaGroup
// and, since Bot API 10.0, poll media (SendPollRequest.Media and
// .ExplanationMedia, InputPollOption.Media).
// Use one of the constructors: NewInputMediaPhoto, NewInputMediaDocument, etc.
type InputMedia struct {
	// Type is the media type: "photo", "video", "document", "audio",
	// "animation", "live_photo"; for poll media also "sticker", "location",
	// "venue" (Bot API 10.0).
	Type string

	// Media is the file to send (FileID, URL, or Reader upload).
	Media InputFile

	// Caption for the media (optional).
	Caption string

	// ParseMode for caption (optional): HTML, Markdown, MarkdownV2.
	ParseMode string

	// Emoji for a just uploaded sticker (sticker only, optional; Bot API 10.0).
	Emoji string

	// Latitude and Longitude of a location or venue (location and venue; always
	// sent, also when 0; Bot API 10.0).
	Latitude  float64
	Longitude float64

	// HorizontalAccuracy in meters, 0-1500 (location and venue, optional; sent
	// when not 0; Bot API 10.0).
	HorizontalAccuracy float64

	// Title and Address of a venue (venue only; always sent; Bot API 10.0).
	Title   string
	Address string

	// Foursquare and Google Places data of a venue (venue only, optional; each
	// sent when not empty; Bot API 10.0).
	FoursquareID    string
	FoursquareType  string
	GooglePlaceID   string
	GooglePlaceType string
}

// NewInputMediaSticker creates an InputMedia of type "sticker" (Bot API 10.0,
// poll option media). Set Emoji on the result for a just uploaded sticker.
func NewInputMediaSticker(media InputFile) InputMedia {
	return InputMedia{Type: "sticker", Media: media}
}

// NewInputMediaLocation creates an InputMedia of type "location" (Bot API
// 10.0, poll media). It has no file; set HorizontalAccuracy on the result if
// needed.
func NewInputMediaLocation(latitude, longitude float64) InputMedia {
	return InputMedia{Type: "location", Latitude: latitude, Longitude: longitude}
}

// NewInputMediaVenue creates an InputMedia of type "venue" (Bot API 10.0,
// poll media). It has no file; set the Foursquare, Google Places and
// HorizontalAccuracy fields on the result if needed.
func NewInputMediaVenue(latitude, longitude float64, title, address string) InputMedia {
	return InputMedia{Type: "venue", Latitude: latitude, Longitude: longitude, Title: title, Address: address}
}

// NewInputMediaPhoto creates an InputMedia of type "photo".
func NewInputMediaPhoto(media InputFile) InputMedia {
	return InputMedia{Type: "photo", Media: media}
}

// NewInputMediaDocument creates an InputMedia of type "document".
func NewInputMediaDocument(media InputFile) InputMedia {
	return InputMedia{Type: "document", Media: media}
}

// NewInputMediaVideo creates an InputMedia of type "video".
func NewInputMediaVideo(media InputFile) InputMedia {
	return InputMedia{Type: "video", Media: media}
}

// NewInputMediaAudio creates an InputMedia of type "audio".
func NewInputMediaAudio(media InputFile) InputMedia {
	return InputMedia{Type: "audio", Media: media}
}

// NewInputMediaAnimation creates an InputMedia of type "animation".
func NewInputMediaAnimation(media InputFile) InputMedia {
	return InputMedia{Type: "animation", Media: media}
}

// NewInputMediaLivePhoto creates an InputMedia of type "live_photo" (Bot API
// 10.0): video is the live photo's video and photo its static photo.
func NewInputMediaLivePhoto(video, photo InputFile) InputMedia {
	return InputMedia{Type: "live_photo", Media: video.WithPhoto(photo)}
}

// WithCaption returns a copy with the caption set.
func (m InputMedia) WithCaption(caption string) InputMedia {
	m.Caption = caption
	return m
}

// WithParseMode returns a copy with the parse mode set.
func (m InputMedia) WithParseMode(mode string) InputMedia {
	m.ParseMode = mode
	return m
}
