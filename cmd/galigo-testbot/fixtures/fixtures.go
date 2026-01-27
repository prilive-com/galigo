package fixtures

import (
	"bytes"
	_ "embed"
	"io"
)

//go:embed photo.jpg
var photoData []byte

// Photo returns a minimal valid JPEG image for testing.
func Photo() io.Reader {
	return bytes.NewReader(photoData)
}

// PhotoBytes returns the photo as bytes.
func PhotoBytes() []byte {
	return photoData
}

// Document returns a simple text document for testing.
func Document() io.Reader {
	content := []byte("galigo-testbot test document\n\nThis is a test file for acceptance testing.\n")
	return bytes.NewReader(content)
}

// DocumentBytes returns the document as bytes.
func DocumentBytes() []byte {
	return []byte("galigo-testbot test document\n\nThis is a test file for acceptance testing.\n")
}

// TestURLs provides public URLs for media types that are hard to generate.
// NOTE: URLs must point directly to files, not web pages that generate content.
var TestURLs = struct {
	Video     string
	Audio     string
	Voice     string
	VideoNote string
}{
	Video:     "",
	Audio:     "",
	Voice:     "",
	VideoNote: "",
}

// HasVideo returns true if a video URL is available.
func HasVideo() bool {
	return TestURLs.Video != ""
}

// HasAudio returns true if an audio URL is available.
func HasAudio() bool {
	return TestURLs.Audio != ""
}

// HasVoice returns true if a voice URL is available.
func HasVoice() bool {
	return TestURLs.Voice != ""
}

// HasVideoNote returns true if a video note is available.
func HasVideoNote() bool {
	return TestURLs.VideoNote != ""
}
