package tg_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-48b1: the Bot API 7.6 paid media read side (PaidMediaInfo, PaidMedia with its 10.0 live_photo kind,
// PaidMediaPurchased) and the holders Message.paid_media, ExternalReplyInfo.paid_media, TransactionPartnerUser.paid_media,
// Update.purchased_paid_media. No test builds a new type with a keyed composite literal (only zero values like
// tg.PaidMedia{}): values come from decoded JSON and are read by selectors, so a field mutant compiles and fails by
// assertion. Ids are fake. The helpers api96Strict
// (tg/api96_test.go), api102aMarshal and api102aKeys (tg/api102a_test.go) are reused.
const (
	b48bPhoto   = `{"type":"photo","photo":[{"file_id":"pp","file_unique_id":"ppu","width":90,"height":90}]}`
	b48bVideo   = `{"type":"video","video":{"file_id":"pv","file_unique_id":"pvu","width":320,"height":240,"duration":3}}`
	b48bPreview = `{"type":"preview","width":640,"height":480,"duration":7}`
	b48bLive    = `{"type":"live_photo","live_photo":{"photo":[{"file_id":"lp","file_unique_id":"lpu","width":1,"height":1}],` +
		`"file_id":"lv","file_unique_id":"lvu","width":2,"height":2,"duration":1}}`
	b48bInfo      = `{"star_count":25000,"paid_media":[` + b48bPhoto + `,` + b48bVideo + `,` + b48bPreview + `,` + b48bLive + `]}`
	b48bPurchased = `{"from":{"id":5000000002,"is_bot":false,"first_name":"Buyer"},"paid_media_payload":"order-42"}`
	b48bMsgBase   = `{"message_id":2001,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}`
	b48bMsg       = b48bMsgBase + `,"paid_media":` + b48bInfo + `}`
	b48bMsgReply  = b48bMsgBase + `,"external_reply":{"origin":{"type":"chat","date":1700000001,` +
		`"sender_chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}},` +
		`"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"},"message_id":2000,"paid_media":` + b48bInfo + `}}`
	b48bPartner = `{"type":"user","user":{"id":5000000002,"is_bot":false,"first_name":"Buyer"},` +
		`"paid_media":[` + b48bPhoto + `,` + b48bPreview + `],"paid_media_payload":"order-42"}`
	b48bUpdate = `{"update_id":9001,"purchased_paid_media":` + b48bPurchased + `}`
)

// T-kind-1..4: each PaidMedia kind decodes strictly, fills only its own fields and re-encodes to the input;
// T-kind-5: a future kind decodes with no error.
func TestB48B1_Kinds(t *testing.T) {
	decode := func(t *testing.T, in string) *tg.PaidMedia {
		t.Helper()
		var pm tg.PaidMedia
		require.NoError(t, api96Strict(t, []byte(in), &pm))
		assert.JSONEq(t, in, api102aMarshal(t, pm), "re-encode")
		return &pm
	}
	t.Run("photo", func(t *testing.T) {
		pm := decode(t, b48bPhoto)
		assert.Equal(t, "photo", pm.Type)
		require.Len(t, pm.Photo, 1)
		assert.Equal(t, "pp", pm.Photo[0].FileID)
		assert.Equal(t, 90, pm.Photo[0].Width)
		assert.Nil(t, pm.Video)
		assert.Nil(t, pm.LivePhoto)
		assert.Zero(t, pm.Width)
		assert.Zero(t, pm.Height)
		assert.Zero(t, pm.Duration)
	})
	t.Run("video", func(t *testing.T) {
		pm := decode(t, b48bVideo)
		assert.Equal(t, "video", pm.Type)
		require.NotNil(t, pm.Video)
		assert.Equal(t, "pv", pm.Video.FileID)
		assert.Equal(t, 3, pm.Video.Duration)
		assert.Nil(t, pm.Photo)
		assert.Nil(t, pm.LivePhoto)
		assert.Zero(t, pm.Width)
		assert.Zero(t, pm.Height)
		assert.Zero(t, pm.Duration)
	})
	t.Run("preview", func(t *testing.T) {
		pm := decode(t, b48bPreview)
		assert.Equal(t, "preview", pm.Type)
		assert.Equal(t, 640, pm.Width)
		assert.Equal(t, 480, pm.Height)
		assert.Equal(t, 7, pm.Duration)
		assert.Nil(t, pm.Photo)
		assert.Nil(t, pm.Video)
		assert.Nil(t, pm.LivePhoto)
	})
	t.Run("live_photo", func(t *testing.T) {
		pm := decode(t, b48bLive)
		assert.Equal(t, "live_photo", pm.Type)
		require.NotNil(t, pm.LivePhoto)
		assert.Equal(t, "lv", pm.LivePhoto.FileID)
		require.Len(t, pm.LivePhoto.Photo, 1)
		assert.Equal(t, "lp", pm.LivePhoto.Photo[0].FileID)
		assert.Nil(t, pm.Photo)
		assert.Nil(t, pm.Video)
		assert.Zero(t, pm.Width)
		assert.Zero(t, pm.Height)
		assert.Zero(t, pm.Duration)
	})
	t.Run("future", func(t *testing.T) {
		pm := decode(t, `{"type":"x"}`)
		assert.Equal(t, "x", pm.Type)
	})
	t.Run("preview_no_size", func(t *testing.T) { // the three preview keys are optional
		pm := decode(t, `{"type":"preview"}`)
		assert.Equal(t, "preview", pm.Type)
	})
}

// T-info: PaidMediaInfo with every kind; T-pur: PaidMediaPurchased.
func TestB48B1_InfoPurchased(t *testing.T) {
	var info tg.PaidMediaInfo
	require.NoError(t, api96Strict(t, []byte(b48bInfo), &info))
	assert.Equal(t, 25000, info.StarCount)
	require.Len(t, info.PaidMedia, 4)
	assert.Equal(t, []string{"photo", "video", "preview", "live_photo"},
		[]string{info.PaidMedia[0].Type, info.PaidMedia[1].Type, info.PaidMedia[2].Type, info.PaidMedia[3].Type})
	assert.JSONEq(t, b48bInfo, api102aMarshal(t, info))

	var empty tg.PaidMediaInfo
	require.NoError(t, api96Strict(t, []byte(`{"star_count":1,"paid_media":[]}`), &empty))
	assert.Equal(t, 1, empty.StarCount)
	assert.NotNil(t, empty.PaidMedia)
	assert.Empty(t, empty.PaidMedia)
	assert.JSONEq(t, `{"star_count":1,"paid_media":[]}`, api102aMarshal(t, empty))

	var pur tg.PaidMediaPurchased
	require.NoError(t, api96Strict(t, []byte(b48bPurchased), &pur))
	assert.Equal(t, int64(5000000002), pur.From.ID)
	assert.Equal(t, "Buyer", pur.From.FirstName)
	assert.Equal(t, "order-42", pur.PaidMediaPayload)
	assert.JSONEq(t, b48bPurchased, api102aMarshal(t, pur))
}

// T-hold-1..5: each holder decodes from a server-shaped JSON with its key.
func TestB48B1_Holders(t *testing.T) {
	t.Run("message", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48bMsg), &m))
		require.NotNil(t, m.PaidMedia)
		assert.Equal(t, 25000, m.PaidMedia.StarCount)
		require.Len(t, m.PaidMedia.PaidMedia, 4)
		assert.Equal(t, "video", m.PaidMedia.PaidMedia[1].Type)
		assert.Nil(t, m.Photo)
		assert.Nil(t, m.Video)
		assert.Nil(t, m.ExternalReply)
		assert.JSONEq(t, b48bMsg, api102aMarshal(t, m))
	})
	t.Run("external_reply", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48bMsgReply), &m))
		require.NotNil(t, m.ExternalReply)
		require.NotNil(t, m.ExternalReply.PaidMedia)
		assert.Equal(t, 25000, m.ExternalReply.PaidMedia.StarCount)
		require.Len(t, m.ExternalReply.PaidMedia.PaidMedia, 4)
		assert.Equal(t, "live_photo", m.ExternalReply.PaidMedia.PaidMedia[3].Type)
		assert.Nil(t, m.PaidMedia)
		assert.JSONEq(t, b48bMsgReply, api102aMarshal(t, m))
	})
	t.Run("transaction_partner", func(t *testing.T) {
		var p tg.TransactionPartnerUser
		require.NoError(t, api96Strict(t, []byte(b48bPartner), &p))
		require.Len(t, p.PaidMedia, 2)
		assert.Equal(t, "photo", p.PaidMedia[0].Type)
		assert.Equal(t, 640, p.PaidMedia[1].Width)
		assert.Equal(t, "order-42", p.PaidMediaPayload)
		assert.JSONEq(t, b48bPartner, api102aMarshal(t, p))
	})
	t.Run("star_transaction", func(t *testing.T) { // through StarTransaction's own Source decode
		// T3-01: every TransactionPartnerUser key is set, and the whole value must equal the direct strict decode, so a
		// Source decode that loses any field fails. T2-02: plus a key galigo does not know (the lenient path keeps it).
		full := strings.Replace(b48bPartner, `"paid_media_payload":"order-42"}`, `"paid_media_payload":"order-42","invoice_payload":"inv-7",`+
			`"subscription_period":2592000,"gift":{"id":"g1","sticker":{"file_id":"s","file_unique_id":"su","type":"regular",`+
			`"width":1,"height":1,"is_animated":false,"is_video":false},"star_count":5}}`, 1)
		require.NotEqual(t, b48bPartner, full)
		var direct tg.TransactionPartnerUser
		require.NoError(t, api96Strict(t, []byte(full), &direct))
		require.NotNil(t, direct.Gift)
		require.Equal(t, 2592000, direct.SubscriptionPeriod)
		src0 := strings.Replace(full, `{"type":"user",`, `{"type":"user","x_future":1,`, 1)
		require.NotEqual(t, full, src0)
		var tx tg.StarTransaction
		require.NoError(t, json.Unmarshal([]byte(`{"id":"tx-1","amount":25000,"date":1700000000,"source":`+src0+`}`), &tx))
		src, ok := tx.Source.(tg.TransactionPartnerUser)
		require.True(t, ok, "source is %T", tx.Source)
		assert.Equal(t, direct, src, "the Source decode keeps every field")
		require.Len(t, src.PaidMedia, 2)
		assert.Equal(t, "preview", src.PaidMedia[1].Type)
		assert.Equal(t, 480, src.PaidMedia[1].Height)
		// C1-01: the item list keeps a future kind, a preview with no size, and the source order (not sorted by type).
		// C3-01 (fixed after the third review, not re-reviewed): plus a video, a live photo and a photo with two sizes,
		// and the same list without paid_media_payload. C3-02: both through Source and through Receiver.
		photo2 := `{"type":"photo","photo":[{"file_id":"a","file_unique_id":"au","width":90,"height":90},` +
			`{"file_id":"b","file_unique_id":"bu","width":320,"height":320}]}`
		mixed := strings.Replace(src0, `"paid_media":[`, `"paid_media":[{"type":"x","x_new":1},{"type":"preview"},`+
			b48bVideo+`,`+b48bLive+`,`+photo2+`,`, 1)
		require.NotEqual(t, src0, mixed)
		nopay := strings.Replace(mixed, `"paid_media_payload":"order-42",`, ``, 1)
		require.NotEqual(t, mixed, nopay)
		for _, in := range []string{mixed, nopay} {
			var want tg.TransactionPartnerUser
			require.NoError(t, json.Unmarshal([]byte(in), &want))
			require.Len(t, want.PaidMedia, 7)
			require.NotNil(t, want.PaidMedia[2].Video)
			require.NotNil(t, want.PaidMedia[3].LivePhoto)
			require.Len(t, want.PaidMedia[4].Photo, 2)
			for _, key := range []string{"source", "receiver"} {
				var tx2 tg.StarTransaction
				require.NoError(t, json.Unmarshal([]byte(`{"id":"tx-1b","amount":1,"date":1700000000,"`+key+`":`+in+`}`), &tx2))
				got := tx2.Source
				if key == "receiver" {
					got = tx2.Receiver
				}
				assert.Equal(t, want, got, "the %s decode keeps every item, in order", key)
			}
		}
	})
	t.Run("star_transaction_fallback", func(t *testing.T) { // P1-02: a paid_media item that does not decode keeps the raw partner
		// C1-01: one space after the first comma, so Raw must be the source bytes, not a compacted copy.
		bad := `{"type":"user", "user":{"id":5000000002,"is_bot":false,"first_name":"Buyer"},"paid_media":[{"type":"photo","photo":"x"}]}`
		for _, key := range []string{"source", "receiver"} { // C3-02: the receiver decode too
			var tx tg.StarTransaction
			require.NoError(t, json.Unmarshal([]byte(`{"id":"tx-2","amount":1,"date":1700000000,"`+key+`":`+bad+`}`), &tx))
			p := tx.Source
			if key == "receiver" {
				p = tx.Receiver
			}
			src, ok := p.(tg.TransactionPartnerUnknown)
			require.True(t, ok, "%s is %T", key, p)
			assert.Equal(t, "user", src.Type)
			assert.Equal(t, bad, string(src.Raw), "Raw keeps the %s bytes exactly (T1-04)", key)
		}
	})
	t.Run("update", func(t *testing.T) {
		var u tg.Update
		require.NoError(t, api96Strict(t, []byte(b48bUpdate), &u))
		require.NotNil(t, u.PurchasedPaidMedia)
		assert.Equal(t, int64(5000000002), u.PurchasedPaidMedia.From.ID)
		assert.Equal(t, "order-42", u.PurchasedPaidMedia.PaidMediaPayload)
		assert.Nil(t, u.Message)
		assert.JSONEq(t, b48bUpdate, api102aMarshal(t, u))
	})
}

// T-cmp (P2-01): the paid_media slice makes TransactionPartnerUser non-comparable (the planned G-api incompatible line);
// the other holders keep their comparability.
func TestB48B1_Comparable(t *testing.T) {
	assert.False(t, reflect.TypeOf(tg.TransactionPartnerUser{}).Comparable(), "TransactionPartnerUser")
	assert.True(t, reflect.TypeOf(tg.Update{}).Comparable(), "Update")
	assert.True(t, reflect.TypeOf(tg.PaidMediaPurchased{}).Comparable(), "PaidMediaPurchased")
	assert.False(t, reflect.TypeOf(tg.PaidMediaInfo{}).Comparable(), "PaidMediaInfo")
	assert.False(t, reflect.TypeOf(tg.PaidMedia{}).Comparable(), "PaidMedia")
	// P3-01: StarTransaction still compiles with ==, but == on two user partners inside the interface panics at run time.
	assert.True(t, reflect.TypeOf(tg.StarTransaction{}).Comparable(), "StarTransaction")
	var tx tg.StarTransaction
	require.NoError(t, json.Unmarshal([]byte(`{"id":"tx-1","amount":1,"date":1700000000,"source":`+b48bPartner+`}`), &tx))
	other := tx
	assert.PanicsWithError(t, "runtime error: comparing uncomparable type tg.TransactionPartnerUser", func() { _ = tx.Source == other.Source })
	assert.NotPanics(t, func() { _ = tx.Source == nil })
	// T1-02: a map key that holds the user partner panics too.
	assert.PanicsWithError(t, "runtime error: hash of unhashable type tg.TransactionPartnerUser", func() {
		m := map[tg.TransactionPartner]int{}
		m[tx.Source] = 1
	})
	// T2-01: a comparison with a different partner type does not panic (the note says it is safe).
	var fr tg.StarTransaction
	require.NoError(t, json.Unmarshal([]byte(`{"id":"tx-3","amount":1,"date":1700000000,"source":{"type":"fragment"}}`), &fr))
	require.IsType(t, tg.TransactionPartnerFragment{}, fr.Source)
	assert.NotPanics(t, func() { assert.False(t, tx.Source == fr.Source) })
}

// T-zero: a zero value encodes only the required keys; zero holders encode no new key.
func TestB48B1_ZeroEncode(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{tg.PaidMedia{}, `{"type":""}`},
		{tg.PaidMediaInfo{}, `{"star_count":0,"paid_media":null}`},
		{tg.PaidMediaPurchased{}, `{"from":{"id":0,"is_bot":false,"first_name":""},"paid_media_payload":""}`},
	} {
		assert.Equal(t, tc.want, api102aMarshal(t, tc.v), "%T", tc.v)
	}
	for _, tc := range []struct {
		v   any
		key string
	}{
		{tg.Message{}, "paid_media"}, {tg.ExternalReplyInfo{}, "paid_media"},
		{tg.TransactionPartnerUser{}, "paid_media"}, {tg.Update{}, "purchased_paid_media"},
	} {
		assert.NotContains(t, api102aKeys(t, tc.v), tc.key, "%T", tc.v)
	}
}

// The spec key lists, in spec order (E25/lookups/sec-b48b.txt); PaidMedia is the flat union of its four kinds, in the
// spec's kind order (live_photo, photo, preview, video).
var b48bSpecKeys = map[string][]string{
	"PaidMediaInfo":      {"star_count", "paid_media"},
	"PaidMedia":          {"type", "live_photo", "photo", "width", "height", "duration", "video"},
	"PaidMediaPurchased": {"from", "paid_media_payload"},
}

// T-shape: each new type's fields, in order, equal the planned rows exactly (name, Go type, JSON tag, not embedded);
// the tag keys equal the spec key list; no methods. T-shape/holders: the four holder fields and their places.
func TestB48B1_Shapes(t *testing.T) {
	type row struct{ name, typ, tag string }
	shapes := []struct {
		v    any
		rows []row
	}{
		{tg.PaidMediaInfo{}, []row{{"StarCount", "int", "star_count"}, {"PaidMedia", "[]tg.PaidMedia", "paid_media"}}},
		{tg.PaidMedia{}, []row{
			{"Type", "string", "type"}, {"LivePhoto", "*tg.LivePhoto", "live_photo,omitempty"},
			{"Photo", "[]tg.PhotoSize", "photo,omitempty"}, {"Width", "int", "width,omitempty"},
			{"Height", "int", "height,omitempty"}, {"Duration", "int", "duration,omitempty"}, {"Video", "*tg.Video", "video,omitempty"},
		}},
		{tg.PaidMediaPurchased{}, []row{{"From", "tg.User", "from"}, {"PaidMediaPayload", "string", "paid_media_payload"}}},
	}
	for _, s := range shapes {
		rt := reflect.TypeOf(s.v)
		t.Run(rt.Name(), func(t *testing.T) {
			require.Equal(t, len(s.rows), rt.NumField(), "field count")
			var keys []string
			for i := range rt.NumField() {
				f := rt.Field(i)
				assert.False(t, f.Anonymous, "%s.%s must be a named field", rt.Name(), f.Name)
				assert.Equal(t, s.rows[i], row{f.Name, f.Type.String(), f.Tag.Get("json")}, "%s field %d", rt.Name(), i)
				key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				keys = append(keys, key)
			}
			assert.Equal(t, b48bSpecKeys[rt.Name()], keys, "the spec keys in spec order")
			assert.Equal(t, 0, rt.NumMethod(), "no value methods")
			assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod(), "no pointer methods")
		})
	}
	t.Run("holders", func(t *testing.T) {
		for _, h := range []struct {
			v     any
			r     row
			after string // PLAN D3: the field the holder follows
		}{
			{tg.Message{}, row{"PaidMedia", "*tg.PaidMediaInfo", "paid_media,omitempty"}, "Document"},
			{tg.ExternalReplyInfo{}, row{"PaidMedia", "*tg.PaidMediaInfo", "paid_media,omitempty"}, "LivePhoto"},
			{tg.TransactionPartnerUser{}, row{"PaidMedia", "[]tg.PaidMedia", "paid_media,omitempty"}, "SubscriptionPeriod"},
			{tg.Update{}, row{"PurchasedPaidMedia", "*tg.PaidMediaPurchased", "purchased_paid_media,omitempty"}, "PreCheckoutQuery"},
		} {
			rt := reflect.TypeOf(h.v)
			f, ok := rt.FieldByName(h.r.name)
			p, okp := rt.FieldByName(h.after)
			if assert.True(t, ok, "%s.%s", rt.Name(), h.r.name) && assert.True(t, okp, "%s.%s", rt.Name(), h.after) {
				assert.False(t, f.Anonymous, "%s.%s must be a named field", rt.Name(), h.r.name)
				assert.Equal(t, h.r, row{f.Name, f.Type.String(), f.Tag.Get("json")}, rt.Name())
				assert.Equal(t, []int{p.Index[0] + 1}, f.Index, "%s.%s must follow %s", rt.Name(), h.r.name, h.after)
			}
		}
	})
	// T1-05, T3-02: the full field order of the holders without another full-order row (ExternalReplyInfo: the
	// TestB48A_Shapes rows). Base order plus the one planned field (Message: 61 base names, PaidMedia after Document).
	t.Run("holder_order", func(t *testing.T) {
		for _, h := range []struct {
			v     any
			names []string
		}{
			{tg.Message{}, []string{"MessageID", "MessageThreadID", "From", "SenderChat", "ReceiverUser", "EphemeralMessageID", "Date", "Chat", "ForwardFrom", "ForwardFromChat", "ForwardDate", "ForwardOrigin", "IsTopicMessage", "IsAutomaticForward", "ReplyToMessage", "ExternalReply", "Quote", "ViaBot", "EditDate", "HasProtectedContent", "MediaGroupID", "AuthorSignature", "Text", "Entities", "RichMessage", "Caption", "CaptionEntities", "Photo", "Document", "PaidMedia", "Animation", "Video", "LivePhoto", "Audio", "Voice", "Sticker", "VideoNote", "Contact", "Location", "Venue", "Poll", "NewChatMembers", "LeftChatMember", "NewChatTitle", "NewChatPhoto", "DeleteChatPhoto", "GroupChatCreated", "SupergroupChatCreated", "ChannelChatCreated", "ChatOwnerLeft", "ChatOwnerChanged", "SenderTag", "ManagedBotCreated", "PollOptionAdded", "PollOptionDeleted", "ReplyToPollOptionID", "GuestBotCallerUser", "GuestBotCallerChat", "CommunityChatAdded", "CommunityChatRemoved", "ReplyMarkup", "GuestQueryID"}},
			{tg.TransactionPartnerUser{}, []string{"Type", "User", "InvoicePayload", "PaidMediaPayload", "SubscriptionPeriod", "PaidMedia", "Gift"}},
			{tg.Update{}, []string{"UpdateID", "Message", "EditedMessage", "ChannelPost", "EditedChannelPost", "CallbackQuery",
				"InlineQuery", "ChosenInlineResult", "ShippingQuery", "PreCheckoutQuery", "PurchasedPaidMedia", "Poll", "PollAnswer",
				"MyChatMember", "ChatMember", "ChatJoinRequest", "ManagedBot", "GuestMessage", "Subscription"}},
		} {
			rt := reflect.TypeOf(h.v)
			var got []string
			for i := range rt.NumField() {
				got = append(got, rt.Field(i).Name)
			}
			assert.Equal(t, h.names, got, "%s field order", rt.Name())
		}
	})
}

// T-own: walk every struct type reachable from tg.Update and tg.TransactionPartnerUser (the latter sits behind the
// TransactionPartner interface, where a walk stops) and check that the new keys live only on their planned holders.
func TestB48B1_KeyOwners(t *testing.T) {
	watched := map[string]bool{"paid_media": true, "purchased_paid_media": true, "paid_media_payload": true}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{}), reflect.TypeOf(tg.TransactionPartnerUser{})}
	for len(queue) > 0 {
		rt := queue[0]
		queue = queue[1:]
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Array || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			continue
		}
		seen[rt] = true
		for i := range rt.NumField() {
			f := rt.Field(i)
			if key, _, _ := strings.Cut(f.Tag.Get("json"), ","); watched[key] {
				owners = append(owners, rt.Name()+"."+f.Name)
			}
			queue = append(queue, f.Type)
		}
	}
	sort.Strings(owners)
	assert.Equal(t, []string{
		"ExternalReplyInfo.PaidMedia", "Message.PaidMedia", "PaidMediaInfo.PaidMedia", "PaidMediaPurchased.PaidMediaPayload",
		"TransactionPartnerUser.PaidMedia", "TransactionPartnerUser.PaidMediaPayload", "Update.PurchasedPaidMedia",
	}, owners)
	for _, v := range []any{tg.Message{}, tg.ExternalReplyInfo{}, tg.PaidMediaInfo{}, tg.PaidMedia{}, tg.PaidMediaPurchased{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
