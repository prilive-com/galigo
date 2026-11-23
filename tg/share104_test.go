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

// B-65 65h: Bot API 10.4 TON wallet transfer, the wallet card and share theme in ChatFullInfo, the share theme
// backdrops (TEST-PLAN section 2). Ids and addresses are fake.

const (
	b65hT1 = `{"transaction_id":"tx-fake-1","peer_address":"EQfakePeer1","amount":"-1500000000","comment":"hi"}`
	b65hE1 = `{"type":"emoji","emojis":[{"type":"emoji","emoji":"🙂"},{"type":"custom_emoji","custom_emoji_id":"5368fake"}],"background_backdrop_id":0}`
	b65hG1 = `{"type":"unique_gift","gift":{"base_name":"bn","name":"gn","number":11,` +
		`"model":{"name":"","sticker":{"file_id":"","file_unique_id":"","type":"","width":0,"height":0,"is_animated":false,"is_video":false},"rarity_per_mille":0},` +
		`"symbol":{"name":"","sticker":{"file_id":"","file_unique_id":"","type":"","width":0,"height":0,"is_animated":false,"is_video":false},"rarity_per_mille":0},` +
		`"backdrop":{"name":"","colors":{"center_color":0,"edge_color":0,"symbol_color":0,"text_color":0},"rarity_per_mille":0}}}`
	b65hC1 = `{"address":"EQfakeCard1","amount":"1000","stickers":[]}`
	b65hB1 = `{"backdrops":[{"id":0,"name":"Black","colors":{"center_color":1,"edge_color":2,"symbol_color":0,"text_color":16772551}},` +
		`{"id":79,"name":"","colors":{"center_color":16777215,"edge_color":3,"symbol_color":4,"text_color":5}}]}`
	b65hMsgHead = `{"message_id":1,"date":1,"chat":{"id":77,"type":"private"}`
	b65hCFIHead = `{"id":77,"type":"private","accent_color_id":0,"max_reaction_count":0`
)

func b65hPtr(v int) *int { return &v }

func b65hT1Value() *tg.TonWalletTransfer {
	return &tg.TonWalletTransfer{TransactionID: "tx-fake-1", PeerAddress: "EQfakePeer1", Amount: "-1500000000", Comment: "hi"}
}

func b65hE1Value() *tg.ChatShareTheme {
	return &tg.ChatShareTheme{Type: "emoji", Emojis: []tg.EmojiType{{Type: "emoji", Emoji: "🙂"},
		{Type: "custom_emoji", CustomEmojiID: "5368fake"}}, BackgroundBackdropID: b65hPtr(0)}
}

func b65hG1Value() *tg.ChatShareTheme {
	return &tg.ChatShareTheme{Type: "unique_gift", Gift: &tg.UniqueGift{BaseName: "bn", Name: "gn", Number: 11}}
}

func b65hC1Value() *tg.TonWalletCard {
	return &tg.TonWalletCard{Address: "EQfakeCard1", Amount: "1000", Stickers: []tg.TonWalletCardSticker{}}
}

func b65hB1Value() tg.ChatShareThemeBackdrops {
	return tg.ChatShareThemeBackdrops{Backdrops: []tg.ChatShareThemeBackdrop{
		{ID: 0, Name: "Black", Colors: tg.UniqueGiftBackdropColors{CenterColor: 1, EdgeColor: 2, SymbolColor: 0, TextColor: 16772551}},
		{ID: 79, Name: "", Colors: tg.UniqueGiftBackdropColors{CenterColor: 16777215, EdgeColor: 3, SymbolColor: 4, TextColor: 5}},
	}}
}

// T-transfer-decode: Message.ton_wallet_transfer, with an empty transaction id and no comment, absent (in the full leaf),
// and null over a seeded value.
func TestB65H_TransferDecode(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       *tg.TonWalletTransfer
	}{
		{"full", b65hMsgHead + `,"ton_wallet_transfer":` + b65hT1 + `}`, b65hT1Value()},
		{"not_sent", b65hMsgHead + `,"ton_wallet_transfer":{"transaction_id":"","peer_address":"EQfakePeer1","amount":"7"}}`,
			&tg.TonWalletTransfer{PeerAddress: "EQfakePeer1", Amount: "7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m tg.Message
			require.NoError(t, json.Unmarshal([]byte(tc.body), &m))
			assert.Equal(t, tc.want, m.TonWalletTransfer)
			if tc.name == "full" { // the key absent (A-T2-02: inside a leaf that has a killer)
				var a tg.Message
				require.NoError(t, json.Unmarshal([]byte(b65hMsgHead+`}`), &a))
				assert.Nil(t, a.TonWalletTransfer, "absent")
			}
		})
	}
	t.Run("null_clears", func(t *testing.T) {
		m := tg.Message{TonWalletTransfer: &tg.TonWalletTransfer{Amount: "9"}}
		require.NoError(t, json.Unmarshal([]byte(b65hMsgHead+`,"ton_wallet_transfer":null}`), &m))
		assert.Nil(t, m.TonWalletTransfer)
	})
}

// T-transfer-encode: exact bytes; transaction_id, peer_address and amount always encode; comment only when set.
func TestB65H_TransferEncode(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"zero", tg.TonWalletTransfer{}, `{"transaction_id":"","peer_address":"","amount":""}`},
		{"no_comment", tg.TonWalletTransfer{PeerAddress: "EQfakePeer1", Amount: "1"}, `{"transaction_id":"","peer_address":"EQfakePeer1","amount":"1"}`},
		{"full", b65hT1Value(), b65hT1},
		{"message", tg.Message{MessageID: 1, Date: 1, TonWalletTransfer: b65hT1Value()}, `{"message_id":1,"date":1,"chat":null,"ton_wallet_transfer":` + b65hT1 + `}`},
		{"message_nil", tg.Message{MessageID: 1, Date: 1}, `{"message_id":1,"date":1,"chat":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.v)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(b))
		})
	}
}

// T-cfi-decode: ChatFullInfo.ton_wallet_card and share_theme (both variants), and both absent (in the emoji leaf).
func TestB65H_ChatFullInfoDecode(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		card       *tg.TonWalletCard
		theme      *tg.ChatShareTheme
	}{
		{"emoji", b65hCFIHead + `,"ton_wallet_card":` + b65hC1 + `,"share_theme":` + b65hE1 + `}`, b65hC1Value(), b65hE1Value()},
		{"unique_gift", b65hCFIHead + `,"share_theme":` + b65hG1 + `}`, nil, b65hG1Value()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c tg.ChatFullInfo
			require.NoError(t, json.Unmarshal([]byte(tc.body), &c))
			assert.Equal(t, tc.card, c.TonWalletCard)
			assert.Equal(t, tc.theme, c.ShareTheme)
			if tc.name == "emoji" && assert.NotNil(t, c.ShareTheme) && assert.NotNil(t, c.ShareTheme.BackgroundBackdropID) {
				assert.Equal(t, 0, *c.ShareTheme.BackgroundBackdropID, "backdrop 0 is a real backdrop")
			}
			if tc.name == "emoji" { // both keys absent (A-T2-02: inside a leaf that has a killer)
				var a tg.ChatFullInfo
				require.NoError(t, json.Unmarshal([]byte(b65hCFIHead+`}`), &a))
				assert.Nil(t, a.TonWalletCard, "absent")
				assert.Nil(t, a.ShareTheme, "absent")
			}
		})
	}
}

// T-theme-encode and T-emoji: exact bytes for each variant; [] vs nil emojis; the backdrop pointer to 0.
func TestB65H_ThemeEncode(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"emoji_empty_list_backdrop0", tg.ChatShareTheme{Type: "emoji", Emojis: []tg.EmojiType{}, BackgroundBackdropID: b65hPtr(0)},
			`{"type":"emoji","emojis":[],"background_backdrop_id":0}`},
		{"emoji_nil_list", tg.ChatShareTheme{Type: "emoji"}, `{"type":"emoji"}`},
		{"E1", b65hE1Value(), b65hE1},
		{"G1", b65hG1Value(), b65hG1},
		{"emoji_type_emoji", tg.EmojiType{Type: "emoji", Emoji: "🙂"}, `{"type":"emoji","emoji":"🙂"}`},
		{"emoji_type_custom", tg.EmojiType{Type: "custom_emoji", CustomEmojiID: "5368fake"}, `{"type":"custom_emoji","custom_emoji_id":"5368fake"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.v)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(b))
		})
	}
}

// T-emoji (decode): both EmojiType variants decode to the exact struct, and an unknown type decodes with its Type and no
// error (the upgrade note; mutation discovery d1: no test held this).
func TestB65H_EmojiDecode(t *testing.T) {
	for _, tc := range []struct {
		body string
		want tg.EmojiType
	}{
		{`{"type":"emoji","emoji":"🙂"}`, tg.EmojiType{Type: "emoji", Emoji: "🙂"}},
		{`{"type":"custom_emoji","custom_emoji_id":"5368fake"}`, tg.EmojiType{Type: "custom_emoji", CustomEmojiID: "5368fake"}},
		{`{"type":"z_future","k":1}`, tg.EmojiType{Type: "z_future"}},
	} {
		var got tg.EmojiType
		require.NoError(t, json.Unmarshal([]byte(tc.body), &got))
		assert.Equal(t, tc.want, got, tc.body)
	}
}

// T-theme-elem: [] stays a non-nil empty list; null clears a seeded list; absent stays nil; an unknown variant decodes.
func TestB65H_ThemeElements(t *testing.T) {
	t.Run("empty_list", func(t *testing.T) {
		th := tg.ChatShareTheme{Emojis: []tg.EmojiType{{Type: "seed"}}}
		require.NoError(t, json.Unmarshal([]byte(`{"type":"emoji","emojis":[]}`), &th))
		require.NotNil(t, th.Emojis)
		assert.Empty(t, th.Emojis)
	})
	t.Run("null_clears", func(t *testing.T) {
		th := tg.ChatShareTheme{Emojis: []tg.EmojiType{{Type: "seed"}}}
		require.NoError(t, json.Unmarshal([]byte(`{"type":"emoji","emojis":null}`), &th))
		assert.Nil(t, th.Emojis)
	})
	t.Run("absent", func(t *testing.T) {
		var th tg.ChatShareTheme
		require.NoError(t, json.Unmarshal([]byte(`{"type":"emoji"}`), &th))
		assert.Nil(t, th.Emojis)
		assert.Equal(t, "emoji", th.Type)
	})
	t.Run("unknown_variant", func(t *testing.T) {
		var th tg.ChatShareTheme
		require.NoError(t, json.Unmarshal([]byte(`{"type":"z_future","k":1}`), &th))
		assert.Equal(t, tg.ChatShareTheme{Type: "z_future"}, th)
	})
}

// T-bad: a value of the wrong JSON type under a new key fails the decode, and the error names the key (PLAN §4).
func TestB65H_BadValues(t *testing.T) {
	for _, tc := range []struct {
		name, body, field string
		v                 any
	}{
		{"emojis", `{"type":"emoji","emojis":{}}`, "emojis", &tg.ChatShareTheme{}},
		{"share_theme", b65hCFIHead + `,"share_theme":[]}`, "share_theme", &tg.ChatFullInfo{}},
		{"ton_wallet_card", b65hCFIHead + `,"ton_wallet_card":"x"}`, "ton_wallet_card", &tg.ChatFullInfo{}},
		{"amount", b65hMsgHead + `,"ton_wallet_transfer":{"transaction_id":"","peer_address":"p","amount":5}}`, "ton_wallet_transfer.amount", &tg.Message{}},
		{"nested_amount", b65hMsgHead + `,"reply_to_message":` + b65hMsgHead + `,"ton_wallet_transfer":{"transaction_id":"","peer_address":"p","amount":5}}}`,
			"reply_to_message.ton_wallet_transfer.amount", &tg.Message{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tc.body), tc.v)
			require.Error(t, err)
			var ute *json.UnmarshalTypeError
			require.True(t, errors.As(err, &ute), "%T: %v", err, err)
			assert.Equal(t, tc.field, ute.Field)
		})
	}
}

// T-backdrops: the backdrop list decodes exactly and encodes back to the same bytes (id 0 and an empty name kept).
func TestB65H_Backdrops(t *testing.T) {
	var got tg.ChatShareThemeBackdrops
	require.NoError(t, json.Unmarshal([]byte(b65hB1), &got))
	assert.Equal(t, b65hB1Value(), got)
	b, err := json.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, b65hB1, string(b))
}

type b65hField struct {
	Name, Type, Tag string
	Anonymous       bool
}

func b65hFields(rt reflect.Type) []b65hField {
	var out []b65hField
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, b65hField{f.Name, f.Type.String(), f.Tag.Get("json"), f.Anonymous})
	}
	return out
}

func b65hHolder(t *testing.T, v any, name string) b65hField {
	t.Helper()
	f, ok := reflect.TypeOf(v).FieldByName(name)
	require.True(t, ok, "%T.%s", v, name)
	return b65hField{f.Name, f.Type.String(), f.Tag.Get("json"), f.Anonymous}
}

// T-shapes: every field of the new types, the three holders, their places, and no methods (E-0126).
func TestB65H_Shapes(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want []b65hField
	}{
		{tg.TonWalletTransfer{}, []b65hField{{"TransactionID", "string", "transaction_id", false}, {"PeerAddress", "string", "peer_address", false},
			{"Amount", "string", "amount", false}, {"Comment", "string", "comment,omitempty", false}}},
		{tg.EmojiType{}, []b65hField{{"Type", "string", "type", false}, {"Emoji", "string", "emoji,omitempty", false},
			{"CustomEmojiID", "string", "custom_emoji_id,omitempty", false}}},
		{tg.ChatShareTheme{}, []b65hField{{"Type", "string", "type", false}, {"Emojis", "[]tg.EmojiType", "emojis,omitzero", false},
			{"BackgroundBackdropID", "*int", "background_backdrop_id,omitempty", false}, {"Gift", "*tg.UniqueGift", "gift,omitempty", false}}},
		{tg.ChatShareThemeBackdrop{}, []b65hField{{"ID", "int", "id", false}, {"Name", "string", "name", false},
			{"Colors", "tg.UniqueGiftBackdropColors", "colors", false}}},
		{tg.ChatShareThemeBackdrops{}, []b65hField{{"Backdrops", "[]tg.ChatShareThemeBackdrop", "backdrops", false}}},
	} {
		rt := reflect.TypeOf(tc.v)
		t.Run(rt.Name(), func(t *testing.T) {
			assert.Equal(t, tc.want, b65hFields(rt))
			assert.Equal(t, 0, rt.NumMethod(), "no value methods")
			assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod(), "no pointer methods")
		})
	}
	t.Run("holders", func(t *testing.T) {
		assert.Equal(t, b65hField{"TonWalletTransfer", "*tg.TonWalletTransfer", "ton_wallet_transfer,omitempty", false}, b65hHolder(t, tg.Message{}, "TonWalletTransfer"))
		assert.Equal(t, b65hField{"TonWalletCard", "*tg.TonWalletCard", "ton_wallet_card,omitempty", false}, b65hHolder(t, tg.ChatFullInfo{}, "TonWalletCard"))
		assert.Equal(t, b65hField{"ShareTheme", "*tg.ChatShareTheme", "share_theme,omitempty", false}, b65hHolder(t, tg.ChatFullInfo{}, "ShareTheme"))
	})
	t.Run("order", func(t *testing.T) {
		last := func(rt reflect.Type, n int) []string {
			var out []string
			for i := rt.NumField() - n; i < rt.NumField(); i++ {
				out = append(out, rt.Field(i).Name)
			}
			return out
		}
		assert.Equal(t, []string{"Community", "TonWalletCard", "ShareTheme"}, last(reflect.TypeOf(tg.ChatFullInfo{}), 3))
		assert.Equal(t, []string{"SuggestedPostRefunded", "TonWalletTransfer"}, last(reflect.TypeOf(tg.Message{}), 2))
	})
}

// T-old-zero: the zero values encode to the bytes they had at fe75308 (trials/old-zero-base.txt).
func TestB65H_OldZero(t *testing.T) {
	m, err := json.Marshal(tg.Message{})
	require.NoError(t, err)
	assert.Equal(t, `{"message_id":0,"date":0,"chat":null}`, string(m))
	c, err := json.Marshal(tg.ChatFullInfo{})
	require.NoError(t, err)
	assert.Equal(t, `{"id":0,"type":""}`, string(c))
}
