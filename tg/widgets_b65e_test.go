package tg_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.4 widgets, read side (B-65 65e, TEST-PLAN §2): Message.Widget, ExternalReplyInfo.Widget,
// Update.WidgetQuery, Update.SharedWidgetQuery and their types. Ids and addresses are fake.

// The default encoding/json encoder writes <, > and & in a string as a backslash escape. The backslash is built from
// its code point, so no escape is typed in this file (E-0146); TestEscapeBytes65e checks the bytes.
const bs = "\x5c"

const (
	escLT  = bs + "u003c"
	escGT  = bs + "u003e"
	escAmp = bs + "u0026"
)

func TestEscapeBytes65e(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  string
		want []byte
	}{
		{"lt", escLT, []byte{0x5c, 'u', '0', '0', '3', 'c'}},
		{"gt", escGT, []byte{0x5c, 'u', '0', '0', '3', 'e'}},
		{"amp", escAmp, []byte{0x5c, 'u', '0', '0', '2', '6'}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, []byte(tc.got))
		})
	}
}

const (
	b65eUser  = `{"id":456,"is_bot":false,"first_name":"u"}`
	b65eGroup = `{"id":-1009876543210,"type":"supergroup"}`
)

func b65eUserV() *tg.User { return &tg.User{ID: 456, FirstName: "u"} }
func b65eGroupV() *tg.Chat {
	return &tg.Chat{ID: -1009876543210, Type: "supergroup"}
}

// b65eHolderKey re-encodes the whole holder and checks that its key holds exactly the child's bytes (O-T1-06).
func b65eHolderKey(t *testing.T, holder any, key string, child any) {
	t.Helper()
	hb, err := json.Marshal(holder)
	require.NoError(t, err)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(hb, &keys))
	cb, err := json.Marshal(child)
	require.NoError(t, err)
	assert.Equal(t, string(cb), string(keys[key]), "holder key %q", key)
}

func b65eEncode(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// T-widget-msg
func TestB65E_WidgetMessage(t *testing.T) {
	in := `{"message_id":1,"date":1,"chat":` + b65eGroup + `,"caption":"cap",` +
		`"widget":{"id":"w1","width":320,"height":200,"html":"<b>x</b>","public_data":"p","chat_data":"c"}}`
	var m tg.Message
	require.NoError(t, json.Unmarshal([]byte(in), &m))
	want := tg.Message{MessageID: 1, Date: 1, Chat: b65eGroupV(), Caption: "cap",
		Widget: &tg.Widget{ID: "w1", Width: 320, Height: 200, HTML: "<b>x</b>", PublicData: "p", ChatData: "c"}}
	assert.Equal(t, want, m)
	assert.Equal(t, `{"id":"w1","width":320,"height":200,"html":"`+escLT+"b"+escGT+"x"+escLT+"/b"+escGT+
		`","public_data":"p","chat_data":"c"}`, b65eEncode(t, m.Widget))
	b65eHolderKey(t, m, "widget", m.Widget)
}

// T-widget-min
func TestB65E_WidgetMin(t *testing.T) {
	t.Run("html_only", func(t *testing.T) {
		assert.Equal(t, `{"html":"`+escLT+"b"+escGT+"x"+escLT+"/b"+escGT+escAmp+`"}`, b65eEncode(t, tg.Widget{HTML: "<b>x</b>&"}))
	})
	t.Run("zero", func(t *testing.T) {
		assert.Equal(t, `{"html":""}`, b65eEncode(t, tg.Widget{}))
	})
}

// T-eri-widget
func TestB65E_ERIWidget(t *testing.T) {
	in := `{"origin":{"type":"user","date":1,"sender_user":` + b65eUser + `},"widget":{"html":"h","width":10}}`
	var e tg.ExternalReplyInfo
	require.NoError(t, json.Unmarshal([]byte(in), &e))
	assert.Equal(t, tg.ExternalReplyInfo{Origin: tg.MessageOrigin{Type: "user", Date: 1, SenderUser: b65eUserV()},
		Widget: &tg.Widget{HTML: "h", Width: 10}}, e)
}

// The full wallet card of T-qtype (1), as the encoder writes it (struct order; no gift_id).
const (
	b65eSticker1 = `{"file_id":"f1","file_unique_id":"u1","type":"custom_emoji","width":100,"height":101,"is_animated":true,"is_video":false}`
	b65eSticker2 = `{"file_id":"f2","file_unique_id":"u2","type":"regular","width":102,"height":103,"is_animated":false,"is_video":true}`
	b65eGiftHead = `{"base_name":"bn","name":"gn","number":11,` +
		`"model":{"name":"mn","sticker":` + b65eSticker1 + `,"rarity_per_mille":12,"rarity":"rare"},` +
		`"symbol":{"name":"sn","sticker":` + b65eSticker2 + `,"rarity_per_mille":13},` +
		`"backdrop":{"name":"bd","colors":{"center_color":14,"edge_color":15,"symbol_color":16,"text_color":17},"rarity_per_mille":18},` +
		`"colors":{"model_custom_emoji_id":"me","symbol_custom_emoji_id":"se","light_theme_main_color":19,` +
		`"light_theme_other_colors":[20,21],"dark_theme_main_color":22,"dark_theme_other_colors":[23]},"is_burned":true`
	b65ePosition = `{"x_percentage":12.5,"y_percentage":34.25,"width_percentage":10.5,"height_percentage":20.75,` +
		`"rotation_angle":45.5,"corner_radius_percentage":3.125}`
)

func b65eCard(gift string) string {
	return `{"address":"EQfake1","amount":"1000","stickers":[{"position":` + b65ePosition + `,` +
		`"nft":{"address":"EQfake2","collection_address":"EQfake3","index":"7","name":"n","description":"d",` +
		`"attributes":[{"trait_type":"a1","value":"v1"},{"trait_type":"a2","value":"v2"}],"gift":` + gift + `}}],` +
		`"background_nft_address":"EQfake2"}`
}

func b65ePositionV() tg.AreaPosition {
	return tg.AreaPosition{XPercentage: 12.5, YPercentage: 34.25, WidthPercentage: 10.5, HeightPercentage: 20.75,
		RotationAngle: 45.5, CornerRadiusPercentage: 3.125}
}

func b65eCardV() *tg.TonWalletCard {
	gift := &tg.UniqueGift{BaseName: "bn", Name: "gn", Number: 11,
		Model: tg.UniqueGiftModel{Name: "mn", Sticker: tg.Sticker{FileID: "f1", FileUniqueID: "u1", Type: "custom_emoji",
			Width: 100, Height: 101, IsAnimated: true}, RarityPerMille: 12, Rarity: "rare"},
		Symbol: tg.UniqueGiftSymbol{Name: "sn", Sticker: tg.Sticker{FileID: "f2", FileUniqueID: "u2", Type: "regular",
			Width: 102, Height: 103, IsVideo: true}, RarityPerMille: 13},
		Backdrop: tg.UniqueGiftBackdrop{Name: "bd", Colors: tg.UniqueGiftBackdropColors{CenterColor: 14, EdgeColor: 15,
			SymbolColor: 16, TextColor: 17}, RarityPerMille: 18},
		Colors: &tg.UniqueGiftColors{ModelCustomEmojiID: "me", SymbolCustomEmojiID: "se", LightThemeMainColor: 19,
			LightThemeOtherColors: []int{20, 21}, DarkThemeMainColor: 22, DarkThemeOtherColors: []int{23}},
		IsBurned: true}
	return &tg.TonWalletCard{Address: "EQfake1", Amount: "1000", BackgroundNFTAddress: "EQfake2",
		Stickers: []tg.TonWalletCardSticker{{Position: b65ePositionV(), NFT: tg.TonNFT{Address: "EQfake2",
			CollectionAddress: "EQfake3", Index: "7", Name: "n", Description: "d",
			Attributes: []tg.TonNFTAttribute{{TraitType: "a1", Value: "v1"}, {TraitType: "a2", Value: "v2"}}, Gift: gift}}}}
}

func b65eWallet() *tg.WidgetQueryWallet { return &tg.WidgetQueryWallet{Address: "a", Amount: "1"} }

// b65eQueryUpdate puts a WidgetQueryType JSON into the widget_query of a whole Update.
func b65eQueryUpdate(qtype string) string {
	return `{"update_id":1,"widget_query":{"id":"q","from":` + b65eUser + `,"chat_instance":"ci","type":` + qtype + `,"data":"d"}}`
}

// T-qtype
func TestB65E_QueryType(t *testing.T) {
	minCard := `{"address":"EQfake1","amount":"1","stickers":[{"position":` + b65ePosition + `,` +
		`"nft":{"address":"EQfake2","index":"1","name":"n","description":"d"}}]}`
	for _, tc := range []struct {
		name    string
		in      string
		prefill tg.WidgetQueryType
		want    tg.WidgetQueryType
		out     string // the exact re-encode; "" = not checked
	}{
		{name: "01_full_card", in: `{"type":"data","wallet_card":` + b65eCard(b65eGiftHead+`,"gift_id":"g99"}`) + `}`,
			want: tg.WidgetQueryType{Type: "data", WalletCard: b65eCardV()},
			out:  `{"type":"data","wallet_card":` + b65eCard(b65eGiftHead+`}`) + `}`},
		{name: "01b_min_nft", in: `{"type":"data","wallet_card":` + minCard + `}`,
			want: tg.WidgetQueryType{Type: "data", WalletCard: &tg.TonWalletCard{Address: "EQfake1", Amount: "1",
				Stickers: []tg.TonWalletCardSticker{{Position: b65ePositionV(),
					NFT: tg.TonNFT{Address: "EQfake2", Index: "1", Name: "n", Description: "d"}}}}},
			out: `{"type":"data","wallet_card":` + minCard + `}`},
		{name: "02_data_no_card", in: `{"type":"data"}`, want: tg.WidgetQueryType{Type: "data"}, out: `{"type":"data"}`},
		{name: "03_sign_data", in: `{"type":"sign_data","wallet":{"address":"EQfake5","amount":"2","public_key":"ab","state_init":"te6"},"sign_data":"{\"t\":3}"}`,
			want: tg.WidgetQueryType{Type: "sign_data", Wallet: &tg.WidgetQueryWallet{Address: "EQfake5", Amount: "2",
				PublicKey: "ab", StateInit: "te6"}, SignData: `{"t":3}`},
			out: `{"type":"sign_data","wallet":{"address":"EQfake5","amount":"2","public_key":"ab","state_init":"te6"},"sign_data":"{\"t\":3}"}`},
		{name: "04_sent_transaction", in: `{"type":"sent_transaction","wallet":{"address":"EQfake6","amount":"3"},"msg_hash":"h4"}`,
			want: tg.WidgetQueryType{Type: "sent_transaction", Wallet: &tg.WidgetQueryWallet{Address: "EQfake6", Amount: "3"}, MsgHash: "h4"},
			out:  `{"type":"sent_transaction","wallet":{"address":"EQfake6","amount":"3"},"msg_hash":"h4"}`},
		{name: "05_future", in: `{"type":"future","x":1}`, want: tg.WidgetQueryType{Type: "future"}, out: `{"type":"future"}`},
		{name: "06_future_wallet", in: `{"type":"future","x":1,"wallet":{"address":"a","amount":"1"}}`,
			want: tg.WidgetQueryType{Type: "future", Wallet: b65eWallet()}, out: `{"type":"future","wallet":{"address":"a","amount":"1"}}`},
		{name: "07c_null_zero", in: `{"type":"future","wallet":null,"sign_data":null}`, want: tg.WidgetQueryType{Type: "future"}},
		{name: "07d_null_prefilled", in: `{"wallet":null,"sign_data":null}`,
			prefill: tg.WidgetQueryType{Type: "t0", Wallet: b65eWallet(), SignData: "s0"},
			want:    tg.WidgetQueryType{Type: "t0", SignData: "s0"}},
		{name: "08_empty_sign_data", in: `{"type":"sign_data","wallet":{"address":"a","amount":"1"},"sign_data":""}`,
			want: tg.WidgetQueryType{Type: "sign_data", Wallet: b65eWallet()},
			out:  `{"type":"sign_data","wallet":{"address":"a","amount":"1"}}`},
		{name: "08b_empty_msg_hash", in: `{"type":"sent_transaction","wallet":{"address":"a","amount":"1"},"msg_hash":""}`,
			want: tg.WidgetQueryType{Type: "sent_transaction", Wallet: b65eWallet()},
			out:  `{"type":"sent_transaction","wallet":{"address":"a","amount":"1"}}`},
		{name: "09_folded_keys", in: `{"TYPE":"sign_data","WaLlEt":{"ADDRESS":"a","amount":"1"},"Sign_Data":"{\"t\":9}"}`,
			want: tg.WidgetQueryType{Type: "sign_data", Wallet: b65eWallet(), SignData: `{"t":9}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.prefill
			require.NoError(t, json.Unmarshal([]byte(tc.in), &got))
			assert.Equal(t, tc.want, got)
			if tc.out != "" {
				assert.Equal(t, tc.out, b65eEncode(t, got))
			}
		})
	}
	// The cases decoded as widget_query.type of a whole Update.
	for _, tc := range []struct{ name, qtype, field string }{
		{"07_wallet_string", `{"type":"future","wallet":"EQfake1"}`, "widget_query.type.wallet"},
		{"07b_nested_misfit", `{"type":"future","wallet":{"address":5}}`, "widget_query.type.wallet.address"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var u tg.Update
			err := json.Unmarshal([]byte(b65eQueryUpdate(tc.qtype)), &u)
			var te *json.UnmarshalTypeError
			require.ErrorAs(t, err, &te)
			assert.Equal(t, tc.field, te.Field)
		})
	}
	t.Run("10_mixed", func(t *testing.T) {
		var u tg.Update
		require.NoError(t, json.Unmarshal([]byte(b65eQueryUpdate(`{"type":"data","msg_hash":"h"}`)), &u))
		assert.Equal(t, tg.Update{UpdateID: 1, WidgetQuery: &tg.WidgetQuery{ID: "q", From: b65eUserV(), ChatInstance: "ci",
			Type: tg.WidgetQueryType{Type: "data", MsgHash: "h"}, Data: "d"}}, u)
	})
}

// The T-query and T-shared children, as the encoder writes them.
const (
	b65eQueryChat = `{"id":"q1","from":` + b65eUser + `,"chat":` + b65eGroup + `,"message_id":5,"chat_instance":"ci1",` +
		`"type":{"type":"sign_data","wallet":{"address":"a","amount":"1"},"sign_data":"{\"t\":1}"},"data":"d1","widget_id":"w1"}`
	b65eQueryInline = `{"id":"q2","from":` + b65eUser + `,"inline_message_id":"im1","chat_instance":"ci2","chat_type":"sender",` +
		`"type":{"type":"data"},"data":"d2"}`
	b65eShared = `{"id":"q3","from":` + b65eUser + `,"type":{"type":"data"},"data":"d3","widget_id":"w3"}`
)

// T-query
func TestB65E_Query(t *testing.T) {
	for _, tc := range []struct {
		name, child string
		want        *tg.WidgetQuery
	}{
		{"chat", b65eQueryChat, &tg.WidgetQuery{ID: "q1", From: b65eUserV(), Chat: b65eGroupV(), MessageID: 5, ChatInstance: "ci1",
			Type: tg.WidgetQueryType{Type: "sign_data", Wallet: b65eWallet(), SignData: `{"t":1}`}, Data: "d1", WidgetID: "w1"}},
		{"inline", b65eQueryInline, &tg.WidgetQuery{ID: "q2", From: b65eUserV(), InlineMessageID: "im1", ChatInstance: "ci2",
			ChatType: "sender", Type: tg.WidgetQueryType{Type: "data"}, Data: "d2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var u tg.Update
			require.NoError(t, json.Unmarshal([]byte(`{"update_id":10,"widget_query":`+tc.child+`}`), &u))
			assert.Equal(t, tg.Update{UpdateID: 10, WidgetQuery: tc.want}, u)
			assert.Equal(t, tc.child, b65eEncode(t, u.WidgetQuery))
			b65eHolderKey(t, u, "widget_query", u.WidgetQuery)
		})
	}
}

// T-shared
func TestB65E_Shared(t *testing.T) {
	var u tg.Update
	require.NoError(t, json.Unmarshal([]byte(`{"update_id":12,"shared_widget_query":`+b65eShared+`}`), &u))
	assert.Equal(t, tg.Update{UpdateID: 12, SharedWidgetQuery: &tg.SharedWidgetQuery{ID: "q3", From: b65eUserV(),
		Type: tg.WidgetQueryType{Type: "data"}, Data: "d3", WidgetID: "w3"}}, u)
	assert.Equal(t, b65eShared, b65eEncode(t, u.SharedWidgetQuery))
	b65eHolderKey(t, u, "shared_widget_query", u.SharedWidgetQuery)
}

// T-zero: every required key present, no optional key.
func TestB65E_Zero(t *testing.T) {
	pos0 := `{"x_percentage":0,"y_percentage":0,"width_percentage":0,"height_percentage":0,"rotation_angle":0,"corner_radius_percentage":0}`
	nft0 := `{"address":"","index":"","name":"","description":""}`
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"WidgetQuery", tg.WidgetQuery{}, `{"id":"","from":null,"chat_instance":"","type":{"type":""},"data":""}`},
		{"SharedWidgetQuery", tg.SharedWidgetQuery{}, `{"id":"","from":null,"type":{"type":""},"data":"","widget_id":""}`},
		{"WidgetQueryWallet", tg.WidgetQueryWallet{}, `{"address":"","amount":""}`},
		{"TonWalletCard", tg.TonWalletCard{}, `{"address":"","amount":"","stickers":null}`},
		{"TonWalletCardSticker", tg.TonWalletCardSticker{}, `{"position":` + pos0 + `,"nft":` + nft0 + `}`},
		{"TonNFT", tg.TonNFT{}, nft0},
		{"TonNFTAttribute", tg.TonNFTAttribute{}, `{"trait_type":"","value":""}`},
		{"AreaPosition", tg.AreaPosition{}, pos0},
		{"WidgetQueryType", tg.WidgetQueryType{}, `{"type":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, b65eEncode(t, tc.v))
		})
	}
}

// T-consts
func TestB65E_Consts(t *testing.T) {
	for _, tc := range []struct{ name, got, want string }{
		{"data", tg.WidgetQueryTypeData, "data"},
		{"sign_data", tg.WidgetQueryTypeSignData, "sign_data"},
		{"sent_transaction", tg.WidgetQueryTypeSentTransaction, "sent_transaction"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
}

type b65eField struct {
	Name, Type, Tag string
	Anonymous       bool
}

func b65eFields(rt reflect.Type) []b65eField {
	var out []b65eField
	for i := range rt.NumField() {
		f := rt.Field(i)
		out = append(out, b65eField{f.Name, f.Type.String(), string(f.Tag), f.Anonymous})
	}
	return out
}

// T-shapes: one leaf per new type and one per holder field (the hand-written lists of PLAN §4.1).
func TestB65E_Shapes(t *testing.T) {
	f := func(name, typ, tag string) b65eField { return b65eField{name, typ, `json:"` + tag + `"`, false} }
	for _, s := range []struct {
		v      any
		fields []b65eField
	}{
		{tg.Widget{}, []b65eField{f("ID", "string", "id,omitempty"), f("Width", "int", "width,omitempty"),
			f("Height", "int", "height,omitempty"), f("HTML", "string", "html"), f("PublicData", "string", "public_data,omitempty"),
			f("ChatData", "string", "chat_data,omitempty")}},
		{tg.WidgetQueryType{}, []b65eField{f("Type", "string", "type"), f("WalletCard", "*tg.TonWalletCard", "wallet_card,omitempty"),
			f("Wallet", "*tg.WidgetQueryWallet", "wallet,omitempty"), f("SignData", "string", "sign_data,omitempty"),
			f("MsgHash", "string", "msg_hash,omitempty")}},
		{tg.WidgetQueryWallet{}, []b65eField{f("Address", "string", "address"), f("Amount", "string", "amount"),
			f("PublicKey", "string", "public_key,omitempty"), f("StateInit", "string", "state_init,omitempty")}},
		{tg.WidgetQuery{}, []b65eField{f("ID", "string", "id"), f("From", "*tg.User", "from"), f("Chat", "*tg.Chat", "chat,omitempty"),
			f("MessageID", "int", "message_id,omitempty"), f("InlineMessageID", "string", "inline_message_id,omitempty"),
			f("ChatInstance", "string", "chat_instance"), f("ChatType", "string", "chat_type,omitempty"),
			f("Type", "tg.WidgetQueryType", "type"), f("Data", "string", "data"), f("WidgetID", "string", "widget_id,omitempty")}},
		{tg.SharedWidgetQuery{}, []b65eField{f("ID", "string", "id"), f("From", "*tg.User", "from"),
			f("Type", "tg.WidgetQueryType", "type"), f("Data", "string", "data"), f("WidgetID", "string", "widget_id")}},
		{tg.TonWalletCard{}, []b65eField{f("Address", "string", "address"), f("Amount", "string", "amount"),
			f("Stickers", "[]tg.TonWalletCardSticker", "stickers"), f("BackgroundNFTAddress", "string", "background_nft_address,omitempty")}},
		{tg.TonWalletCardSticker{}, []b65eField{f("Position", "tg.AreaPosition", "position"), f("NFT", "tg.TonNFT", "nft")}},
		{tg.TonNFT{}, []b65eField{f("Address", "string", "address"), f("CollectionAddress", "string", "collection_address,omitempty"),
			f("Index", "string", "index"), f("Name", "string", "name"), f("Description", "string", "description"),
			f("Attributes", "[]tg.TonNFTAttribute", "attributes,omitempty"), f("Gift", "*tg.UniqueGift", "gift,omitempty")}},
		{tg.TonNFTAttribute{}, []b65eField{f("TraitType", "string", "trait_type"), f("Value", "string", "value")}},
		{tg.AreaPosition{}, []b65eField{f("XPercentage", "float64", "x_percentage"), f("YPercentage", "float64", "y_percentage"),
			f("WidthPercentage", "float64", "width_percentage"), f("HeightPercentage", "float64", "height_percentage"),
			f("RotationAngle", "float64", "rotation_angle"), f("CornerRadiusPercentage", "float64", "corner_radius_percentage")}},
	} {
		rt := reflect.TypeOf(s.v)
		t.Run(rt.Name(), func(t *testing.T) {
			assert.Equal(t, s.fields, b65eFields(rt))
			assert.Equal(t, len(s.fields), rt.NumField(), "NumField")
			assert.Equal(t, 0, rt.NumMethod(), "no value methods")
			assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod(), "no pointer methods")
		})
	}
	type holder struct {
		b65eField
		Prev string
	}
	for _, h := range []struct {
		v           any
		field, prev string
		want        b65eField
	}{
		{tg.Message{}, "Widget", "Voice", f("Widget", "*tg.Widget", "widget,omitempty")},
		{tg.ExternalReplyInfo{}, "Widget", "Voice", f("Widget", "*tg.Widget", "widget,omitempty")},
		{tg.Update{}, "WidgetQuery", "CallbackQuery", f("WidgetQuery", "*tg.WidgetQuery", "widget_query,omitempty")},
		{tg.Update{}, "SharedWidgetQuery", "WidgetQuery", f("SharedWidgetQuery", "*tg.SharedWidgetQuery", "shared_widget_query,omitempty")},
	} {
		rt := reflect.TypeOf(h.v)
		t.Run("holder_"+rt.Name()+"."+h.field, func(t *testing.T) {
			sf, ok := rt.FieldByName(h.field)
			require.True(t, ok, "field %s", h.field)
			require.Len(t, sf.Index, 1, "a direct field")
			require.Positive(t, sf.Index[0])
			got := holder{b65eField{sf.Name, sf.Type.String(), string(sf.Tag), sf.Anonymous}, rt.Field(sf.Index[0] - 1).Name}
			assert.Equal(t, holder{h.want, h.prev}, got)
		})
	}
}

// T-old-zero: the zero holders encode as before 65e (the base bytes, measured at 27b9535).
func TestB65E_OldZero(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"Message", tg.Message{}, `{"message_id":0,"date":0,"chat":null}`},
		{"Update", tg.Update{}, `{"update_id":0}`},
		{"ExternalReplyInfo", tg.ExternalReplyInfo{}, `{"origin":{"type":"","date":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, b65eEncode(t, tc.v))
		})
	}
}
