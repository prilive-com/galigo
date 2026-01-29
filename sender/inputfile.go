package sender

import (
	"bytes"
	"encoding/json"
	"io"
)

const (
	// MaxUploadSize is the maximum file size for Bot API uploads (50MB).
	// For larger files, use external storage and send URL.
	MaxUploadSize = 50 * 1024 * 1024

	// MaxPhotoSize is the maximum photo file size (10MB).
	MaxPhotoSize = 10 * 1024 * 1024
)

// InputFile represents a file to upload or reference.
// Use one of the constructors: FromReader, FromFileID, FromURL, FromBytes.
type InputFile struct {
	// FileID references an existing file on Telegram servers.
	FileID string

	// URL references a file by HTTP URL (Telegram will download).
	URL string

	// Reader provides file content for upload.
	// Content is streamed directly - not buffered in memory.
	// WARNING: io.Reader can only be consumed once. If the request is retried
	// (e.g., on 429/5xx), the retry will send an empty file. Prefer FromBytes
	// for retry-safe uploads, or set Source instead.
	Reader io.Reader

	// Source is a factory that returns a fresh io.Reader for each attempt.
	// When set, this takes priority over Reader for multipart uploads,
	// making the request retry-safe.
	Source func() io.Reader

	// FileName is required when Reader or Source is set.
	FileName string

	// MediaType is used for media groups (e.g., "photo", "video").
	MediaType string

	// Caption for media items.
	Caption string

	// ParseMode for caption (HTML, Markdown, MarkdownV2).
	ParseMode string
}

// FromReader creates an InputFile from an io.Reader.
// The reader is streamed directly - not buffered in memory.
// WARNING: Not retry-safe. If the request is retried, the reader will be at EOF.
// Use FromBytes for retry-safe uploads from in-memory data.
func FromReader(r io.Reader, filename string) InputFile {
	return InputFile{
		Reader:   r,
		FileName: filename,
	}
}

// FromBytes creates a retry-safe InputFile from in-memory bytes.
// Each request attempt gets a fresh reader, so retries work correctly.
func FromBytes(data []byte, filename string) InputFile {
	return InputFile{
		Source: func() io.Reader {
			return bytes.NewReader(data)
		},
		FileName: filename,
	}
}

// FromFileID creates an InputFile referencing an existing Telegram file.
func FromFileID(fileID string) InputFile {
	return InputFile{FileID: fileID}
}

// FromURL creates an InputFile from a URL (Telegram will download).
func FromURL(url string) InputFile {
	return InputFile{URL: url}
}

// IsUpload returns true if this InputFile requires upload (has Reader or Source).
func (f InputFile) IsUpload() bool {
	return f.Reader != nil || f.Source != nil
}

// IsEmpty returns true if the InputFile has no value set.
func (f InputFile) IsEmpty() bool {
	return f.FileID == "" && f.URL == "" && f.Reader == nil && f.Source == nil
}

// OpenReader returns a reader for the file content.
// If Source is set, returns a fresh reader (retry-safe).
// Otherwise returns Reader directly (single-use).
func (f InputFile) OpenReader() io.Reader {
	if f.Source != nil {
		return f.Source()
	}
	return f.Reader
}

// Value returns the string value for JSON serialization (FileID or URL).
// Returns empty string if this is an upload (Reader-based).
func (f InputFile) Value() string {
	if f.FileID != "" {
		return f.FileID
	}
	if f.URL != "" {
		return f.URL
	}
	return ""
}

// WithCaption returns a copy with the caption set.
func (f InputFile) WithCaption(caption string) InputFile {
	f.Caption = caption
	return f
}

// WithParseMode returns a copy with the parse mode set.
func (f InputFile) WithParseMode(mode string) InputFile {
	f.ParseMode = mode
	return f
}

// WithMediaType returns a copy with the media type set.
func (f InputFile) WithMediaType(mediaType string) InputFile {
	f.MediaType = mediaType
	return f
}

// MarshalJSON returns the string value (URL or FileID) for JSON encoding.
// For uploads (Reader-based), this returns empty string as those use multipart.
func (f InputFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Value())
}
