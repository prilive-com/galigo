package fixtures

import (
	"bytes"
	_ "embed"
	"io"
)

//go:embed photo.jpg
var photoData []byte

//go:embed animation.gif
var animationData []byte

//go:embed sticker.png
var stickerData []byte

//go:embed audio.mp3
var audioData []byte

//go:embed voice.ogg
var voiceData []byte

//go:embed video.mp4
var videoData []byte

//go:embed videonote.mp4
var videoNoteData []byte

// Photo returns a minimal valid JPEG image for testing.
func Photo() io.Reader {
	return bytes.NewReader(photoData)
}

// PhotoBytes returns the photo as bytes.
func PhotoBytes() []byte {
	return photoData
}

// Animation returns a minimal valid GIF animation for testing.
func Animation() io.Reader {
	return bytes.NewReader(animationData)
}

// AnimationBytes returns the animation as bytes.
func AnimationBytes() []byte {
	return animationData
}

// Sticker returns a minimal valid PNG sticker (512x512) for testing.
func Sticker() io.Reader {
	return bytes.NewReader(stickerData)
}

// StickerBytes returns the sticker as bytes.
func StickerBytes() []byte {
	return stickerData
}

// Audio returns a minimal valid MP3 audio for testing.
func Audio() io.Reader {
	return bytes.NewReader(audioData)
}

// AudioBytes returns the audio as bytes.
func AudioBytes() []byte {
	return audioData
}

// Voice returns a minimal valid OGG Opus voice message for testing.
func Voice() io.Reader {
	return bytes.NewReader(voiceData)
}

// VoiceBytes returns the voice as bytes.
func VoiceBytes() []byte {
	return voiceData
}

// Video returns a minimal valid MP4 video for testing.
func Video() io.Reader {
	return bytes.NewReader(videoData)
}

// VideoBytes returns the video as bytes.
func VideoBytes() []byte {
	return videoData
}

// VideoNote returns a minimal valid MP4 video note (square) for testing.
func VideoNote() io.Reader {
	return bytes.NewReader(videoNoteData)
}

// VideoNoteBytes returns the video note as bytes.
func VideoNoteBytes() []byte {
	return videoNoteData
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
