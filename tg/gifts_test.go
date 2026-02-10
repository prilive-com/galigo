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

// ==================== UniqueGift Types (Bot API 9.0-9.4) ====================

func TestUniqueGiftModel_Rarity(t *testing.T) {
	raw := `{"name":"Star","sticker":{"file_id":"s1","file_unique_id":"su1","type":"custom_emoji","width":100,"height":100,"is_animated":false,"is_video":false},"rarity_per_mille":50,"rarity":"legendary"}`
	var m UniqueGiftModel
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	assert.Equal(t, "legendary", m.Rarity)
	assert.Equal(t, 50, m.RarityPerMille)
}

func TestUniqueGift_IsBurned(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #1","number":1,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":100},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":200},"backdrop":{"name":"B","colors":{"center_color":16777215,"edge_color":0,"symbol_color":11184810,"text_color":1118481},"rarity_per_mille":300},"is_burned":true}`
	var g UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	assert.True(t, g.IsBurned)
	// Verify color fields are int (RGB), not strings
	assert.Equal(t, 16777215, g.Backdrop.Colors.CenterColor) // 0xFFFFFF
	assert.Equal(t, 0, g.Backdrop.Colors.EdgeColor)          // 0x000000
	assert.Equal(t, 11184810, g.Backdrop.Colors.SymbolColor) // 0xAAAAAA
	// Verify rarity_per_mille on all sub-types
	assert.Equal(t, 100, g.Model.RarityPerMille)
	assert.Equal(t, 200, g.Symbol.RarityPerMille)
	assert.Equal(t, 300, g.Backdrop.RarityPerMille)
}

func TestUniqueGift_WithColors(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #2","number":2,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"backdrop":{"name":"B","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0},"colors":{"model_custom_emoji_id":"5368324170671202286","symbol_custom_emoji_id":"5368324170671202287","light_theme_main_color":16711680,"light_theme_other_colors":[65280,255],"dark_theme_main_color":8388608,"dark_theme_other_colors":[32768,128,64]}}`
	var g UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	// Colors is a singular pointer, not a slice
	require.NotNil(t, g.Colors)
	assert.Equal(t, "5368324170671202286", g.Colors.ModelCustomEmojiID)
	assert.Equal(t, 16711680, g.Colors.LightThemeMainColor) // 0xFF0000 (red)
	assert.Len(t, g.Colors.LightThemeOtherColors, 2)
	assert.Len(t, g.Colors.DarkThemeOtherColors, 3) // up to 3 allowed
}

func TestUniqueGift_WithoutColors(t *testing.T) {
	raw := `{"base_name":"Gift","name":"Gift #3","number":3,"model":{"name":"M","sticker":{"file_id":"s","file_unique_id":"su","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"symbol":{"name":"S","sticker":{"file_id":"s2","file_unique_id":"su2","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":0},"backdrop":{"name":"B","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0}}`
	var g UniqueGift
	require.NoError(t, json.Unmarshal([]byte(raw), &g))
	assert.Nil(t, g.Colors) // colors is optional
}

func TestGiftRarityConstants(t *testing.T) {
	assert.Equal(t, "uncommon", GiftRarityUncommon)
	assert.Equal(t, "rare", GiftRarityRare)
	assert.Equal(t, "epic", GiftRarityEpic)
	assert.Equal(t, "legendary", GiftRarityLegendary)
}
