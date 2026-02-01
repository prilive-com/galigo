package sender_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== GetStickerSet ====================

func TestGetStickerSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getStickerSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"name":         "test_stickers",
			"title":        "Test Stickers",
			"sticker_type": "regular",
			"stickers":     []map[string]any{},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	set, err := client.GetStickerSet(context.Background(), "test_stickers")
	require.NoError(t, err)
	assert.Equal(t, "test_stickers", set.Name)
	assert.Equal(t, "Test Stickers", set.Title)
	assert.Equal(t, "regular", set.StickerType)
}

func TestGetStickerSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetStickerSet(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

// ==================== GetCustomEmojiStickers ====================

func TestGetCustomEmojiStickers(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getCustomEmojiStickers", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]any{
			{"file_id": "sticker1", "file_unique_id": "u1", "type": "custom_emoji", "width": 100, "height": 100, "is_animated": false, "is_video": false},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	stickers, err := client.GetCustomEmojiStickers(context.Background(), []string{"emoji1"})
	require.NoError(t, err)
	require.Len(t, stickers, 1)
	assert.Equal(t, "sticker1", stickers[0].FileID)
}

func TestGetCustomEmojiStickers_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{"empty", []string{}, "at least one"},
		{"too many", make([]string, 201), "at most 200"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetCustomEmojiStickers(context.Background(), tt.ids)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== UploadStickerFile ====================

func TestUploadStickerFile(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/uploadStickerFile", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"file_id":        "file_abc",
			"file_unique_id": "unique_abc",
			"file_size":      1024,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	file, err := client.UploadStickerFile(context.Background(), sender.UploadStickerFileRequest{
		UserID:        123,
		Sticker:       sender.FromBytes([]byte("fake png data"), "sticker.png"),
		StickerFormat: "static",
	})
	require.NoError(t, err)
	assert.Equal(t, "file_abc", file.FileID)
}

func TestUploadStickerFile_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.UploadStickerFileRequest
		want string
	}{
		{"missing user_id", sender.UploadStickerFileRequest{Sticker: sender.FromBytes([]byte("x"), "s.png"), StickerFormat: "static"}, "user_id"},
		{"missing sticker", sender.UploadStickerFileRequest{UserID: 1, StickerFormat: "static"}, "sticker"},
		{"missing format", sender.UploadStickerFileRequest{UserID: 1, Sticker: sender.FromBytes([]byte("x"), "s.png")}, "sticker_format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.UploadStickerFile(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== CreateNewStickerSet ====================

func TestCreateNewStickerSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createNewStickerSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.CreateNewStickerSet(context.Background(), sender.CreateNewStickerSetRequest{
		UserID: 123,
		Name:   "test_by_bot",
		Title:  "Test Set",
		Stickers: []sender.InputSticker{
			{
				Sticker:   sender.FromFileID("file_abc"),
				Format:    "static",
				EmojiList: []string{"😀"},
			},
		},
	})
	require.NoError(t, err)
}

func TestCreateNewStickerSet_WithUpload(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createNewStickerSet", func(w http.ResponseWriter, r *http.Request) {
		// Verify multipart upload
		assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.CreateNewStickerSet(context.Background(), sender.CreateNewStickerSetRequest{
		UserID: 123,
		Name:   "test_by_bot",
		Title:  "Test Set",
		Stickers: []sender.InputSticker{
			{
				Sticker:   sender.FromBytes([]byte("fake png"), "sticker.png"),
				Format:    "static",
				EmojiList: []string{"😀"},
			},
		},
	})
	require.NoError(t, err)
}

func TestCreateNewStickerSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.CreateNewStickerSetRequest
		want string
	}{
		{"missing user_id", sender.CreateNewStickerSetRequest{Name: "n", Title: "t", Stickers: []sender.InputSticker{{Sticker: sender.FromFileID("f"), Format: "static", EmojiList: []string{"😀"}}}}, "user_id"},
		{"missing name", sender.CreateNewStickerSetRequest{UserID: 1, Title: "t", Stickers: []sender.InputSticker{{Sticker: sender.FromFileID("f"), Format: "static", EmojiList: []string{"😀"}}}}, "name"},
		{"missing title", sender.CreateNewStickerSetRequest{UserID: 1, Name: "n", Stickers: []sender.InputSticker{{Sticker: sender.FromFileID("f"), Format: "static", EmojiList: []string{"😀"}}}}, "title"},
		{"no stickers", sender.CreateNewStickerSetRequest{UserID: 1, Name: "n", Title: "t"}, "stickers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CreateNewStickerSet(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== AddStickerToSet ====================

func TestAddStickerToSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/addStickerToSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AddStickerToSet(context.Background(), sender.AddStickerToSetRequest{
		UserID: 123,
		Name:   "test_by_bot",
		Sticker: sender.InputSticker{
			Sticker:   sender.FromFileID("file_abc"),
			Format:    "static",
			EmojiList: []string{"😀"},
		},
	})
	require.NoError(t, err)
}

func TestAddStickerToSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, _ = server, client
	err := client.AddStickerToSet(context.Background(), sender.AddStickerToSetRequest{
		Name: "test",
		Sticker: sender.InputSticker{
			Sticker:   sender.FromFileID("f"),
			Format:    "static",
			EmojiList: []string{"😀"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id")
}

// ==================== SetStickerPositionInSet ====================

func TestSetStickerPositionInSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerPositionInSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerPositionInSet(context.Background(), sender.SetStickerPositionInSetRequest{
		Sticker:  "file_abc",
		Position: 0, // Position 0 is valid
	})
	require.NoError(t, err)
}

func TestSetStickerPositionInSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.SetStickerPositionInSet(context.Background(), sender.SetStickerPositionInSetRequest{Position: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sticker")
}

// ==================== DeleteStickerFromSet ====================

func TestDeleteStickerFromSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteStickerFromSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.DeleteStickerFromSet(context.Background(), "file_abc")
	require.NoError(t, err)
}

func TestDeleteStickerFromSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.DeleteStickerFromSet(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sticker")
}

// ==================== SetStickerSetTitle ====================

func TestSetStickerSetTitle(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerSetTitle", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerSetTitle(context.Background(), "test_by_bot", "New Title")
	require.NoError(t, err)
}

func TestSetStickerSetTitle_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name, sName, title, want string
	}{
		{"missing name", "", "title", "name"},
		{"missing title", "name", "", "title"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetStickerSetTitle(context.Background(), tt.sName, tt.title)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== DeleteStickerSet ====================

func TestDeleteStickerSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteStickerSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.DeleteStickerSet(context.Background(), "test_by_bot")
	require.NoError(t, err)
}

// ==================== SetStickerSetThumbnail ====================

func TestSetStickerSetThumbnail(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerSetThumbnail", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerSetThumbnail(context.Background(), sender.SetStickerSetThumbnailRequest{
		Name:   "test_by_bot",
		UserID: 123,
		Format: "static",
	})
	require.NoError(t, err)
}

func TestSetStickerSetThumbnail_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetStickerSetThumbnailRequest
		want string
	}{
		{"missing name", sender.SetStickerSetThumbnailRequest{UserID: 1, Format: "static"}, "name"},
		{"missing user_id", sender.SetStickerSetThumbnailRequest{Name: "n", Format: "static"}, "user_id"},
		{"missing format", sender.SetStickerSetThumbnailRequest{Name: "n", UserID: 1}, "format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetStickerSetThumbnail(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetStickerEmojiList ====================

func TestSetStickerEmojiList(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerEmojiList", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerEmojiList(context.Background(), sender.SetStickerEmojiListRequest{
		Sticker:   "file_abc",
		EmojiList: []string{"😀", "😁"},
	})
	require.NoError(t, err)
}

func TestSetStickerEmojiList_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SetStickerEmojiListRequest
		want string
	}{
		{"missing sticker", sender.SetStickerEmojiListRequest{EmojiList: []string{"😀"}}, "sticker"},
		{"empty emoji_list", sender.SetStickerEmojiListRequest{Sticker: "f"}, "emoji_list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetStickerEmojiList(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== SetStickerKeywords ====================

func TestSetStickerKeywords(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerKeywords", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerKeywords(context.Background(), sender.SetStickerKeywordsRequest{
		Sticker:  "file_abc",
		Keywords: []string{"happy", "smile"},
	})
	require.NoError(t, err)
}

// ==================== SetStickerMaskPosition ====================

func TestSetStickerMaskPosition(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setStickerMaskPosition", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetStickerMaskPosition(context.Background(), sender.SetStickerMaskPositionRequest{
		Sticker: "file_abc",
		MaskPosition: &tg.MaskPosition{
			Point:  "forehead",
			XShift: 0.5,
			YShift: 0.5,
			Scale:  1.0,
		},
	})
	require.NoError(t, err)
}

// ==================== ReplaceStickerInSet ====================

func TestReplaceStickerInSet(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/replaceStickerInSet", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.ReplaceStickerInSet(context.Background(), sender.ReplaceStickerInSetRequest{
		UserID:     123,
		Name:       "test_by_bot",
		OldSticker: "old_file_id",
		Sticker: sender.InputSticker{
			Sticker:   sender.FromFileID("new_file_id"),
			Format:    "static",
			EmojiList: []string{"😀"},
		},
	})
	require.NoError(t, err)
}

func TestReplaceStickerInSet_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	sticker := sender.InputSticker{Sticker: sender.FromFileID("f"), Format: "static", EmojiList: []string{"😀"}}

	tests := []struct {
		name string
		req  sender.ReplaceStickerInSetRequest
		want string
	}{
		{"missing user_id", sender.ReplaceStickerInSetRequest{Name: "n", OldSticker: "o", Sticker: sticker}, "user_id"},
		{"missing name", sender.ReplaceStickerInSetRequest{UserID: 1, OldSticker: "o", Sticker: sticker}, "name"},
		{"missing old_sticker", sender.ReplaceStickerInSetRequest{UserID: 1, Name: "n", Sticker: sticker}, "old_sticker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ReplaceStickerInSet(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
