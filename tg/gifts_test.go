package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGift_Unmarshal(t *testing.T) {
	data := `{"id":"gift_1","sticker":{"file_id":"f1","file_unique_id":"u1","type":"regular","width":512,"height":512,"is_animated":false,"is_video":false},"star_count":50,"total_count":100,"remaining_count":42}`

	var g Gift
	require.NoError(t, json.Unmarshal([]byte(data), &g))
	assert.Equal(t, "gift_1", g.ID)
	assert.Equal(t, 50, g.StarCount)
	assert.Equal(t, 100, g.TotalCount)
	assert.Equal(t, 42, g.RemainingCount)
}

func TestOwnedGift_Unmarshal(t *testing.T) {
	data := `{"type":"regular","owned_gift_id":"og_1","send_date":1700000000,"is_saved":true,"convert_star_count":25}`

	var g OwnedGift
	require.NoError(t, json.Unmarshal([]byte(data), &g))
	assert.Equal(t, "regular", g.Type)
	assert.Equal(t, "og_1", g.OwnedGiftID)
	assert.True(t, g.IsSaved)
	assert.Equal(t, 25, g.ConvertStarCount)
}

func TestAcceptedGiftTypes_Unmarshal(t *testing.T) {
	data := `{"unlimited_gifts":true,"unique_gifts":true}`

	var a AcceptedGiftTypes
	require.NoError(t, json.Unmarshal([]byte(data), &a))
	assert.True(t, a.UnlimitedGifts)
	assert.False(t, a.LimitedGifts)
	assert.True(t, a.UniqueGifts)
}
