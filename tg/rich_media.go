package tg

// Bot API 10.2 rich messages, send side: the media of InputRichMessage.Media and of the media blocks.

// InputRichMedia is the media of an InputRichMessageMedia or of a media block (Bot API 10.2): an InputMediaAnimation,
// InputMediaAudio, InputMediaDocument, InputMediaPhoto, InputMediaVideo or InputMediaVoiceNote. Use a
// sender.InputMedia (sender.NewInputMediaPhoto, ..., sender.NewInputMediaVoiceNote). A file upload is possible only in
// InputRichMessage.Media of a request sent by sender.Client, and only in a sender.InputMedia or *sender.InputMedia (not
// in a type that embeds one); a media block takes a file_id or an HTTP URL. The JSON is the concrete type's own
// encoding; a nil *sender.InputMedia encodes as null.
type InputRichMedia interface {
	// InputRichMediaType returns the media type, for example "photo".
	InputRichMediaType() string
}

// InputRichMessageMedia is a media element of a rich message to send (Bot API 10.2). The HTML or Markdown text refers
// to it with a tg://photo?id=, tg://video?id= or tg://audio?id= link that names its ID.
type InputRichMessageMedia struct {
	ID    string         `json:"id"`
	Media InputRichMedia `json:"media"`
}

// InputRichBlockAnimation is a block with an animation, corresponding to the HTML tag <video> (Bot API 10.2). The
// caption of the media itself is ignored; use Caption.
type InputRichBlockAnimation struct {
	Animation InputRichMedia    `json:"animation"`
	Caption   *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockAudio is a block with a music file, corresponding to the HTML tag <audio> (Bot API 10.2).
type InputRichBlockAudio struct {
	Audio   InputRichMedia    `json:"audio"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockPhoto is a block with a photo, corresponding to the HTML tag <img> (Bot API 10.2).
type InputRichBlockPhoto struct {
	Photo   InputRichMedia    `json:"photo"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockVideo is a block with a video, corresponding to the HTML tag <video> (Bot API 10.2).
type InputRichBlockVideo struct {
	Video   InputRichMedia    `json:"video"`
	Caption *RichBlockCaption `json:"caption,omitempty"`
}

// InputRichBlockVoiceNote is a block with a voice note, corresponding to the HTML tag <audio> (Bot API 10.2).
type InputRichBlockVoiceNote struct {
	VoiceNote InputRichMedia    `json:"voice_note"`
	Caption   *RichBlockCaption `json:"caption,omitempty"`
}

// MarshalJSON encodes the block with "type":"animation".
func (b InputRichBlockAnimation) MarshalJSON() ([]byte, error) {
	type t InputRichBlockAnimation
	return marshalInputRichBlock("animation", t(b))
}

// MarshalJSON encodes the block with "type":"audio".
func (b InputRichBlockAudio) MarshalJSON() ([]byte, error) {
	type t InputRichBlockAudio
	return marshalInputRichBlock("audio", t(b))
}

// MarshalJSON encodes the block with "type":"photo".
func (b InputRichBlockPhoto) MarshalJSON() ([]byte, error) {
	type t InputRichBlockPhoto
	return marshalInputRichBlock("photo", t(b))
}

// MarshalJSON encodes the block with "type":"video".
func (b InputRichBlockVideo) MarshalJSON() ([]byte, error) {
	type t InputRichBlockVideo
	return marshalInputRichBlock("video", t(b))
}

// MarshalJSON encodes the block with "type":"voice_note".
func (b InputRichBlockVoiceNote) MarshalJSON() ([]byte, error) {
	type t InputRichBlockVoiceNote
	return marshalInputRichBlock("voice_note", t(b))
}

func (InputRichBlockAnimation) inputRichBlock() {}
func (InputRichBlockAudio) inputRichBlock()     {}
func (InputRichBlockPhoto) inputRichBlock()     {}
func (InputRichBlockVideo) inputRichBlock()     {}
func (InputRichBlockVoiceNote) inputRichBlock() {}
