package fixtures

import (
	"bytes"
	"encoding/base64"
	"io"
)

// Minimal valid JPEG (1x1 red pixel) - base64 encoded
const minimalJPEG = "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCAABAAEDASIAAhEBAxEB/8QAFQABAQAAAAAAAAAAAAAAAAAAAAn/xAAUEAEAAAAAAAAAAAAAAAAAAAAA/8QAFQEBAQAAAAAAAAAAAAAAAAAAAAX/xAAUEQEAAAAAAAAAAAAAAAAAAAAA/9oADAMBEQCEQT8AVKgB/9k="

// Minimal valid GIF (1x1 transparent pixel) - base64 encoded
const minimalGIF = "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"

// Minimal valid PNG (1x1 red pixel) - base64 encoded
const minimalPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg=="

// Minimal valid WebP (1x1 pixel) - base64 encoded
const minimalWebP = "UklGRlYAAABXRUJQVlA4IEoAAADQAQCdASoBAAEAAUAmJYgCdAEO/hOMAAD++O9O+C9K3/4T/xPB/t7fRv/Y9X/6B+CP4l/qf78/Iz///sA/r39g/6wA"

// Photo returns a minimal valid JPEG image for testing.
func Photo() io.Reader {
	data, _ := base64.StdEncoding.DecodeString(minimalJPEG)
	return bytes.NewReader(data)
}

// PhotoBytes returns the photo as bytes.
func PhotoBytes() []byte {
	data, _ := base64.StdEncoding.DecodeString(minimalJPEG)
	return data
}

// Animation returns a minimal valid GIF for testing.
func Animation() io.Reader {
	data, _ := base64.StdEncoding.DecodeString(minimalGIF)
	return bytes.NewReader(data)
}

// Sticker returns a minimal valid WebP for testing.
// Note: Telegram may reject this as it expects proper sticker dimensions.
// Use StickerFallbackURL() if upload fails.
func Sticker() io.Reader {
	data, _ := base64.StdEncoding.DecodeString(minimalWebP)
	return bytes.NewReader(data)
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
// These are small, publicly available test files.
var TestURLs = struct {
	// Photo - A small test image
	Photo string
	// Video - Big Buck Bunny trailer (small version)
	Video string
	// Audio - Sample MP3
	Audio string
	// Voice - Sample OGG (Telegram requires OGG Opus for voice)
	Voice string
	// VideoNote - Square video for video notes
	VideoNote string
	// Animation - A small test GIF
	Animation string
	// Sticker - A valid Telegram sticker (use file_id after first upload)
	Sticker string
}{
	// Using well-known public test files
	Photo:     "https://via.placeholder.com/150/FF0000/FFFFFF?text=Test",
	Video:     "https://sample-videos.com/video321/mp4/720/big_buck_bunny_720p_1mb.mp4",
	Audio:     "https://sample-videos.com/audio/mp3/crowd-cheering.mp3",
	Voice:     "", // OGG Opus is rare, will skip or use fallback
	VideoNote: "", // Requires specific format, will skip
	Animation: "https://upload.wikimedia.org/wikipedia/commons/2/2c/Rotating_earth_%28large%29.gif",
	Sticker:   "", // Will capture file_id from first successful upload
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

// HasPhoto returns true if a photo URL is available.
func HasPhoto() bool {
	return TestURLs.Photo != ""
}

// HasAnimation returns true if an animation URL is available.
func HasAnimation() bool {
	return TestURLs.Animation != ""
}
