package sender

import "errors"

// Stub for the red record (TEST-PLAN §7): the names exist, the behaviour does not.

// NewInputMediaVoiceNote creates an InputMedia of type "voice_note" (Bot API 10.2, rich messages).
func NewInputMediaVoiceNote(media InputFile) InputMedia { return InputMedia{} }

// InputRichMediaType returns the media type; it makes InputMedia a tg.InputRichMedia (Bot API 10.2).
func (m InputMedia) InputRichMediaType() string { return "" }

// MarshalJSON is a stub.
func (m InputMedia) MarshalJSON() ([]byte, error) { return nil, errors.New("stub") }
