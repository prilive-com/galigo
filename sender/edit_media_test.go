package sender_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
)

// W4 (B-45): a FileID-based edit is sent as multipart/form-data, because the
// Bot API server reads the media object only from a form field. The old
// assertion (application/json) checked the transport of the defect.
func TestEditMessageMedia_FileID(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageMedia", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 42)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 42,
		Media: sender.NewInputMediaPhoto(sender.FromFileID("AgACAgIAAxkBAAI")).
			WithCaption("updated caption").WithParseMode("HTML"),
	})

	require.NoError(t, err)
	assert.Equal(t, 42, msg.MessageID)

	require.Equal(t, 1, server.CaptureCount())
	mediaType, args, files := wireArgs(t, capturedRequest(server.LastCapture()))
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":    wireChatID,
		"message_id": "42",
		"media":      `{"type":"photo","media":"AgACAgIAAxkBAAI","caption":"updated caption","parse_mode":"HTML"}`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestEditMessageMedia_Upload(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageMedia", func(w http.ResponseWriter, r *http.Request) {
		// Upload should use multipart
		ct := r.Header.Get("Content-Type")
		assert.Contains(t, ct, "multipart/form-data")
		testutil.ReplyMessage(w, 42)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	msg, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 42,
		Media: sender.NewInputMediaDocument(
			sender.FromReader(strings.NewReader("new file content"), "replaced.txt"),
		).WithCaption("replaced via editMessageMedia"),
	})

	require.NoError(t, err)
	assert.Equal(t, 42, msg.MessageID)

	// W9 (B-45): the whole upload request is unchanged.
	require.Equal(t, 1, server.CaptureCount())
	_, args, files := wireArgs(t, capturedRequest(server.LastCapture()))
	assertArgs(t, map[string]string{
		"chat_id":    wireChatID,
		"message_id": "42",
		"media":      `{"type":"document","media":"attach://file0","caption":"replaced via editMessageMedia"}`,
	}, args)
	assertFiles(t, map[string]wireFile{"file0": {FileName: "replaced.txt", Data: []byte("new file content")}}, files)
}

func TestEditMessageMedia_Error(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/editMessageMedia", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyError(w, 400, "Bad Request: message to edit not found", nil)
	})

	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{
		ChatID:    testutil.TestChatID,
		MessageID: 999,
		Media:     sender.NewInputMediaPhoto(sender.FromFileID("AgACAgIAAxkBAAI")),
	})

	require.Error(t, err)
}

func TestBuildMultipartRequest_InputMedia_FileID(t *testing.T) {
	type TestRequest struct {
		ChatID    int64             `json:"chat_id"`
		MessageID int               `json:"message_id"`
		Media     sender.InputMedia `json:"media"`
	}

	req := TestRequest{
		ChatID:    123456,
		MessageID: 42,
		Media: sender.NewInputMediaPhoto(sender.FromFileID("AgACAgIAAxkBAAI")).
			WithCaption("test caption"),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.False(t, result.HasUploads())
	mediaJSON := result.Params["media"]
	assert.Contains(t, mediaJSON, `"type":"photo"`)
	assert.Contains(t, mediaJSON, `"media":"AgACAgIAAxkBAAI"`)
	assert.Contains(t, mediaJSON, `"caption":"test caption"`)
}

func TestBuildMultipartRequest_InputMedia_Upload(t *testing.T) {
	type TestRequest struct {
		ChatID    int64             `json:"chat_id"`
		MessageID int               `json:"message_id"`
		Media     sender.InputMedia `json:"media"`
	}

	req := TestRequest{
		ChatID:    123456,
		MessageID: 42,
		Media: sender.NewInputMediaDocument(
			sender.FromReader(strings.NewReader("doc content"), "test.txt"),
		),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.True(t, result.HasUploads())
	require.Len(t, result.Files, 1)
	assert.Equal(t, "file0", result.Files[0].FieldName)
	assert.Equal(t, "test.txt", result.Files[0].FileName)

	content, _ := io.ReadAll(result.Files[0].Reader)
	assert.Equal(t, "doc content", string(content))

	mediaJSON := result.Params["media"]
	assert.Contains(t, mediaJSON, `"type":"document"`)
	assert.Contains(t, mediaJSON, `"media":"attach://file0"`)
}

func TestBuildMultipartRequest_InputMedia_URL(t *testing.T) {
	type TestRequest struct {
		ChatID int64             `json:"chat_id"`
		Media  sender.InputMedia `json:"media"`
	}

	req := TestRequest{
		ChatID: 123456,
		Media:  sender.NewInputMediaPhoto(sender.FromURL("https://example.com/photo.jpg")),
	}

	result, err := sender.BuildMultipartRequest(req)
	require.NoError(t, err)

	assert.False(t, result.HasUploads())
	mediaJSON := result.Params["media"]
	assert.Contains(t, mediaJSON, `"media":"https://example.com/photo.jpg"`)
}
