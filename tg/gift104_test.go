package tg_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-65 65i: the Bot API 10.4 unique gift and gift fields (TEST-PLAN section 2). Fake values only.

const (
	b65iModel    = `"model":{"name":"m","sticker":{"file_id":"f1","file_unique_id":"u1","type":"custom_emoji","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":1}`
	b65iSymbol   = `"symbol":{"name":"s","sticker":{"file_id":"f2","file_unique_id":"u2","type":"custom_emoji","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":2}`
	b65iBackdrop = `"backdrop":{"name":"b","colors":{"center_color":1,"edge_color":2,"symbol_color":3,"text_color":4},"rarity_per_mille":3}`
	b65iColors   = `"colors":{"model_custom_emoji_id":"me","symbol_custom_emoji_id":"se","light_theme_main_color":5,"light_theme_other_colors":[6],"dark_theme_main_color":7,"dark_theme_other_colors":[8]}`
	b65iPub      = `"publisher_chat":{"id":-100777,"type":"channel","title":"pub"}`
	b65iHead     = `{"gift_id":"5936013938331222567","base_name":"Plush Pepe","name":"PlushPepe-1","number":1,` + b65iModel + `,` + b65iSymbol + `,` + b65iBackdrop

	// U1: every required key, nothing optional.
	b65iU1 = b65iHead + `}`
	// UF: U1 plus is_premium, colors, is_burned and publisher_chat (no is_from_blockchain).
	b65iUF = b65iHead + `,"is_premium":true,` + b65iColors + `,"is_burned":true,` + b65iPub + `}`
	// UB: U1 plus is_from_blockchain only.
	b65iUB = b65iHead + `,"is_from_blockchain":true}`
	// GF: a regular gift with a publisher chat.
	b65iGF = `{"id":"g-fake-7","sticker":{"file_id":"f3","file_unique_id":"u3","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"star_count":5,` + b65iPub + `}`
)

func b65iPubChat() *tg.Chat { return &tg.Chat{ID: -100777, Type: "channel", Title: "pub"} }

func b65iU1Value() tg.UniqueGift {
	return tg.UniqueGift{GiftID: "5936013938331222567", BaseName: "Plush Pepe", Name: "PlushPepe-1", Number: 1,
		Model: tg.UniqueGiftModel{Name: "m", Sticker: tg.Sticker{FileID: "f1", FileUniqueID: "u1", Type: "custom_emoji",
			Width: 1, Height: 1}, RarityPerMille: 1},
		Symbol: tg.UniqueGiftSymbol{Name: "s", Sticker: tg.Sticker{FileID: "f2", FileUniqueID: "u2", Type: "custom_emoji",
			Width: 1, Height: 1}, RarityPerMille: 2},
		Backdrop: tg.UniqueGiftBackdrop{Name: "b", Colors: tg.UniqueGiftBackdropColors{CenterColor: 1, EdgeColor: 2,
			SymbolColor: 3, TextColor: 4}, RarityPerMille: 3}}
}

func b65iUFValue() tg.UniqueGift {
	g := b65iU1Value()
	g.IsPremium = true
	g.Colors = &tg.UniqueGiftColors{ModelCustomEmojiID: "me", SymbolCustomEmojiID: "se", LightThemeMainColor: 5,
		LightThemeOtherColors: []int{6}, DarkThemeMainColor: 7, DarkThemeOtherColors: []int{8}}
	g.IsBurned = true
	g.PublisherChat = b65iPubChat()
	return g
}

func b65iUBValue() tg.UniqueGift {
	g := b65iU1Value()
	g.IsFromBlockchain = true
	return g
}

// T-ug-decode, T-ug-encode: each fixture decodes to its exact value and encodes back to the same bytes.
func TestB65I_UniqueGiftCodec(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want tg.UniqueGift
	}{
		{"U1", b65iU1, b65iU1Value()},
		{"UF", b65iUF, b65iUFValue()},
		{"UB", b65iUB, b65iUBValue()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got tg.UniqueGift
			require.NoError(t, json.Unmarshal([]byte(tc.in), &got))
			assert.Equal(t, tc.want, got)
			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.Equal(t, tc.in, string(out))
		})
	}
	t.Run("zero", func(t *testing.T) {
		out, err := json.Marshal(tg.UniqueGift{})
		require.NoError(t, err)
		assert.Equal(t, `{"gift_id":"","base_name":"","name":"","number":0,`+
			`"model":{"name":"","sticker":{"file_id":"","file_unique_id":"","type":"","width":0,"height":0,"is_animated":false,"is_video":false},"rarity_per_mille":0},`+
			`"symbol":{"name":"","sticker":{"file_id":"","file_unique_id":"","type":"","width":0,"height":0,"is_animated":false,"is_video":false},"rarity_per_mille":0},`+
			`"backdrop":{"name":"","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0}}`, string(out))
	})
}

// T-gift: the regular gift's publisher_chat.
func TestB65I_GiftCodec(t *testing.T) {
	var g tg.Gift
	require.NoError(t, json.Unmarshal([]byte(b65iGF), &g))
	assert.Equal(t, tg.Gift{ID: "g-fake-7", Sticker: tg.Sticker{FileID: "f3", FileUniqueID: "u3", Type: "regular",
		Width: 1, Height: 1}, StarCount: 5, PublisherChat: b65iPubChat()}, g)
	out, err := json.Marshal(g)
	require.NoError(t, err)
	assert.Equal(t, b65iGF, string(out))
	g.PublisherChat = nil
	out, err = json.Marshal(g)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "publisher_chat")
}

type b65iField struct {
	Name, Type, Tag string
	Anonymous       bool
}

func b65iFields(v any) []b65iField {
	rt := reflect.TypeOf(v)
	out := make([]b65iField, 0, rt.NumField())
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, b65iField{f.Name, f.Type.String(), f.Tag.Get("json"), f.Anonymous})
	}
	return out
}

// T-shapes: every field of UniqueGift and Gift, in order, with its type, tag and embedding.
func TestB65I_Shapes(t *testing.T) {
	assert.Equal(t, []b65iField{
		{"GiftID", "string", "gift_id", false},
		{"BaseName", "string", "base_name", false},
		{"Name", "string", "name", false},
		{"Number", "int", "number", false},
		{"Model", "tg.UniqueGiftModel", "model", false},
		{"Symbol", "tg.UniqueGiftSymbol", "symbol", false},
		{"Backdrop", "tg.UniqueGiftBackdrop", "backdrop", false},
		{"IsPremium", "bool", "is_premium,omitempty", false},
		{"Colors", "*tg.UniqueGiftColors", "colors,omitempty", false},
		{"IsBurned", "bool", "is_burned,omitempty", false},
		{"IsFromBlockchain", "bool", "is_from_blockchain,omitempty", false},
		{"PublisherChat", "*tg.Chat", "publisher_chat,omitempty", false},
	}, b65iFields(tg.UniqueGift{}))
	assert.Equal(t, []b65iField{
		{"ID", "string", "id", false},
		{"Sticker", "tg.Sticker", "sticker", false},
		{"StarCount", "int", "star_count", false},
		{"UpgradeStarCount", "int", "upgrade_star_count,omitempty", false},
		{"TotalCount", "int", "total_count,omitempty", false},
		{"RemainingCount", "int", "remaining_count,omitempty", false},
		{"PublisherChat", "*tg.Chat", "publisher_chat,omitempty", false},
	}, b65iFields(tg.Gift{}))
}

// T-bad: a wrong JSON type in a new field fails the decode, with the field named (PLAN AC5, O-P2-06).
func TestB65I_BadTypes(t *testing.T) {
	badID := `{"gift_id":7,"base_name":"bn","name":"gn","number":1}`
	for _, tc := range []struct {
		name, in, field string
		into            any
	}{
		{"unique_gift", badID, "gift_id", &tg.UniqueGift{}},
		{"share_theme", `{"type":"unique_gift","gift":` + badID + `}`, "gift.gift_id", &tg.ChatShareTheme{}},
		{"gift", `{"id":"g","sticker":{"file_id":"f","file_unique_id":"u","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"star_count":1,"publisher_chat":"x"}`, "publisher_chat", &tg.Gift{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.in), tc.into)
			var te *json.UnmarshalTypeError
			require.True(t, errors.As(err, &te), "want *json.UnmarshalTypeError, got %v", err)
			assert.Equal(t, tc.field, te.Field)
		})
	}
}

// T-holders: the new fields reach every holder of a gift.
func TestB65I_Holders(t *testing.T) {
	var nft tg.TonNFT
	require.NoError(t, json.Unmarshal([]byte(`{"address":"a","index":"1","name":"n","description":"d","gift":`+b65iUF+`}`), &nft))
	require.NotNil(t, nft.Gift)
	assert.Equal(t, b65iUFValue(), *nft.Gift)

	var theme tg.ChatShareTheme
	require.NoError(t, json.Unmarshal([]byte(`{"type":"unique_gift","gift":`+b65iUB+`}`), &theme))
	require.NotNil(t, theme.Gift)
	assert.Equal(t, b65iUBValue(), *theme.Gift)

	var owned tg.OwnedGift
	require.NoError(t, json.Unmarshal([]byte(`{"type":"regular","gift":`+b65iGF+`,"send_date":1}`), &owned))
	require.NotNil(t, owned.Gift)
	assert.Equal(t, b65iPubChat(), owned.Gift.PublisherChat)

	var partner tg.TransactionPartnerUser
	require.NoError(t, json.Unmarshal([]byte(`{"type":"user","user":{"id":1,"is_bot":false,"first_name":"u"},"gift":`+b65iGF+`}`), &partner))
	require.NotNil(t, partner.Gift)
	assert.Equal(t, b65iPubChat(), partner.Gift.PublisherChat)
}
