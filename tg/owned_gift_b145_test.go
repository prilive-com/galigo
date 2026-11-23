package tg_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-145 (TEST-PLAN section 2): the OwnedGift codec routes "gift" by Type into Gift or UniqueGift.

const (
	b145Sticker = `{"file_id":"f-1","file_unique_id":"fu-1","type":"regular","width":512,"height":512,"is_animated":false,"is_video":false}`
	b145RegGift = `{"id":"g-1","sticker":` + b145Sticker + `,"star_count":5}`
	b145UniGift = `{"gift_id":"u-1","base_name":"Pepe","name":"Pepe-9","number":9,"model":{"name":"m","sticker":` + b145Sticker +
		`,"rarity_per_mille":1},"symbol":{"name":"s","sticker":` + b145Sticker + `,"rarity_per_mille":2},"backdrop":{"name":"b","colors":` +
		`{"center_color":1,"edge_color":2,"symbol_color":3,"text_color":4},"rarity_per_mille":3}}`
	b145RegJSON = `{"type":"regular","gift":` + b145RegGift + `,"owned_gift_id":"og-1","sender_user":{"id":42,"is_bot":false,` +
		`"first_name":"A"},"send_date":1700000000,"text":"hi you","entities":[{"type":"bold","offset":0,"length":2}],` +
		`"is_private":true,"is_saved":true,"can_be_upgraded":true,"was_refunded":true,"convert_star_count":3,` +
		`"prepaid_upgrade_star_count":4,"is_upgrade_separate":true,"unique_gift_number":77}`
	b145UniJSON = `{"type":"unique","gift":` + b145UniGift + `,"owned_gift_id":"og-2","sender_user":{"id":43,"is_bot":false,` +
		`"first_name":"B"},"send_date":1700000001,"is_saved":true,"can_be_transferred":true,"transfer_star_count":25,` +
		`"next_transfer_date":4102444800}`
)

func b145Sticker0() tg.Sticker {
	return tg.Sticker{FileID: "f-1", FileUniqueID: "fu-1", Type: "regular", Width: 512, Height: 512}
}

func b145WantGift() *tg.Gift { return &tg.Gift{ID: "g-1", Sticker: b145Sticker0(), StarCount: 5} }

func b145WantUnique() *tg.UniqueGift {
	var u tg.UniqueGift
	if err := json.Unmarshal([]byte(b145UniGift), &u); err != nil {
		panic(err)
	}
	return &u
}

func b145WantReg() tg.OwnedGift {
	return tg.OwnedGift{
		Type: "regular", Gift: b145WantGift(), OwnedGiftID: "og-1",
		SenderUser: &tg.User{ID: 42, FirstName: "A"}, SendDate: 1700000000, Text: "hi you",
		TextEntities: []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 2}},
		IsPrivate:    true, IsSaved: true, CanBeUpgraded: true, WasRefunded: true, ConvertStarCount: 3,
		PrepaidUpgradeStarCount: 4, IsUpgradeSeparate: true, UniqueGiftNumber: 77,
	}
}

func b145WantUni() tg.OwnedGift {
	return tg.OwnedGift{
		Type: "unique", UniqueGift: b145WantUnique(), OwnedGiftID: "og-2",
		SenderUser: &tg.User{ID: 43, FirstName: "B"}, SendDate: 1700000001, IsSaved: true,
		CanBeTransferred: true, TransferStarCount: 25, NextTransferDate: 4102444800,
	}
}

func b145Decode(t *testing.T, data string, into *tg.OwnedGift) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(data), into), "decode %s", data)
}

// T1, T2: every field of both variants, compared as whole values.
func TestB145_DecodeVariants(t *testing.T) {
	var reg, uni tg.OwnedGift
	b145Decode(t, b145RegJSON, &reg)
	assert.Equal(t, b145WantReg(), reg)
	b145Decode(t, b145UniJSON, &uni)
	assert.Equal(t, b145WantUni(), uni)
	if assert.NotNil(t, uni.UniqueGift) {
		assert.Equal(t, 9, uni.UniqueGift.Number, "the fixture's unique gift is not empty")
	}
}

// T3: "gift" before "type" routes the same.
func TestB145_DecodeKeyOrder(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       tg.OwnedGift
	}{
		{"regular", `{"gift":` + b145RegGift + `,` + strings.TrimPrefix(strings.Replace(b145RegJSON, `"gift":`+b145RegGift+`,`, "", 1), "{"), b145WantReg()},
		{"unique", `{"gift":` + b145UniGift + `,` + strings.TrimPrefix(strings.Replace(b145UniJSON, `"gift":`+b145UniGift+`,`, "", 1), "{"), b145WantUni()},
	} {
		t.Run(c.name, func(t *testing.T) {
			require.True(t, strings.HasPrefix(c.data, `{"gift":`))
			var g tg.OwnedGift
			b145Decode(t, c.data, &g)
			assert.Equal(t, c.want, g)
		})
	}
}

// T4, T5, T6: unknown or missing type, and a null or absent gift: no pointer, no error, the other fields kept.
func TestB145_DecodeNoGift(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       tg.OwnedGift
	}{
		{"T4 unknown object", `{"type":"future","gift":` + b145RegGift + `,"owned_gift_id":"o","send_date":5}`, tg.OwnedGift{Type: "future", OwnedGiftID: "o", SendDate: 5}},
		{"T4 unknown number", `{"type":"future","gift":5,"owned_gift_id":"o"}`, tg.OwnedGift{Type: "future", OwnedGiftID: "o"}},
		{"T5 no type", `{"gift":` + b145RegGift + `,"owned_gift_id":"o"}`, tg.OwnedGift{OwnedGiftID: "o"}},
		{"T6 regular null", `{"type":"regular","gift":null,"send_date":1}`, tg.OwnedGift{Type: "regular", SendDate: 1}},
		{"T6 regular absent", `{"type":"regular","send_date":1}`, tg.OwnedGift{Type: "regular", SendDate: 1}},
		{"T6 unique null", `{"type":"unique","gift":null,"send_date":1}`, tg.OwnedGift{Type: "unique", SendDate: 1}},
		{"T6 unique absent", `{"type":"unique","send_date":1}`, tg.OwnedGift{Type: "unique", SendDate: 1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var g tg.OwnedGift
			b145Decode(t, c.data, &g)
			assert.Equal(t, c.want, g)
		})
	}
}

// T7: a wrong JSON type inside the gift of a known variant, or in a plain field, is a decode error.
func TestB145_DecodeWrongType(t *testing.T) {
	for _, data := range []string{
		`{"type":"regular","gift":{"star_count":"x"}}`,
		`{"type":"regular","gift":5}`,
		`{"type":"unique","gift":{"number":"x"}}`,
		`{"type":"regular","send_date":"x"}`,
	} {
		t.Run(data, func(t *testing.T) {
			var g tg.OwnedGift
			err := json.Unmarshal([]byte(data), &g)
			var te *json.UnmarshalTypeError
			assert.True(t, errors.As(err, &te), "want *json.UnmarshalTypeError, got %v", err)
		})
	}
}

// T8: JSON null changes nothing.
func TestB145_DecodeNull(t *testing.T) {
	for _, want := range []tg.OwnedGift{b145WantReg(), b145WantUni()} {
		t.Run(want.Type, func(t *testing.T) {
			g := want
			require.NoError(t, json.Unmarshal([]byte(`null`), &g))
			assert.Equal(t, want, g)
		})
	}
}

// T9: decoding into a used value clears both pointers, merges the other keys and routes by the Type after the decode.
func TestB145_DecodeReuse(t *testing.T) {
	t.Run("a regular to unique", func(t *testing.T) {
		g := b145WantReg()
		b145Decode(t, b145UniJSON, &g)
		want := b145WantUni()
		want.Text, want.TextEntities, want.IsPrivate, want.CanBeUpgraded, want.WasRefunded = "hi you", b145WantReg().TextEntities, true, true, true
		want.ConvertStarCount, want.PrepaidUpgradeStarCount, want.IsUpgradeSeparate, want.UniqueGiftNumber = 3, 4, true, 77
		assert.Equal(t, want, g)
	})
	t.Run("b unique to regular", func(t *testing.T) {
		g := b145WantUni()
		b145Decode(t, b145RegJSON, &g)
		want := b145WantReg()
		want.CanBeTransferred, want.TransferStarCount, want.NextTransferDate = true, 25, 4102444800
		assert.Equal(t, want, g)
	})
	t.Run("c no gift clears", func(t *testing.T) {
		g := b145WantReg()
		b145Decode(t, `{"type":"regular","send_date":1}`, &g)
		assert.Nil(t, g.Gift)
		assert.Nil(t, g.UniqueGift)
	})
	t.Run("d no type uses the old Type", func(t *testing.T) {
		g := tg.OwnedGift{Type: "regular"}
		b145Decode(t, `{"gift":{"id":"g-2","sticker":`+b145Sticker+`,"star_count":1}}`, &g)
		assert.Equal(t, &tg.Gift{ID: "g-2", Sticker: b145Sticker0(), StarCount: 1}, g.Gift)
		assert.Nil(t, g.UniqueGift)
	})
	t.Run("e reused slice", func(t *testing.T) {
		list := []tg.OwnedGift{b145WantReg(), b145WantReg()}
		require.NoError(t, json.Unmarshal([]byte(`[`+b145UniJSON+`,`+b145RegJSON+`]`), &list))
		assert.Nil(t, list[0].Gift)
		assert.Equal(t, b145WantUnique(), list[0].UniqueGift)
		assert.Equal(t, b145WantGift(), list[1].Gift)
	})
	for _, start := range []tg.OwnedGift{b145WantReg(), b145WantUni()} {
		t.Run("f unknown clears "+start.Type, func(t *testing.T) {
			g := start
			b145Decode(t, `{"type":"future"}`, &g)
			assert.Nil(t, g.Gift)
			assert.Nil(t, g.UniqueGift)
		})
	}
	t.Run("g same type new pointer", func(t *testing.T) {
		g := b145WantReg()
		old := g.Gift
		b145Decode(t, `{"type":"regular","gift":{"id":"g-3"}}`, &g)
		assert.Equal(t, &tg.Gift{ID: "g-3"}, g.Gift)
		assert.Equal(t, b145WantGift(), old, "the old pointer is not written")
	})
	t.Run("h same type new unique pointer", func(t *testing.T) {
		g := b145WantUni()
		old := g.UniqueGift
		b145Decode(t, `{"type":"unique","gift":{"gift_id":"u-3"}}`, &g)
		assert.Equal(t, &tg.UniqueGift{GiftID: "u-3"}, g.UniqueGift)
		assert.Equal(t, b145WantUnique(), old, "the old pointer is not written")
	})
	t.Run("i repeated gift key", func(t *testing.T) {
		var g tg.OwnedGift
		b145Decode(t, `{"type":"regular","gift":{"id":"g-4","star_count":5},"gift":{"id":"g-5"}}`, &g)
		assert.Equal(t, &tg.Gift{ID: "g-5"}, g.Gift)
	})
	for _, start := range []tg.OwnedGift{b145WantReg(), b145WantUni()} {
		t.Run("j failed decode still clears "+start.Type, func(t *testing.T) {
			g := start
			require.Error(t, json.Unmarshal([]byte(`{"type":"regular","send_date":"bad"}`), &g))
			assert.Nil(t, g.Gift)
			assert.Nil(t, g.UniqueGift)
		})
	}
}

func b145Marshal(t *testing.T, g tg.OwnedGift) string {
	t.Helper()
	v, err := json.Marshal(g)
	require.NoError(t, err)
	p, err := json.Marshal(&g)
	require.NoError(t, err)
	require.Equal(t, string(v), string(p), "value and pointer give the same bytes")
	return string(v)
}

// T10: exact encode bytes; "gift" is the second key, written from the pointer the Type names.
func TestB145_Encode(t *testing.T) {
	gb, err := json.Marshal(b145WantGift())
	require.NoError(t, err)
	ub, err := json.Marshal(b145WantUnique())
	require.NoError(t, err)
	both := tg.OwnedGift{Gift: b145WantGift(), UniqueGift: b145WantUnique()}
	withType := func(g tg.OwnedGift, typ string) tg.OwnedGift { g.Type = typ; return g }
	for _, c := range []struct {
		name string
		in   tg.OwnedGift
		want string
	}{
		{"a regular", b145WantReg(), `{"type":"regular","gift":` + string(gb) + `,"owned_gift_id":"og-1","sender_user":{"id":42,"is_bot":false,` +
			`"first_name":"A"},"send_date":1700000000,"text":"hi you","entities":[{"type":"bold","offset":0,"length":2}],"is_private":true,` +
			`"is_saved":true,"can_be_upgraded":true,"was_refunded":true,"convert_star_count":3,"prepaid_upgrade_star_count":4,` +
			`"is_upgrade_separate":true,"unique_gift_number":77}`},
		{"b unique", b145WantUni(), `{"type":"unique","gift":` + string(ub) + `,"owned_gift_id":"og-2","sender_user":{"id":43,"is_bot":false,` +
			`"first_name":"B"},"send_date":1700000001,"is_saved":true,"can_be_transferred":true,"transfer_star_count":25,` +
			`"next_transfer_date":4102444800}`},
		// The bytes of the same value at 0cd66c8 (before B-145).
		{"c no gift, as before", tg.OwnedGift{Type: "regular", OwnedGiftID: "o"}, `{"type":"regular","owned_gift_id":"o","send_date":0}`},
		{"d unique with only Gift", tg.OwnedGift{Type: "unique", Gift: b145WantGift()}, `{"type":"unique","send_date":0}`},
		{"e regular with only UniqueGift", tg.OwnedGift{Type: "regular", UniqueGift: b145WantUnique()}, `{"type":"regular","send_date":0}`},
		{"f zero", tg.OwnedGift{}, `{"type":"","send_date":0}`},
		{"g both, unique", withType(both, "unique"), `{"type":"unique","gift":` + string(ub) + `,"send_date":0}`},
		{"g both, regular", withType(both, "regular"), `{"type":"regular","gift":` + string(gb) + `,"send_date":0}`},
		{"h future with Gift", tg.OwnedGift{Type: "future", Gift: b145WantGift()}, `{"type":"future","gift":` + string(gb) + `,"send_date":0}`},
		{"h empty type with Gift", tg.OwnedGift{Gift: b145WantGift()}, `{"type":"","gift":` + string(gb) + `,"send_date":0}`},
	} {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, b145Marshal(t, c.in)) })
	}
}

// T11: decode, encode, decode gives the first value.
func TestB145_RoundTrip(t *testing.T) {
	for _, data := range []string{b145RegJSON, b145UniJSON} {
		var a, b tg.OwnedGift
		b145Decode(t, data, &a)
		b145Decode(t, b145Marshal(t, a), &b)
		assert.Equal(t, a, b)
	}
}

// T12: the spec key is "entities"; "text_entities" is not read and not written.
func TestB145_Entities(t *testing.T) {
	var g, old tg.OwnedGift
	b145Decode(t, `{"type":"regular","entities":[{"type":"bold","offset":0,"length":1}]}`, &g)
	assert.Equal(t, []tg.MessageEntity{{Type: "bold", Length: 1}}, g.TextEntities)
	b145Decode(t, `{"type":"regular","text_entities":[{"type":"bold","offset":0,"length":1}]}`, &old)
	assert.Nil(t, old.TextEntities)
	out := b145Marshal(t, g)
	assert.Contains(t, out, `"entities":[`)
	assert.NotContains(t, out, `text_entities`)
}

// T13: a list holds both variants and an unknown one; a bad element fails the whole list.
func TestB145_OwnedGiftsList(t *testing.T) {
	var l tg.OwnedGifts
	require.NoError(t, json.Unmarshal([]byte(`{"total_count":3,"gifts":[`+b145RegJSON+`,`+b145UniJSON+`,{"type":"future"}],"next_offset":"n-1"}`), &l))
	assert.Equal(t, tg.OwnedGifts{TotalCount: 3, Gifts: []tg.OwnedGift{b145WantReg(), b145WantUni(), {Type: "future"}}, NextOffset: "n-1"}, l)
	var bad tg.OwnedGifts
	err := json.Unmarshal([]byte(`{"total_count":1,"gifts":[{"type":"regular","gift":{"star_count":"x"}}]}`), &bad)
	var te *json.UnmarshalTypeError
	assert.True(t, errors.As(err, &te), "got %v", err)
}

type b145Extra struct {
	tg.OwnedGift
	Extra string
}

type b145NilEmbed struct{ *tg.OwnedGift }

type b145BothCodec struct{ Note string }

func (b145BothCodec) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func (*b145BothCodec) UnmarshalJSON([]byte) error  { return nil }

type b145MarshalOnly struct{ Note string }

func (b145MarshalOnly) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }

type b145Cancel struct {
	tg.OwnedGift
	b145BothCodec
}

type b145HalfCancel struct {
	tg.OwnedGift
	b145MarshalOnly
}

// T14: the embedding behaviour documented in the upgrade note (PLAN section 9a, rows 1-4).
func TestB145_Embedding(t *testing.T) {
	t.Run("a own fields lost", func(t *testing.T) {
		out, err := json.Marshal(b145Extra{OwnedGift: tg.OwnedGift{Type: "regular"}, Extra: "x"})
		require.NoError(t, err)
		assert.Equal(t, `{"type":"regular","send_date":0}`, string(out))
		var in b145Extra
		require.NoError(t, json.Unmarshal([]byte(`{"type":"regular","Extra":"y"}`), &in))
		assert.Equal(t, "", in.Extra)
		assert.Equal(t, "regular", in.Type)
	})
	t.Run("b nil embedded pointer", func(t *testing.T) {
		assert.Panics(t, func() { _, _ = json.Marshal(b145NilEmbed{}) })
		assert.Panics(t, func() { var v b145NilEmbed; _ = json.Unmarshal([]byte(`{}`), &v) })
		assert.NotPanics(t, func() { var v b145NilEmbed; _ = json.Unmarshal([]byte(`null`), &v) })
	})
	t.Run("c both methods cancel", func(t *testing.T) {
		// Neither codec is promoted: plain encoding of both embedded structs; UniqueGift (json:"-") is not written.
		out, err := json.Marshal(b145Cancel{OwnedGift: tg.OwnedGift{Type: "unique", UniqueGift: b145WantUnique()}})
		require.NoError(t, err)
		assert.Equal(t, `{"type":"unique","send_date":0,"Note":""}`, string(out))
	})
	t.Run("d marshal only cancels marshal", func(t *testing.T) {
		// Only MarshalJSON is ambiguous: decoding still uses OwnedGift's codec, so a unique gift is routed.
		var v b145HalfCancel
		require.NoError(t, json.Unmarshal([]byte(b145UniJSON), &v))
		assert.Equal(t, b145WantUnique(), v.UniqueGift)
		assert.Nil(t, v.Gift)
	})
}

// T15: DisallowUnknownFields does not reach inside an OwnedGift (its codec decodes); it still works on OwnedGifts.
func TestB145_DisallowUnknown(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader([]byte(`{"type":"regular","zz":1}`)))
	dec.DisallowUnknownFields()
	var g tg.OwnedGift
	assert.NoError(t, dec.Decode(&g))
	dec = json.NewDecoder(bytes.NewReader([]byte(`{"total_count":0,"gifts":[],"zz":1}`)))
	dec.DisallowUnknownFields()
	var l tg.OwnedGifts
	assert.Error(t, dec.Decode(&l))
}

// T17: each key of both variants fills exactly its own field (one leaf: a loop, no subtests).
func TestB145_KeyTable(t *testing.T) {
	user := &tg.User{ID: 9, FirstName: "C"}
	for _, c := range []struct {
		key, value string
		want       tg.OwnedGift
	}{
		{"owned_gift_id", `"o"`, tg.OwnedGift{OwnedGiftID: "o"}},
		{"sender_user", `{"id":9,"is_bot":false,"first_name":"C"}`, tg.OwnedGift{SenderUser: user}},
		{"send_date", `7`, tg.OwnedGift{SendDate: 7}},
		{"text", `"t"`, tg.OwnedGift{Text: "t"}},
		{"entities", `[{"type":"bold","offset":0,"length":1}]`, tg.OwnedGift{TextEntities: []tg.MessageEntity{{Type: "bold", Length: 1}}}},
		{"is_private", `true`, tg.OwnedGift{IsPrivate: true}},
		{"is_saved", `true`, tg.OwnedGift{IsSaved: true}},
		{"can_be_upgraded", `true`, tg.OwnedGift{CanBeUpgraded: true}},
		{"was_refunded", `true`, tg.OwnedGift{WasRefunded: true}},
		{"convert_star_count", `3`, tg.OwnedGift{ConvertStarCount: 3}},
		{"prepaid_upgrade_star_count", `4`, tg.OwnedGift{PrepaidUpgradeStarCount: 4}},
		{"is_upgrade_separate", `true`, tg.OwnedGift{IsUpgradeSeparate: true}},
		{"unique_gift_number", `77`, tg.OwnedGift{UniqueGiftNumber: 77}},
		{"can_be_transferred", `true`, tg.OwnedGift{CanBeTransferred: true}},
		{"transfer_star_count", `25`, tg.OwnedGift{TransferStarCount: 25}},
		{"next_transfer_date", `4102444800`, tg.OwnedGift{NextTransferDate: 4102444800}},
	} {
		var g tg.OwnedGift
		b145Decode(t, `{"type":"regular","`+c.key+`":`+c.value+`}`, &g)
		c.want.Type = "regular"
		assert.Equal(t, c.want, g, c.key)
	}
}
