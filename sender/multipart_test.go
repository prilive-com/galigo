package sender_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultipartEncoder_BasicFields(t *testing.T) {
	var buf bytes.Buffer
	enc := sender.NewMultipartEncoder(&buf)

	req := sender.MultipartRequest{
		Params: map[string]string{
			"chat_id": "123456",
			"text":    "Hello, World!",
		},
	}

	err := enc.Encode(req)
	require.NoError(t, err)

	err = enc.Close()
	require.NoError(t, err)

	contentType := enc.ContentType()
	assert.Contains(t, contentType, "multipart/form-data")
	assert.Contains(t, contentType, "boundary=")
}

func TestMultipartEncoder_WithFile(t *testing.T) {
	var buf bytes.Buffer
	enc := sender.NewMultipartEncoder(&buf)

	fileContent := "test file content"
	req := sender.MultipartRequest{
		Files: []sender.FilePart{
			{
				FieldName: "document",
				FileName:  "test.txt",
				Reader:    strings.NewReader(fileContent),
			},
		},
		Params: map[string]string{
			"chat_id": "123456",
		},
	}

	err := enc.Encode(req)
	require.NoError(t, err)

	err = enc.Close()
	require.NoError(t, err)

	body := buf.String()
	assert.Contains(t, body, "test file content")
	assert.Contains(t, body, "test.txt")
	assert.Contains(t, body, "document")
}

func TestBuildMultipartRequest_FileID(t *testing.T) {
	type TestRequest struct {
		ChatID   int64            `json:"chat_id"`
		Document sender.InputFile `json:"document"`
	}

	req := TestRequest{
		ChatID:   123456,
		Document: sender.FromFileID("AgACAgIAAxkBAAI"),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.False(t, result.HasUploads())
	assert.Equal(t, "123456", result.Params["chat_id"])
	assert.Equal(t, "AgACAgIAAxkBAAI", result.Params["document"])
}

func TestBuildMultipartRequest_URL(t *testing.T) {
	type TestRequest struct {
		ChatID int64            `json:"chat_id"`
		Photo  sender.InputFile `json:"photo"`
	}

	req := TestRequest{
		ChatID: 123456,
		Photo:  sender.FromURL("https://example.com/photo.jpg"),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.False(t, result.HasUploads())
	assert.Equal(t, "https://example.com/photo.jpg", result.Params["photo"])
}

func TestBuildMultipartRequest_Upload(t *testing.T) {
	type TestRequest struct {
		ChatID   int64            `json:"chat_id"`
		Document sender.InputFile `json:"document"`
	}

	reader := strings.NewReader("file content")
	req := TestRequest{
		ChatID:   123456,
		Document: sender.FromReader(reader, "test.txt"),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.True(t, result.HasUploads())
	// For single file uploads, the file goes directly in the field (no attach://)
	assert.NotContains(t, result.Params, "document") // File is in Files, not Params
	require.Len(t, result.Files, 1)
	assert.Equal(t, "document", result.Files[0].FieldName) // Field name matches JSON tag
	assert.Equal(t, "test.txt", result.Files[0].FileName)

	// Verify reader is passed correctly
	content, _ := io.ReadAll(result.Files[0].Reader)
	assert.Equal(t, "file content", string(content))
}

func TestBuildMultipartRequest_MultipleUploads(t *testing.T) {
	type TestRequest struct {
		ChatID    int64            `json:"chat_id"`
		Photo     sender.InputFile `json:"photo"`
		Thumbnail sender.InputFile `json:"thumbnail"`
	}

	req := TestRequest{
		ChatID:    123456,
		Photo:     sender.FromReader(strings.NewReader("photo"), "photo.jpg"),
		Thumbnail: sender.FromReader(strings.NewReader("thumb"), "thumb.jpg"),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.True(t, result.HasUploads())
	// For single file uploads, files go directly in their fields (no attach://)
	assert.NotContains(t, result.Params, "photo")
	assert.NotContains(t, result.Params, "thumbnail")
	require.Len(t, result.Files, 2)
	assert.Equal(t, "photo", result.Files[0].FieldName)
	assert.Equal(t, "thumbnail", result.Files[1].FieldName)
}

func TestBuildMultipartRequest_InputFileSlice(t *testing.T) {
	type TestRequest struct {
		ChatID int64              `json:"chat_id"`
		Media  []sender.InputFile `json:"media"`
	}

	req := TestRequest{
		ChatID: 123456,
		Media: []sender.InputFile{
			sender.FromFileID("file1").WithMediaType("photo").WithCaption("First"),
			sender.FromURL("https://example.com/video.mp4").WithMediaType("video"),
			sender.FromReader(strings.NewReader("photo"), "local.jpg").WithMediaType("photo"),
		},
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.True(t, result.HasUploads())
	require.Len(t, result.Files, 1)

	// Check the media JSON
	mediaJSON := result.Params["media"]
	assert.Contains(t, mediaJSON, "file1")
	assert.Contains(t, mediaJSON, "https://example.com/video.mp4")
	assert.Contains(t, mediaJSON, "attach://file0")
	assert.Contains(t, mediaJSON, "\"caption\":\"First\"")
}

func TestBuildMultipartRequest_PrimitiveTypes(t *testing.T) {
	type TestRequest struct {
		ChatID       int64   `json:"chat_id"`
		Text         string  `json:"text"`
		DisableNotif bool    `json:"disable_notification"`
		Latitude     float64 `json:"latitude"`
	}

	req := TestRequest{
		ChatID:       123456,
		Text:         "Hello",
		DisableNotif: true,
		Latitude:     51.5074,
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.Equal(t, "123456", result.Params["chat_id"])
	assert.Equal(t, "Hello", result.Params["text"])
	assert.Equal(t, "true", result.Params["disable_notification"])
	assert.Equal(t, "51.5074", result.Params["latitude"])
}

func TestBuildMultipartRequest_ComplexType(t *testing.T) {
	type ReplyMarkup struct {
		Buttons [][]string `json:"keyboard"`
	}

	type TestRequest struct {
		ChatID      int64       `json:"chat_id"`
		ReplyMarkup ReplyMarkup `json:"reply_markup"`
	}

	req := TestRequest{
		ChatID: 123456,
		ReplyMarkup: ReplyMarkup{
			Buttons: [][]string{{"A", "B"}, {"C", "D"}},
		},
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.Contains(t, result.Params["reply_markup"], "keyboard")
	assert.Contains(t, result.Params["reply_markup"], "[\"A\",\"B\"]")
}

func TestBuildMultipartRequest_SkipsZeroValues(t *testing.T) {
	type TestRequest struct {
		ChatID    int64  `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}

	req := TestRequest{
		ChatID: 123456,
		Text:   "Hello",
		// ParseMode is zero value
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	_, hasParseMode := result.Params["parse_mode"]
	assert.False(t, hasParseMode, "should skip zero value fields")
}

func TestBuildMultipartRequest_SkipsUnexported(t *testing.T) {
	type TestRequest struct {
		ChatID     int64  `json:"chat_id"`
		unexported string //nolint:unused
	}

	req := TestRequest{
		ChatID: 123456,
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.Equal(t, "123456", result.Params["chat_id"])
	assert.Len(t, result.Params, 1)
}

func TestBuildMultipartRequest_JSONTagDash(t *testing.T) {
	type TestRequest struct {
		ChatID  int64  `json:"chat_id"`
		Ignored string `json:"-"`
	}

	req := TestRequest{
		ChatID:  123456,
		Ignored: "should be ignored",
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	_, hasIgnored := result.Params["Ignored"]
	assert.False(t, hasIgnored)
	_, hasDash := result.Params["-"]
	assert.False(t, hasDash)
}

func TestBuildMultipartRequest_EmptyInputFile_Error(t *testing.T) {
	type TestRequest struct {
		ChatID int64            `json:"chat_id"`
		Photo  sender.InputFile `json:"photo"`
	}

	req := TestRequest{
		ChatID: 123456,
		Photo:  sender.InputFile{}, // Empty
	}

	// Empty InputFile is zero value, should be skipped
	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)
	_, hasPhoto := result.Params["photo"]
	assert.False(t, hasPhoto)
}

func TestMultipartRequest_HasUploads(t *testing.T) {
	withUploads := sender.MultipartRequest{
		Files: []sender.FilePart{{FieldName: "test"}},
	}
	assert.True(t, withUploads.HasUploads())

	withoutUploads := sender.MultipartRequest{
		Params: map[string]string{"chat_id": "123"},
	}
	assert.False(t, withoutUploads.HasUploads())
}
