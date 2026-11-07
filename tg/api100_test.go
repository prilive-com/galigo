package tg_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.0 part a decode and encode tests (B-25 slice 2a, TEST-PLAN §2, rows T-tg-1 … T-tg-11).
//
// Live fixtures: replies of 2026-11-04 (E8/lookups/probe100a/01 getMe, 04 getChat). The only edit is the identity
// fields, replaced by internal/testutil values because the repository is public: bot id -> testutil.TestBotID, bot
// first_name -> "Test Bot", bot username -> testutil.TestBotUsername. Only probe 04's "permissions" object is
// embedded (no chat id, title, description or invite link). Every 10.0 field is byte-exact. Fixtures named
// "synthetic" are built from the API reference tables (E8/lookups/sec-10.0.txt); there is no live sample for them.

const (
	// probe 01: getMe, identity fields replaced.
	api100Probe01 = `{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot",` +
		`"can_join_groups":true,"can_read_all_group_messages":false,"supports_inline_queries":false,` +
		`"supports_guest_queries":false,"can_connect_to_business":false,"has_main_web_app":false,` +
		`"has_topics_enabled":false,"allows_users_to_create_topics":false,"can_manage_bots":false,` +
		`"supports_join_request_queries":false}`

	// probe 04: getChat result.permissions only (it holds no identity value).
	api100Perms = `{"can_send_messages":true,"can_send_media_messages":true,"can_send_audios":true,` +
		`"can_send_documents":true,"can_send_photos":true,"can_send_videos":true,"can_send_video_notes":true,` +
		`"can_send_voice_notes":true,"can_send_polls":true,"can_send_other_messages":true,` +
		`"can_add_web_page_previews":true,"can_react_to_messages":true,"can_edit_tag":false,` +
		`"can_change_info":true,"can_invite_users":true,"can_pin_messages":true,"can_manage_topics":false}`

	// synthetic: user A (the guest caller), chat B (the caller's chat), and the guest message's own chat C; three
	// distinct ids (E-0013).
	api100UserA = `{"id":987654321,"is_bot":false,"first_name":"Test","username":"testuser"}`
	api100ChatB = `{"id":123456789,"type":"supergroup","title":"Test Chat"}`
	api100ChatC = `{"id":555000222,"type":"private","first_name":"Guest"}`

	// synthetic: a message with the three guest fields.
	api100GuestMsg = `{"message_id":77,"date":1793799008,"chat":` + api100ChatC + `,"text":"hello guest",` +
		`"guest_bot_caller_user":` + api100UserA + `,"guest_bot_caller_chat":` + api100ChatB + `,` +
		`"guest_query_id":"gq-100"}`

	// synthetic: an update with guest_message.
	api100GuestUpdate = `{"update_id":9,"guest_message":` + api100GuestMsg + `}`

	// synthetic: a restricted member; every permission false except can_send_messages, can_react_to_messages true.
	api100Restricted = `{"status":"restricted","user":` + api100UserA + `,"is_member":true,` +
		`"can_send_messages":true,"can_send_audios":false,"can_send_documents":false,"can_send_photos":false,` +
		`"can_send_videos":false,"can_send_video_notes":false,"can_send_voice_notes":false,"can_send_polls":false,` +
		`"can_send_other_messages":false,"can_add_web_page_previews":false,"can_change_info":false,` +
		`"can_invite_users":false,"can_pin_messages":false,"can_manage_topics":false,"until_date":0,` +
		`"can_edit_tag":false,"can_react_to_messages":true}`

	// synthetic: users U1 and U2 (distinct ids, one a bot) and the access settings that list them.
	api100U1     = `{"id":987654321,"is_bot":false,"first_name":"Test","username":"testuser"}`
	api100U2     = `{"id":555000111,"is_bot":true,"first_name":"Managed","username":"managed_test_bot"}`
	api100Access = `{"is_access_restricted":true,"added_users":[` + api100U1 + `,` + api100U2 + `]}`
)

func api100Bool(v bool) *bool { return &v }

func api100WantUserA() *tg.User {
	return &tg.User{ID: testutil.TestUserID, FirstName: "Test", Username: testutil.TestUsername}
}

func api100WantGuestMsg() tg.Message {
	return tg.Message{
		MessageID:          77,
		Date:               1793799008,
		Chat:               &tg.Chat{ID: 555000222, Type: "private", FirstName: "Guest"},
		Text:               "hello guest",
		GuestBotCallerUser: api100WantUserA(),
		GuestBotCallerChat: &tg.Chat{ID: testutil.TestChatID, Type: "supergroup", Title: "Test Chat"},
		GuestQueryID:       "gq-100",
	}
}

func api100WantRestricted(react bool) tg.ChatMemberRestricted {
	m := tg.ChatMemberRestricted{IsMember: true, CanSendMessages: true, CanReactToMessages: react}
	m.User = api100WantUserA()
	return m
}

func api100WantAccess() tg.BotAccessSettings {
	return tg.BotAccessSettings{IsAccessRestricted: true, AddedUsers: []tg.User{
		{ID: testutil.TestUserID, FirstName: "Test", Username: testutil.TestUsername},
		{ID: 555000111, IsBot: true, FirstName: "Managed", Username: "managed_test_bot"},
	}}
}

// api100Keys returns the JSON object b with only the named keys (each must be present), for a JSONEq on a subset.
func api100Keys(t *testing.T, b []byte, keys ...string) string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &m))
	out := map[string]json.RawMessage{}
	for _, k := range keys {
		v, ok := m[k]
		require.True(t, ok, "key %q is not in %s", k, b)
		out[k] = v
	}
	s, err := json.Marshal(out)
	require.NoError(t, err)
	return string(s)
}

func TestAPI100_UserSupportsGuestQueries(t *testing.T) {
	want := tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot", Username: testutil.TestBotUsername,
		CanJoinGroups: true}

	// API100 T-tg-1: the probe's false (a control) and a synthetic true
	var got tg.User
	require.NoError(t, json.Unmarshal([]byte(api100Probe01), &got))
	assert.Equal(t, want, got)

	trueVariant := strings.Replace(api100Probe01, `"supports_guest_queries":false`, `"supports_guest_queries":true`, 1)
	require.NotEqual(t, api100Probe01, trueVariant)
	got = tg.User{}
	require.NoError(t, json.Unmarshal([]byte(trueVariant), &got))
	want.SupportsGuestQueries = true
	assert.Equal(t, want, got)
}

func TestAPI100_MessageGuestFields(t *testing.T) {
	// API100 T-tg-2
	var got tg.Message
	require.NoError(t, json.Unmarshal([]byte(api100GuestMsg), &got))
	assert.Equal(t, api100WantGuestMsg(), got)
}

func TestAPI100_UpdateGuestMessage(t *testing.T) {
	// API100 T-tg-3
	var got tg.Update
	require.NoError(t, json.Unmarshal([]byte(api100GuestUpdate), &got))
	msg := api100WantGuestMsg()
	assert.Equal(t, tg.Update{UpdateID: 9, GuestMessage: &msg}, got)
}

func TestAPI100_ChatPermissionsReact(t *testing.T) {
	want := tg.ChatPermissions{
		CanSendMessages: api100Bool(true), CanSendAudios: api100Bool(true), CanSendDocuments: api100Bool(true),
		CanSendPhotos: api100Bool(true), CanSendVideos: api100Bool(true), CanSendVideoNotes: api100Bool(true),
		CanSendVoiceNotes: api100Bool(true), CanSendPolls: api100Bool(true), CanSendOtherMessages: api100Bool(true),
		CanAddWebPagePreviews: api100Bool(true), CanChangeInfo: api100Bool(true), CanInviteUsers: api100Bool(true),
		CanPinMessages: api100Bool(true), CanManageTopics: api100Bool(false), CanEditTag: api100Bool(false),
		CanReactToMessages: api100Bool(true),
	}

	// API100 T-tg-4: probe 04 (legacy can_send_media_messages deleted) and a synthetic false copy
	var got tg.ChatPermissions
	require.NoError(t, json.Unmarshal(api96Sub(t, api100Perms, nil, "can_send_media_messages"), &got))
	assert.Equal(t, want, got)

	falseVariant := strings.Replace(api100Perms, `"can_react_to_messages":true`, `"can_react_to_messages":false`, 1)
	require.NotEqual(t, api100Perms, falseVariant)
	got = tg.ChatPermissions{}
	require.NoError(t, json.Unmarshal(api96Sub(t, falseVariant, nil, "can_send_media_messages"), &got))
	want.CanReactToMessages = api100Bool(false)
	assert.Equal(t, want, got)
}

func TestAPI100_ChatMemberRestrictedReact(t *testing.T) {
	// API100 T-tg-5: through the union decoder, true and false
	for _, react := range []bool{true, false} {
		raw := api100Restricted
		if !react {
			raw = strings.Replace(raw, `"can_react_to_messages":true`, `"can_react_to_messages":false`, 1)
			require.NotEqual(t, api100Restricted, raw)
		}
		got, err := tg.UnmarshalChatMember([]byte(raw))
		require.NoError(t, err)
		assert.Equal(t, tg.ChatMember(api100WantRestricted(react)), got, "can_react_to_messages %v", react)
	}
}

func TestAPI100_EncodeReact(t *testing.T) {
	// API100 T-tg-6: the member writes can_react_to_messages:false and no status key
	b, err := json.Marshal(api100WantRestricted(false))
	require.NoError(t, err)
	assert.JSONEq(t, `{"user":`+api100UserA+`,"is_member":true,"can_send_messages":true,"can_send_audios":false,`+
		`"can_send_documents":false,"can_send_photos":false,"can_send_videos":false,"can_send_video_notes":false,`+
		`"can_send_voice_notes":false,"can_send_polls":false,"can_send_other_messages":false,`+
		`"can_add_web_page_previews":false,"can_change_info":false,"can_invite_users":false,"can_pin_messages":false,`+
		`"can_manage_topics":false,"until_date":0,"can_edit_tag":false,"can_react_to_messages":false}`, string(b))

	b, err = json.Marshal(tg.ChatPermissions{CanReactToMessages: api100Bool(false)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"can_react_to_messages":false}`, string(b))

	b, err = json.Marshal(tg.ChatPermissions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(b))
}

func TestAPI100_PermissionHelpersUnchanged(t *testing.T) {
	// API100 T-tg-7: the four helpers leave CanReactToMessages nil, also after CanSendMessages is flipped
	helpers := map[string]func() tg.ChatPermissions{
		"AllPermissions": tg.AllPermissions, "NoPermissions": tg.NoPermissions,
		"ReadOnlyPermissions": tg.ReadOnlyPermissions, "TextOnlyPermissions": tg.TextOnlyPermissions,
	}
	for name, h := range helpers {
		p := h()
		assert.Nil(t, p.CanReactToMessages, name)
		flipped := h()
		send := flipped.CanSendMessages != nil && *flipped.CanSendMessages
		flipped.CanSendMessages = api100Bool(!send)
		for _, v := range []tg.ChatPermissions{p, flipped} {
			b, err := json.Marshal(v)
			require.NoError(t, err)
			assert.NotContains(t, string(b), "can_react_to_messages", name)
		}
	}
}

func TestAPI100_SentGuestMessage(t *testing.T) {
	// API100 T-tg-8
	var got tg.SentGuestMessage
	require.NoError(t, json.Unmarshal([]byte(`{"inline_message_id":"im-100"}`), &got))
	assert.Equal(t, tg.SentGuestMessage{InlineMessageID: "im-100"}, got)
	b, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"inline_message_id":"im-100"}`, string(b))
}

func TestAPI100_BotAccessSettings(t *testing.T) {
	// API100 T-tg-9: restricted with two users; not restricted (AddedUsers nil, a zero-value control); encode zero
	var got tg.BotAccessSettings
	require.NoError(t, json.Unmarshal([]byte(api100Access), &got))
	assert.Equal(t, api100WantAccess(), got)

	got = tg.BotAccessSettings{}
	require.NoError(t, json.Unmarshal([]byte(`{"is_access_restricted":false}`), &got))
	assert.Equal(t, tg.BotAccessSettings{}, got)

	b, err := json.Marshal(tg.BotAccessSettings{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"is_access_restricted":false}`, string(b))
}

func TestAPI100_StrictDecode(t *testing.T) {
	// API100 T-tg-10: every 10.0a object decodes strictly, so a JSON name without a struct field fails
	// API101B T-tg-2: supports_join_request_queries is modelled since 10.1 part b
	// (tg.User.SupportsJoinRequestQueries), so it is no longer deleted before the strict decode.
	getMeOther := []string{"can_connect_to_business", "has_main_web_app", "has_topics_enabled"}
	rows := []struct {
		name string
		b    []byte
		v    any
	}{
		{"User getMe (probe 01)", api96Sub(t, api100Probe01, nil, getMeOther...), &tg.User{}},
		{"Message guest (synthetic)", api96Sub(t, api100GuestMsg, nil), &tg.Message{}},
		{"Update guest_message (synthetic)", api96Sub(t, api100GuestUpdate, nil), &tg.Update{}},
		{"ChatPermissions (probe 04)", api96Sub(t, api100Perms, nil, "can_send_media_messages"), &tg.ChatPermissions{}},
		{"ChatMemberRestricted (synthetic)", api96Sub(t, api100Restricted, nil, "status"), &tg.ChatMemberRestricted{}},
		{"SentGuestMessage (synthetic)", []byte(`{"inline_message_id":"im-100"}`), &tg.SentGuestMessage{}},
		{"BotAccessSettings (synthetic)", api96Sub(t, api100Access, nil), &tg.BotAccessSettings{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			assert.NoError(t, api96Strict(t, r.b, r.v))
		})
	}
}

func TestAPI100_RoundTrip(t *testing.T) {
	// API100 T-tg-11: the true / non-empty variant of each object encodes its 10.0a keys with the fixture's exact
	// names and values (decode is case-free, so only encoding sees a case-only tag typo)
	trueUser := strings.Replace(api100Probe01, `"supports_guest_queries":false`, `"supports_guest_queries":true`, 1)
	rows := []struct {
		name    string
		fixture string
		v       any
		keys    []string
	}{
		{"User", trueUser, &tg.User{}, []string{"supports_guest_queries"}},
		{"Message", api100GuestMsg, &tg.Message{}, []string{"guest_bot_caller_user", "guest_bot_caller_chat", "guest_query_id"}},
		{"Update", api100GuestUpdate, &tg.Update{}, []string{"guest_message"}},
		{"ChatPermissions", api100Perms, &tg.ChatPermissions{}, []string{"can_react_to_messages"}},
		{"SentGuestMessage", `{"inline_message_id":"im-100"}`, &tg.SentGuestMessage{}, []string{"inline_message_id"}},
		{"BotAccessSettings", api100Access, &tg.BotAccessSettings{}, []string{"is_access_restricted", "added_users"}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			require.NoError(t, json.Unmarshal([]byte(r.fixture), r.v))
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			assert.JSONEq(t, api100Keys(t, []byte(r.fixture), r.keys...), api100Keys(t, b, r.keys...))
		})
	}
	t.Run("ChatMemberRestricted", func(t *testing.T) {
		m, err := tg.UnmarshalChatMember([]byte(api100Restricted))
		require.NoError(t, err)
		b, err := json.Marshal(m)
		require.NoError(t, err)
		assert.JSONEq(t, api100Keys(t, []byte(api100Restricted), "can_react_to_messages"),
			api100Keys(t, b, "can_react_to_messages"))
	})

	// API100 T-tg-11 false/zero-encode rows: the optional keys are absent (green at red, by design: TEST-PLAN §6)
	zero := []struct {
		name   string
		v      any
		absent []string
		exact  string
	}{
		{"User false", tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot"}, []string{"supports_guest_queries"}, ""},
		{"Message minimal", tg.Message{MessageID: 1, Date: 1, Chat: &tg.Chat{ID: testutil.TestChatID, Type: "private"}},
			[]string{"guest_bot_caller_user", "guest_bot_caller_chat", "guest_query_id"}, ""},
		{"Update minimal", tg.Update{UpdateID: 1}, []string{"guest_message"}, ""},
		{"BotAccessSettings no users", tg.BotAccessSettings{IsAccessRestricted: true}, nil, `{"is_access_restricted":true}`},
	}
	for _, r := range zero {
		t.Run(r.name, func(t *testing.T) {
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			for _, k := range r.absent {
				assert.NotContains(t, string(b), `"`+k+`"`)
			}
			if r.exact != "" {
				assert.JSONEq(t, r.exact, string(b))
			}
		})
	}
}

// ================== Bot API 10.0 part b: live photos (B-25 10.0b, TEST-PLAN §2, rows T-tg-1 … T-tg-7) ==================
//
// Live fixtures: replies of 2026-11-05 (E9/lookups/probe100b/01 sendLivePhoto upload, 02 sendLivePhoto by file id,
// 07 editMessageMedia into a live photo). Edits, and only these: "from" -> the testutil bot (id
// testutil.TestBotID, first name "Test Bot", username testutil.TestBotUsername); "chat" -> testutil.TestChatID and
// title "Test Chat"; every file_id / file_unique_id -> a synthetic value (LPV-* the video, LPS90-* and LPS100-* the
// two static sizes). Sizes, dimensions, duration, mime type, captions and dates are byte-exact.

const (
	api100bProbe01 = `{"message_id":1047,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},` +
		`"chat":{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"},"date":1793904606,` +
		`"live_photo":{"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LPV-file","file_unique_id":"LPV-uniq","file_size":663},` +
		`"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"caption":"probe100b upload"}`

	api100bProbe02 = `{"message_id":1048,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},` +
		`"chat":{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"},"date":1793904606,` +
		`"live_photo":{"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LPV-file","file_unique_id":"LPV-uniq","file_size":663},` +
		`"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"caption":"probe100b by id","show_caption_above_media":true,"has_media_spoiler":true}`

	api100bProbe07 = `{"message_id":1055,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},` +
		`"chat":{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"},"date":1793904611,"edit_date":1793904611,` +
		`"live_photo":{"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LPV-file","file_unique_id":"LPV-uniq","file_size":663},` +
		`"photo":[{"file_id":"LPS90-file","file_unique_id":"LPS90-uniq","file_size":341,"width":90,"height":90},` +
		`{"file_id":"LPS100-file","file_unique_id":"LPS100-uniq","file_size":361,"width":100,"height":100}],` +
		`"caption":"edited"}`
)

func api100bSizes() []tg.PhotoSize {
	return []tg.PhotoSize{
		{FileID: "LPS90-file", FileUniqueID: "LPS90-uniq", Width: 90, Height: 90, FileSize: 341},
		{FileID: "LPS100-file", FileUniqueID: "LPS100-uniq", Width: 100, Height: 100, FileSize: 361},
	}
}

func api100bLive() *tg.LivePhoto {
	return &tg.LivePhoto{
		Photo: api100bSizes(), FileID: "LPV-file", FileUniqueID: "LPV-uniq",
		Width: 320, Height: 240, Duration: 1, MimeType: "video/mp4", FileSize: 663,
	}
}

func api100bMessage(id int, date, editDate int64, caption string) *tg.Message {
	return &tg.Message{
		MessageID: id, From: api96Bot(), Chat: api96ChatValue(), Date: date, EditDate: editDate,
		LivePhoto: api100bLive(), Photo: api100bSizes(), Caption: caption,
	}
}

// T-tg-1 … T-tg-5, T-tg-7: live photo objects and messages decode to the whole expected value.
func TestAPI100B_DecodeLivePhoto(t *testing.T) {
	t.Run("T-tg-1 probe 01 live_photo", func(t *testing.T) { // API100B T-tg-1
		raw := api96Sub(t, api100bProbe01, []any{"live_photo"})
		var got tg.LivePhoto
		require.NoError(t, json.Unmarshal(raw, &got))
		assert.Equal(t, *api100bLive(), got)
		var strict tg.LivePhoto
		require.NoError(t, api96Strict(t, raw, &strict))
	})
	t.Run("T-tg-2 spec-shape minimal", func(t *testing.T) { // API100B T-tg-2
		const raw = `{"file_id":"LPV-min","file_unique_id":"LPV-min-uniq","width":1,"height":2,"duration":3}`
		got := api96Decode[tg.LivePhoto](t, raw)
		assert.Equal(t, tg.LivePhoto{FileID: "LPV-min", FileUniqueID: "LPV-min-uniq", Width: 1, Height: 2, Duration: 3}, got)
		b, err := json.Marshal(got)
		require.NoError(t, err)
		assert.JSONEq(t, raw, string(b))
	})
	t.Run("T-tg-2b zero value encode", func(t *testing.T) { // API100B T-tg-2b
		b, err := json.Marshal(tg.LivePhoto{})
		require.NoError(t, err)
		assert.JSONEq(t, `{"file_id":"","file_unique_id":"","width":0,"height":0,"duration":0}`, string(b))
	})
	t.Run("T-tg-3 probe 01 message", func(t *testing.T) { // API100B T-tg-3
		got := api96Decode[tg.Message](t, api100bProbe01)
		assert.Equal(t, *api100bMessage(1047, 1793904606, 0, "probe100b upload"), got)
		var strict tg.Message
		require.NoError(t, api96Strict(t, []byte(api100bProbe01), &strict))
	})
	t.Run("T-tg-4 probe 07 edited message", func(t *testing.T) { // API100B T-tg-4
		got := api96Decode[tg.Message](t, api100bProbe07)
		assert.Equal(t, *api100bMessage(1055, 1793904611, 1793904611, "edited"), got)
		var strict tg.Message
		require.NoError(t, api96Strict(t, []byte(api100bProbe07), &strict))
	})
	t.Run("T-tg-5 64-bit file_size", func(t *testing.T) { // API100B T-tg-5
		got := api96Decode[tg.LivePhoto](t, `{"file_id":"a","file_unique_id":"b","width":1,"height":1,"duration":1,"file_size":4294967296}`)
		want := int64(1)
		want <<= 32 // computed at run time, so an int32 field fails here, not at build time
		assert.Equal(t, want, got.FileSize)
	})
	t.Run("T-tg-7 probe 02 by-id message", func(t *testing.T) { // API100B T-tg-7
		got := api96Decode[tg.Message](t, api100bProbe02)
		assert.Equal(t, *api100bMessage(1048, 1793904606, 0, "probe100b by id"), got)
		// has_media_spoiler and show_caption_above_media are not modelled on Message (B-28); strict decode without them.
		raw := api96Sub(t, api100bProbe02, nil, "has_media_spoiler", "show_caption_above_media")
		var strict tg.Message
		require.NoError(t, api96Strict(t, raw, &strict))
	})
}

// T-tg-6: round trip; a Message without a live photo has no live_photo key.
func TestAPI100B_EncodeLivePhoto(t *testing.T) { // API100B T-tg-6
	b, err := json.Marshal(api100bLive())
	require.NoError(t, err)
	assert.JSONEq(t, string(api96Sub(t, api100bProbe01, []any{"live_photo"})), string(b))
	var back tg.LivePhoto
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, *api100bLive(), back)

	mb, err := json.Marshal(tg.Message{MessageID: 1, LivePhoto: api100bLive()})
	require.NoError(t, err)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(mb, &keys))
	require.Contains(t, keys, "live_photo")
	assert.JSONEq(t, string(b), string(keys["live_photo"]))

	nb, err := json.Marshal(tg.Message{MessageID: 1})
	require.NoError(t, err)
	assert.NotContains(t, string(nb), `"live_photo"`)
}
