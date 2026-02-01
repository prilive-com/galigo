package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGameHighScore_Unmarshal(t *testing.T) {
	data := `{"position":1,"user":{"id":456,"is_bot":false,"first_name":"Alice"},"score":100}`

	var ghs GameHighScore
	require.NoError(t, json.Unmarshal([]byte(data), &ghs))
	assert.Equal(t, 1, ghs.Position)
	assert.Equal(t, 100, ghs.Score)
	assert.Equal(t, "Alice", ghs.User.FirstName)
}

func TestStickerSet_Unmarshal(t *testing.T) {
	data := `{"name":"test_set","title":"Test","sticker_type":"regular","is_animated":false,"is_video":true,"stickers":[]}`

	var ss StickerSet
	require.NoError(t, json.Unmarshal([]byte(data), &ss))
	assert.Equal(t, "test_set", ss.Name)
	assert.False(t, ss.IsAnimated)
	assert.True(t, ss.IsVideo)
}
