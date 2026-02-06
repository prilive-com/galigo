package sender_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/sender"
)

func TestInputFile_FromReader(t *testing.T) {
	reader := strings.NewReader("file content")
	file := sender.FromReader(reader, "test.txt")

	assert.True(t, file.IsUpload())
	assert.False(t, file.IsEmpty())
	assert.Equal(t, "test.txt", file.FileName)
	assert.Equal(t, "", file.Value())
}

func TestInputFile_FromFileID(t *testing.T) {
	file := sender.FromFileID("AgACAgIAAxkBAAI")

	assert.False(t, file.IsUpload())
	assert.False(t, file.IsEmpty())
	assert.Equal(t, "AgACAgIAAxkBAAI", file.FileID)
	assert.Equal(t, "AgACAgIAAxkBAAI", file.Value())
}

func TestInputFile_FromURL(t *testing.T) {
	file := sender.FromURL("https://example.com/photo.jpg")

	assert.False(t, file.IsUpload())
	assert.False(t, file.IsEmpty())
	assert.Equal(t, "https://example.com/photo.jpg", file.URL)
	assert.Equal(t, "https://example.com/photo.jpg", file.Value())
}

func TestInputFile_IsEmpty(t *testing.T) {
	var file sender.InputFile
	assert.True(t, file.IsEmpty())
}

func TestInputFile_WithCaption(t *testing.T) {
	file := sender.FromFileID("abc123").WithCaption("Photo caption")

	assert.Equal(t, "Photo caption", file.Caption)
	assert.Equal(t, "abc123", file.FileID)
}

func TestInputFile_WithParseMode(t *testing.T) {
	file := sender.FromFileID("abc123").WithParseMode("HTML")

	assert.Equal(t, "HTML", file.ParseMode)
}

func TestInputFile_WithMediaType(t *testing.T) {
	file := sender.FromFileID("abc123").WithMediaType("photo")

	assert.Equal(t, "photo", file.MediaType)
}

func TestInputFile_Chaining(t *testing.T) {
	file := sender.FromURL("https://example.com/video.mp4").
		WithMediaType("video").
		WithCaption("Great video").
		WithParseMode("Markdown")

	assert.Equal(t, "https://example.com/video.mp4", file.URL)
	assert.Equal(t, "video", file.MediaType)
	assert.Equal(t, "Great video", file.Caption)
	assert.Equal(t, "Markdown", file.ParseMode)
}

func TestConstants(t *testing.T) {
	assert.Equal(t, int64(50*1024*1024), int64(sender.MaxUploadSize))
	assert.Equal(t, int64(10*1024*1024), int64(sender.MaxPhotoSize))
}
