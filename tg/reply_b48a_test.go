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

// B-48a: the Bot API 7.0 reply family (MessageOrigin, TextQuote, ExternalReplyInfo with Giveaway, GiveawayWinners,
// Invoice) and the Message fields forward_origin, external_reply, quote. No test builds a new type with a composite
// literal: values come from decoded JSON and are read by selectors, so a type mutant compiles and fails by assertion.
// Dates are above 2^31, so an int32 date fails to decode. Ids are fake. The helpers api96Strict (tg/api96_test.go),
// api102aMarshal and api102aKeys (tg/api102a_test.go) are reused.
const (
	b48aOriginUser    = `{"type":"user","date":4102444800,"sender_user":{"id":5000000001,"is_bot":true,"first_name":"Bot"}}`
	b48aOriginHidden  = `{"type":"hidden_user","date":4102444801,"sender_user_name":"Hidden Name"}`
	b48aOriginChat    = `{"type":"chat","date":4102444802,"sender_chat":{"id":-1001234567890,"type":"supergroup","title":"G"},"author_signature":"Admin"}`
	b48aOriginChannel = `{"type":"channel","date":4102444803,"chat":{"id":-1009876543210,"type":"channel","title":"C"},"message_id":77,"author_signature":"Editor"}`
	b48aQuote         = `{"text":"source","entities":[{"type":"bold","offset":0,"length":6}],"position":12,"is_manual":true}`
	b48aInvoice       = `{"title":"Ticket","description":"One ticket","start_parameter":"tk","currency":"XTR","total_amount":145}`
	b48aGiveaway      = `{"chats":[{"id":-1001111111111,"type":"channel","title":"C1"}],"winners_selection_date":4102444900,"winner_count":3,` +
		`"only_new_members":true,"has_public_winners":true,"prize_description":"Prize","country_codes":["DE","PL"],` +
		`"prize_star_count":500,"premium_subscription_month_count":6}`
	b48aWinners = `{"chat":{"id":-1002222222222,"type":"channel","title":"C2"},"giveaway_message_id":42,"winners_selection_date":4102444901,` +
		`"winner_count":2,"winners":[{"id":111,"is_bot":false,"first_name":"W1"},{"id":222,"is_bot":false,"first_name":"W2"}],` +
		`"additional_chat_count":1,"prize_star_count":50,"premium_subscription_month_count":3,"unclaimed_prize_count":1,` +
		`"only_new_members":true,"was_refunded":true,"prize_description":"Extra"}`
	b48aExternalFull = `{"origin":` + b48aOriginChannel + `,"chat":{"id":-1009876543210,"type":"channel","title":"C"},"message_id":77,` +
		`"link_preview_options":{"is_disabled":true},` +
		`"animation":{"file_id":"an","file_unique_id":"anu","width":1,"height":1,"duration":1},` +
		`"audio":{"file_id":"au","file_unique_id":"auu","duration":1},` +
		`"document":{"file_id":"do","file_unique_id":"dou"},` +
		`"live_photo":{"photo":[{"file_id":"lp","file_unique_id":"lpu","width":1,"height":1}],"file_id":"lv","file_unique_id":"lvu","width":1,"height":1,"duration":1},` +
		`"photo":[{"file_id":"ph","file_unique_id":"phu","width":2,"height":2}],` +
		`"sticker":{"file_id":"st","file_unique_id":"stu","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},` +
		`"story":{"chat":{"id":-1009876543210,"type":"channel"},"id":9},` +
		`"video":{"file_id":"vi","file_unique_id":"viu","width":1,"height":1,"duration":1},` +
		`"video_note":{"file_id":"vn","file_unique_id":"vnu","length":1,"duration":1},` +
		`"voice":{"file_id":"vo","file_unique_id":"vou","duration":1},` +
		`"has_media_spoiler":true,` +
		`"checklist":{"title":"Todo","tasks":[],"others_can_add_tasks":false,"others_can_mark_tasks_as_done":false},` +
		`"contact":{"phone_number":"+10000000000","first_name":"Con"},` +
		`"dice":{"emoji":"🎲","value":5},` +
		`"game":{"title":"Gm","description":"d","photo":[]},` +
		`"giveaway":` + b48aGiveaway + `,"giveaway_winners":` + b48aWinners + `,"invoice":` + b48aInvoice + `,` +
		`"location":{"longitude":13.4,"latitude":52.5},` +
		`"poll":{"id":"po","question":"q","options":[],"total_voter_count":0,"is_closed":false,"is_anonymous":true,"type":"regular","allows_multiple_answers":false},` +
		`"venue":{"location":{"longitude":1,"latitude":2},"title":"Ve","address":"a"}}`
	b48aMsgBase    = `{"message_id":1001,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}`
	b48aMsgForward = b48aMsgBase + `,"forward_origin":` + b48aOriginUser + `}`
	b48aMsgQuote   = b48aMsgBase + `,"reply_to_message":{"message_id":1000,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"},"text":"the source text"},"quote":` + b48aQuote + `}`
	// The server still sends the old forward keys beside forward_origin (P1-02); only keys galigo's Message has are
	// used (the server also sends forward_from_message_id and forward_signature for a channel, which Message lacks).
	b48aMsgOldUser    = b48aMsgBase + `,"forward_from":{"id":5000000001,"is_bot":true,"first_name":"Bot"},"forward_date":4102444800,"forward_origin":` + b48aOriginUser + `}`
	b48aMsgOldChannel = b48aMsgBase + `,"forward_from_chat":{"id":-1009876543210,"type":"channel","title":"C"},"forward_date":4102444803,"forward_origin":` + b48aOriginChannel + `}`
	b48aMsgExternal   = b48aMsgBase + `,"external_reply":{"origin":` + b48aOriginChannel + `,"chat":{"id":-1009876543210,"type":"channel","title":"C"},"message_id":77,"photo":[{"file_id":"ph","file_unique_id":"phu","width":2,"height":2}]}}`
)

// T-ori-1..5: each MessageOrigin variant decodes strictly, fills only its own fields and re-encodes to the input;
// a future variant decodes with no error.
func TestB48A_Origin(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(b48aOriginUser), &o))
		assert.Equal(t, "user", o.Type)
		assert.Equal(t, int64(4102444800), o.Date)
		require.NotNil(t, o.SenderUser)
		assert.Equal(t, int64(5000000001), o.SenderUser.ID)
		assert.True(t, o.SenderUser.IsBot)
		assert.Equal(t, "", o.SenderUserName)
		assert.Nil(t, o.SenderChat)
		assert.Nil(t, o.Chat)
		assert.Equal(t, 0, o.MessageID)
		assert.Equal(t, "", o.AuthorSignature)
		assert.JSONEq(t, b48aOriginUser, api102aMarshal(t, o))
	})
	t.Run("hidden_user", func(t *testing.T) {
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(b48aOriginHidden), &o))
		assert.Equal(t, "hidden_user", o.Type)
		assert.Equal(t, int64(4102444801), o.Date)
		assert.Equal(t, "Hidden Name", o.SenderUserName)
		assert.Nil(t, o.SenderUser)
		assert.Nil(t, o.SenderChat)
		assert.Nil(t, o.Chat)
		assert.JSONEq(t, b48aOriginHidden, api102aMarshal(t, o))
	})
	t.Run("chat", func(t *testing.T) {
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(b48aOriginChat), &o))
		assert.Equal(t, "chat", o.Type)
		assert.Equal(t, int64(4102444802), o.Date)
		require.NotNil(t, o.SenderChat)
		assert.Equal(t, int64(-1001234567890), o.SenderChat.ID)
		assert.Equal(t, "Admin", o.AuthorSignature)
		assert.Nil(t, o.Chat)
		assert.Nil(t, o.SenderUser)
		assert.JSONEq(t, b48aOriginChat, api102aMarshal(t, o))
	})
	t.Run("channel", func(t *testing.T) {
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(b48aOriginChannel), &o))
		assert.Equal(t, "channel", o.Type)
		assert.Equal(t, int64(4102444803), o.Date)
		require.NotNil(t, o.Chat)
		assert.Equal(t, int64(-1009876543210), o.Chat.ID)
		assert.Equal(t, 77, o.MessageID)
		assert.Equal(t, "Editor", o.AuthorSignature)
		assert.Nil(t, o.SenderChat)
		assert.Nil(t, o.SenderUser)
		assert.JSONEq(t, b48aOriginChannel, api102aMarshal(t, o))
	})
	t.Run("future", func(t *testing.T) {
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(`{"type":"future","date":1}`), &o))
		assert.Equal(t, "future", o.Type)
	})
	t.Run("all_keys", func(t *testing.T) { // T-ori-6: the flat struct keeps every variant key at once
		in := `{"type":"x","date":4102444804,"sender_user":{"id":5000000002,"is_bot":false,"first_name":"U"},` +
			`"sender_user_name":"N","sender_chat":{"id":-1001111111112,"type":"group"},"author_signature":"S",` +
			`"chat":{"id":-1002222222223,"type":"channel"},"message_id":78}`
		var o tg.MessageOrigin
		require.NoError(t, api96Strict(t, []byte(in), &o))
		require.NotNil(t, o.SenderUser)
		require.NotNil(t, o.SenderChat)
		require.NotNil(t, o.Chat)
		assert.Equal(t, int64(5000000002), o.SenderUser.ID)
		assert.Equal(t, "N", o.SenderUserName)
		assert.Equal(t, int64(-1001111111112), o.SenderChat.ID)
		assert.Equal(t, "S", o.AuthorSignature)
		assert.Equal(t, int64(-1002222222223), o.Chat.ID)
		assert.Equal(t, 78, o.MessageID)
		assert.JSONEq(t, in, api102aMarshal(t, o))
	})
}

// T-quo-1: TextQuote decodes and re-encodes; an empty text and a zero position stay on encode (required keys).
func TestB48A_TextQuote(t *testing.T) {
	var q tg.TextQuote
	require.NoError(t, api96Strict(t, []byte(b48aQuote), &q))
	assert.Equal(t, "source", q.Text)
	assert.Equal(t, 12, q.Position)
	assert.True(t, q.IsManual)
	require.Len(t, q.Entities, 1)
	assert.Equal(t, "bold", q.Entities[0].Type)
	assert.Equal(t, 6, q.Entities[0].Length)
	assert.JSONEq(t, b48aQuote, api102aMarshal(t, q))

	var e tg.TextQuote
	require.NoError(t, api96Strict(t, []byte(`{"text":"","position":0}`), &e))
	assert.JSONEq(t, `{"text":"","position":0}`, api102aMarshal(t, e))
}

// T-inv-1, T-giv-1, T-win-1: every key of Invoice, Giveaway and GiveawayWinners decodes and re-encodes.
func TestB48A_InvoiceGiveaway(t *testing.T) {
	var inv tg.Invoice
	require.NoError(t, api96Strict(t, []byte(b48aInvoice), &inv))
	assert.Equal(t, "Ticket", inv.Title)
	assert.Equal(t, "One ticket", inv.Description)
	assert.Equal(t, "tk", inv.StartParameter)
	assert.Equal(t, "XTR", inv.Currency)
	assert.Equal(t, 145, inv.TotalAmount)
	assert.JSONEq(t, b48aInvoice, api102aMarshal(t, inv))

	var g tg.Giveaway
	require.NoError(t, api96Strict(t, []byte(b48aGiveaway), &g))
	require.Len(t, g.Chats, 1)
	assert.Equal(t, int64(-1001111111111), g.Chats[0].ID)
	assert.Equal(t, int64(4102444900), g.WinnersSelectionDate)
	assert.Equal(t, 3, g.WinnerCount)
	assert.True(t, g.OnlyNewMembers)
	assert.True(t, g.HasPublicWinners)
	assert.Equal(t, "Prize", g.PrizeDescription)
	assert.Equal(t, []string{"DE", "PL"}, g.CountryCodes)
	assert.Equal(t, 500, g.PrizeStarCount)
	assert.Equal(t, 6, g.PremiumSubscriptionMonthCount)
	assert.JSONEq(t, b48aGiveaway, api102aMarshal(t, g))

	var w tg.GiveawayWinners
	require.NoError(t, api96Strict(t, []byte(b48aWinners), &w))
	assert.Equal(t, int64(-1002222222222), w.Chat.ID)
	assert.Equal(t, 42, w.GiveawayMessageID)
	assert.Equal(t, int64(4102444901), w.WinnersSelectionDate)
	assert.Equal(t, 2, w.WinnerCount)
	require.Len(t, w.Winners, 2)
	assert.Equal(t, int64(222), w.Winners[1].ID)
	assert.Equal(t, 1, w.AdditionalChatCount)
	assert.Equal(t, 50, w.PrizeStarCount)
	assert.Equal(t, 3, w.PremiumSubscriptionMonthCount)
	assert.Equal(t, 1, w.UnclaimedPrizeCount)
	assert.True(t, w.OnlyNewMembers)
	assert.True(t, w.WasRefunded)
	assert.Equal(t, "Extra", w.PrizeDescription)
	assert.JSONEq(t, b48aWinners, api102aMarshal(t, w))
}

// T-eri-1: every in-scope ExternalReplyInfo key decodes strictly into its field, and every key is written back.
func TestB48A_ExternalFull(t *testing.T) {
	var e tg.ExternalReplyInfo
	require.NoError(t, api96Strict(t, []byte(b48aExternalFull), &e))
	assert.Equal(t, "channel", e.Origin.Type)
	assert.Equal(t, int64(4102444803), e.Origin.Date)
	require.NotNil(t, e.Chat)
	assert.Equal(t, int64(-1009876543210), e.Chat.ID)
	assert.Equal(t, 77, e.MessageID)
	require.NotNil(t, e.LinkPreviewOptions)
	assert.True(t, e.LinkPreviewOptions.IsDisabled)
	require.NotNil(t, e.Animation)
	assert.Equal(t, "an", e.Animation.FileID)
	require.NotNil(t, e.Audio)
	assert.Equal(t, "au", e.Audio.FileID)
	require.NotNil(t, e.Document)
	assert.Equal(t, "do", e.Document.FileID)
	require.NotNil(t, e.LivePhoto)
	assert.Equal(t, "lv", e.LivePhoto.FileID)
	require.Len(t, e.LivePhoto.Photo, 1)
	assert.Equal(t, "lp", e.LivePhoto.Photo[0].FileID)
	require.Len(t, e.Photo, 1)
	assert.Equal(t, "ph", e.Photo[0].FileID)
	require.NotNil(t, e.Sticker)
	assert.Equal(t, "st", e.Sticker.FileID)
	require.NotNil(t, e.Story)
	assert.Equal(t, 9, e.Story.ID)
	require.NotNil(t, e.Video)
	assert.Equal(t, "vi", e.Video.FileID)
	require.NotNil(t, e.VideoNote)
	assert.Equal(t, "vn", e.VideoNote.FileID)
	require.NotNil(t, e.Voice)
	assert.Equal(t, "vo", e.Voice.FileID)
	assert.True(t, e.HasMediaSpoiler)
	require.NotNil(t, e.Checklist)
	assert.Equal(t, "Todo", e.Checklist.Title)
	require.NotNil(t, e.Contact)
	assert.Equal(t, "Con", e.Contact.FirstName)
	require.NotNil(t, e.Dice)
	assert.Equal(t, 5, e.Dice.Value)
	require.NotNil(t, e.Game)
	assert.Equal(t, "Gm", e.Game.Title)
	require.NotNil(t, e.Giveaway)
	assert.Equal(t, 3, e.Giveaway.WinnerCount)
	require.NotNil(t, e.GiveawayWinners)
	assert.Equal(t, 42, e.GiveawayWinners.GiveawayMessageID)
	require.NotNil(t, e.Invoice)
	assert.Equal(t, "Ticket", e.Invoice.Title)
	require.NotNil(t, e.Location)
	assert.InDelta(t, 52.5, e.Location.Latitude, 1e-9)
	require.NotNil(t, e.Poll)
	assert.Equal(t, "po", e.Poll.ID)
	require.NotNil(t, e.Venue)
	assert.Equal(t, "Ve", e.Venue.Title)

	keys := api102aKeys(t, e)
	got := make([]string, 0, len(keys))
	for k := range keys {
		got = append(got, k)
	}
	sort.Strings(got)
	want := append([]string(nil), b48aSpecKeys["ExternalReplyInfo"]...)
	sort.Strings(want)
	assert.Equal(t, want, got, "every in-scope key is written back")
	assert.JSONEq(t, b48aOriginChannel, string(keys["origin"]))
}

// T-eri-2: the left-out keys (paid_media: B-48b; widget, rich_texts: 10.4) decode leniently and have no field.
func TestB48A_LeftOut(t *testing.T) {
	for _, tc := range []struct {
		name, in, key string
		v             func() any
	}{
		{"paid_media", `{"origin":{"type":"user","date":1},"paid_media":{"star_count":1,"paid_media":[]}}`, "paid_media", func() any { return &tg.ExternalReplyInfo{} }},
		{"widget", `{"origin":{"type":"user","date":1},"widget":{"id":"w"}}`, "widget", func() any { return &tg.ExternalReplyInfo{} }},
		{"rich_texts", `{"text":"a","position":0,"rich_texts":["a"]}`, "rich_texts", func() any { return &tg.TextQuote{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, json.Unmarshal([]byte(tc.in), tc.v()))
			err := api96Strict(t, []byte(tc.in), tc.v())
			require.Error(t, err)
			assert.Contains(t, err.Error(), `unknown field "`+tc.key+`"`)
		})
	}
}

// T-msg-1..5: the three Message holders decode from server-shaped messages; the other holders and the old forward
// fields stay empty; the message re-encodes to the input.
func TestB48A_MessageFields(t *testing.T) {
	t.Run("forward_origin", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48aMsgForward), &m))
		require.NotNil(t, m.ForwardOrigin)
		assert.Equal(t, "user", m.ForwardOrigin.Type)
		require.NotNil(t, m.ForwardOrigin.SenderUser)
		assert.Equal(t, int64(5000000001), m.ForwardOrigin.SenderUser.ID)
		assert.Nil(t, m.ExternalReply)
		assert.Nil(t, m.Quote)
		assert.Nil(t, m.ForwardFrom)
		assert.Nil(t, m.ForwardFromChat)
		assert.Equal(t, int64(0), m.ForwardDate)
		assert.JSONEq(t, b48aMsgForward, api102aMarshal(t, m))
	})
	t.Run("quote", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48aMsgQuote), &m))
		require.NotNil(t, m.Quote)
		assert.Equal(t, "source", m.Quote.Text)
		assert.Equal(t, 12, m.Quote.Position)
		require.NotNil(t, m.ReplyToMessage)
		assert.Equal(t, 1000, m.ReplyToMessage.MessageID)
		assert.Nil(t, m.ForwardOrigin)
		assert.Nil(t, m.ExternalReply)
		assert.JSONEq(t, b48aMsgQuote, api102aMarshal(t, m))
	})
	t.Run("old_forward_user", func(t *testing.T) { // T-msg-4: the old keys still decode beside forward_origin (AC2)
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48aMsgOldUser), &m))
		require.NotNil(t, m.ForwardFrom)
		assert.Equal(t, int64(5000000001), m.ForwardFrom.ID)
		assert.Nil(t, m.ForwardFromChat)
		assert.Equal(t, int64(4102444800), m.ForwardDate)
		require.NotNil(t, m.ForwardOrigin)
		assert.Equal(t, "user", m.ForwardOrigin.Type)
		assert.JSONEq(t, b48aMsgOldUser, api102aMarshal(t, m))
	})
	t.Run("old_forward_channel", func(t *testing.T) { // T-msg-5
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48aMsgOldChannel), &m))
		require.NotNil(t, m.ForwardFromChat)
		assert.Equal(t, int64(-1009876543210), m.ForwardFromChat.ID)
		assert.Equal(t, "channel", m.ForwardFromChat.Type)
		assert.Nil(t, m.ForwardFrom)
		assert.Equal(t, int64(4102444803), m.ForwardDate)
		require.NotNil(t, m.ForwardOrigin)
		require.NotNil(t, m.ForwardOrigin.Chat)
		assert.Equal(t, int64(-1009876543210), m.ForwardOrigin.Chat.ID)
		assert.JSONEq(t, b48aMsgOldChannel, api102aMarshal(t, m))
	})
	t.Run("external_reply", func(t *testing.T) {
		var m tg.Message
		require.NoError(t, api96Strict(t, []byte(b48aMsgExternal), &m))
		require.NotNil(t, m.ExternalReply)
		assert.Equal(t, "channel", m.ExternalReply.Origin.Type)
		require.NotNil(t, m.ExternalReply.Chat)
		assert.Equal(t, int64(-1009876543210), m.ExternalReply.Chat.ID)
		assert.Equal(t, 77, m.ExternalReply.MessageID)
		require.Len(t, m.ExternalReply.Photo, 1)
		assert.Nil(t, m.ForwardOrigin)
		assert.Nil(t, m.Quote)
		assert.Nil(t, m.ReplyToMessage)
		assert.JSONEq(t, b48aMsgExternal, api102aMarshal(t, m))
	})
}

// T-zero: a zero value of each new type encodes exactly its required keys; Message{} has none of the new keys.
func TestB48A_ZeroEncode(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{tg.MessageOrigin{}, `{"type":"","date":0}`},
		{tg.TextQuote{}, `{"text":"","position":0}`},
		{tg.ExternalReplyInfo{}, `{"origin":{"type":"","date":0}}`},
		{tg.Giveaway{}, `{"chats":null,"winners_selection_date":0,"winner_count":0}`},
		{tg.GiveawayWinners{}, `{"chat":{"id":0,"type":""},"giveaway_message_id":0,"winners_selection_date":0,"winner_count":0,"winners":null}`},
		{tg.Invoice{}, `{"title":"","description":"","start_parameter":"","currency":"","total_amount":0}`},
	} {
		assert.Equal(t, tc.want, api102aMarshal(t, tc.v), "%T", tc.v)
	}
	keys := api102aKeys(t, tg.Message{})
	for _, k := range []string{"forward_origin", "external_reply", "quote"} {
		assert.NotContains(t, keys, k)
	}
}

// The spec key lists, in spec order (E20/lookups/sec-b48a.txt), without the left-out keys.
var b48aSpecKeys = map[string][]string{
	"MessageOrigin": {"type", "date", "sender_user", "sender_user_name", "sender_chat", "author_signature", "chat", "message_id"},
	"TextQuote":     {"text", "entities", "position", "is_manual"},
	"ExternalReplyInfo": {"origin", "chat", "message_id", "link_preview_options", "animation", "audio", "document", "live_photo",
		"photo", "sticker", "story", "video", "video_note", "voice", "has_media_spoiler", "checklist", "contact", "dice", "game",
		"giveaway", "giveaway_winners", "invoice", "location", "poll", "venue"},
	"Giveaway": {"chats", "winners_selection_date", "winner_count", "only_new_members", "has_public_winners", "prize_description",
		"country_codes", "prize_star_count", "premium_subscription_month_count"},
	"GiveawayWinners": {"chat", "giveaway_message_id", "winners_selection_date", "winner_count", "winners", "additional_chat_count",
		"prize_star_count", "premium_subscription_month_count", "unclaimed_prize_count", "only_new_members", "was_refunded",
		"prize_description"},
	"Invoice": {"title", "description", "start_parameter", "currency", "total_amount"},
}

// T-shape: each new type's fields, in order, equal the planned rows exactly (name, Go type, JSON tag, not embedded);
// the tag keys equal the spec key list; no methods; the three Message holders have the planned shape.
func TestB48A_Shapes(t *testing.T) {
	type row struct{ name, typ, tag string }
	shapes := []struct {
		v    any
		rows []row
	}{
		{tg.MessageOrigin{}, []row{
			{"Type", "string", "type"}, {"Date", "int64", "date"},
			{"SenderUser", "*tg.User", "sender_user,omitempty"}, {"SenderUserName", "string", "sender_user_name,omitempty"},
			{"SenderChat", "*tg.Chat", "sender_chat,omitempty"}, {"AuthorSignature", "string", "author_signature,omitempty"},
			{"Chat", "*tg.Chat", "chat,omitempty"}, {"MessageID", "int", "message_id,omitempty"},
		}},
		{tg.TextQuote{}, []row{
			{"Text", "string", "text"}, {"Entities", "[]tg.MessageEntity", "entities,omitempty"},
			{"Position", "int", "position"}, {"IsManual", "bool", "is_manual,omitempty"},
		}},
		{tg.ExternalReplyInfo{}, []row{
			{"Origin", "tg.MessageOrigin", "origin"}, {"Chat", "*tg.Chat", "chat,omitempty"}, {"MessageID", "int", "message_id,omitempty"},
			{"LinkPreviewOptions", "*tg.LinkPreviewOptions", "link_preview_options,omitempty"},
			{"Animation", "*tg.Animation", "animation,omitempty"}, {"Audio", "*tg.Audio", "audio,omitempty"},
			{"Document", "*tg.Document", "document,omitempty"}, {"LivePhoto", "*tg.LivePhoto", "live_photo,omitempty"},
			{"Photo", "[]tg.PhotoSize", "photo,omitempty"}, {"Sticker", "*tg.Sticker", "sticker,omitempty"},
			{"Story", "*tg.Story", "story,omitempty"}, {"Video", "*tg.Video", "video,omitempty"},
			{"VideoNote", "*tg.VideoNote", "video_note,omitempty"}, {"Voice", "*tg.Voice", "voice,omitempty"},
			{"HasMediaSpoiler", "bool", "has_media_spoiler,omitempty"}, {"Checklist", "*tg.Checklist", "checklist,omitempty"},
			{"Contact", "*tg.Contact", "contact,omitempty"}, {"Dice", "*tg.Dice", "dice,omitempty"}, {"Game", "*tg.Game", "game,omitempty"},
			{"Giveaway", "*tg.Giveaway", "giveaway,omitempty"}, {"GiveawayWinners", "*tg.GiveawayWinners", "giveaway_winners,omitempty"},
			{"Invoice", "*tg.Invoice", "invoice,omitempty"}, {"Location", "*tg.Location", "location,omitempty"},
			{"Poll", "*tg.Poll", "poll,omitempty"}, {"Venue", "*tg.Venue", "venue,omitempty"},
		}},
		{tg.Giveaway{}, []row{
			{"Chats", "[]tg.Chat", "chats"}, {"WinnersSelectionDate", "int64", "winners_selection_date"}, {"WinnerCount", "int", "winner_count"},
			{"OnlyNewMembers", "bool", "only_new_members,omitempty"}, {"HasPublicWinners", "bool", "has_public_winners,omitempty"},
			{"PrizeDescription", "string", "prize_description,omitempty"}, {"CountryCodes", "[]string", "country_codes,omitempty"},
			{"PrizeStarCount", "int", "prize_star_count,omitempty"},
			{"PremiumSubscriptionMonthCount", "int", "premium_subscription_month_count,omitempty"},
		}},
		{tg.GiveawayWinners{}, []row{
			{"Chat", "tg.Chat", "chat"}, {"GiveawayMessageID", "int", "giveaway_message_id"},
			{"WinnersSelectionDate", "int64", "winners_selection_date"}, {"WinnerCount", "int", "winner_count"},
			{"Winners", "[]tg.User", "winners"}, {"AdditionalChatCount", "int", "additional_chat_count,omitempty"},
			{"PrizeStarCount", "int", "prize_star_count,omitempty"},
			{"PremiumSubscriptionMonthCount", "int", "premium_subscription_month_count,omitempty"},
			{"UnclaimedPrizeCount", "int", "unclaimed_prize_count,omitempty"}, {"OnlyNewMembers", "bool", "only_new_members,omitempty"},
			{"WasRefunded", "bool", "was_refunded,omitempty"}, {"PrizeDescription", "string", "prize_description,omitempty"},
		}},
		{tg.Invoice{}, []row{
			{"Title", "string", "title"}, {"Description", "string", "description"}, {"StartParameter", "string", "start_parameter"},
			{"Currency", "string", "currency"}, {"TotalAmount", "int", "total_amount"},
		}},
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
			assert.Equal(t, b48aSpecKeys[rt.Name()], keys, "the spec keys in spec order")
			assert.Equal(t, 0, rt.NumMethod(), "no value methods")
			assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod(), "no pointer methods")
		})
	}
	// The three holders on Message: their own subtest, so a mutant that breaks only them is seen on this row (T2-04).
	t.Run("Message", func(t *testing.T) {
		mt := reflect.TypeOf(tg.Message{})
		for _, h := range []struct {
			row
			after string // PLAN D5: the field the holder follows
		}{
			{row{"ForwardOrigin", "*tg.MessageOrigin", "forward_origin,omitempty"}, "ForwardDate"},
			{row{"ExternalReply", "*tg.ExternalReplyInfo", "external_reply,omitempty"}, "ReplyToMessage"},
			{row{"Quote", "*tg.TextQuote", "quote,omitempty"}, "ExternalReply"},
		} {
			f, ok := mt.FieldByName(h.name)
			p, okp := mt.FieldByName(h.after)
			if assert.True(t, ok, "Message.%s", h.name) && assert.True(t, okp, "Message.%s", h.after) {
				assert.False(t, f.Anonymous, "Message.%s must be a named field", h.name)
				assert.Equal(t, h.row, row{f.Name, f.Type.String(), f.Tag.Get("json")})
				assert.Equal(t, []int{p.Index[0] + 1}, f.Index, "Message.%s must follow %s", h.name, h.after)
			}
		}
	})
}

// T-own: walk every struct type reachable from tg.Update and tg.ReplyParameters (generated, not a list) and check
// that the new keys live only on their planned holders. The walk stops at interface fields.
func TestB48A_KeyOwners(t *testing.T) {
	watched := map[string]bool{"forward_origin": true, "external_reply": true, "quote": true, "origin": true,
		"sender_user_name": true, "is_manual": true, "giveaway_winners": true, "winners_selection_date": true}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{}), reflect.TypeOf(tg.ReplyParameters{})}
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
		"ExternalReplyInfo.GiveawayWinners", "ExternalReplyInfo.Origin", "Giveaway.WinnersSelectionDate",
		"GiveawayWinners.WinnersSelectionDate", "Message.ExternalReply", "Message.ForwardOrigin", "Message.Quote",
		"MessageOrigin.SenderUserName", "ReplyParameters.Quote", "TextQuote.IsManual",
	}, owners)
	for _, v := range []any{tg.Message{}, tg.MessageOrigin{}, tg.TextQuote{}, tg.ExternalReplyInfo{}, tg.Giveaway{},
		tg.GiveawayWinners{}, tg.Invoice{}, tg.ReplyParameters{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
