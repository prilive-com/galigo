package sender

// InputMedia represents media content for editMessageMedia.
// Use one of the constructors: InputMediaPhoto, InputMediaDocument, etc.
type InputMedia struct {
	// Type is the media type: "photo", "video", "document", "audio", "animation".
	Type string

	// Media is the file to send (FileID, URL, or Reader upload).
	Media InputFile

	// Caption for the media (optional).
	Caption string

	// ParseMode for caption (optional): HTML, Markdown, MarkdownV2.
	ParseMode string
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
