package tg_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 reply markup and general changes (B-65 65d, TEST-PLAN §2). Fake ids only: the group
// -1009876543210, the community 4503599627370495 (52 bits).

func b65dMarshal(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}

// T-enc-disabled
func TestB65D_EncodeDisabled(t *testing.T) {
	assert.Equal(t, `{"text":"off","disabled":{}}`,
		b65dMarshal(t, tg.InlineKeyboardButton{Text: "off", Disabled: &tg.DisabledButton{}}))
	assert.Equal(t, `{"text":"off"}`, b65dMarshal(t, tg.InlineKeyboardButton{Text: "off"}))
}

// T-enc-force
func TestB65D_EncodeForceReply(t *testing.T) {
	row := [][]tg.InlineKeyboardButton{{tg.Btn("a", "b")}}
	assert.Equal(t, `{"inline_keyboard":[[{"text":"a","callback_data":"b"}]],"force_reply":true}`,
		b65dMarshal(t, tg.InlineKeyboardMarkup{InlineKeyboard: row, ForceReply: true}))
	assert.Equal(t, `{"inline_keyboard":[[{"text":"a","callback_data":"b"}]]}`,
		b65dMarshal(t, tg.InlineKeyboardMarkup{InlineKeyboard: row}))
	kb := [][]tg.KeyboardButton{{{Text: "k"}}}
	assert.Equal(t, `{"keyboard":[[{"text":"k"}]],"force_reply":true}`,
		b65dMarshal(t, tg.ReplyKeyboardMarkup{Keyboard: kb, ForceReply: true}))
	assert.Equal(t, `{"keyboard":[[{"text":"k"}]]}`, b65dMarshal(t, tg.ReplyKeyboardMarkup{Keyboard: kb}))
}

// T-dec-probes: reply.result of the saved probes 02, 04 and 08 (ids replaced by fakes) decodes into the whole markup.
func TestB65D_DecodeProbes(t *testing.T) {
	cb, off := tg.InlineKeyboardButton{Text: "cb", CallbackData: "b65d"}, tg.InlineKeyboardButton{Text: "off", Disabled: &tg.DisabledButton{}}
	head := `{"message_id":42,"from":{"id":1000000001,"is_bot":true,"first_name":"b"},` +
		`"chat":{"id":-1009876543210,"type":"supergroup","title":"g"},"date":1795000000,`
	for _, tc := range []struct {
		name, js string
		want     tg.InlineKeyboardMarkup
	}{
		{"02 disabled", head + `"text":"galigo 65d disabled","reply_markup":{"inline_keyboard":[[{"text":"cb","callback_data":"b65d"},{"text":"off","disabled":{}}]]}}`,
			tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{cb, off}}}},
		{"04 disabled force_reply", head + `"text":"galigo 65d both","reply_markup":{"inline_keyboard":[[{"text":"off","disabled":{}}]],"force_reply":true}}`,
			tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{off}}, ForceReply: true}},
		{"08 edit", head + `"edit_date":1795000001,"text":"galigo 65d inline force_reply","reply_markup":{"inline_keyboard":[[{"text":"cb","callback_data":"b65d"},{"text":"off","disabled":{}}]],"force_reply":true}}`,
			tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{{cb, off}}, ForceReply: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var m tg.Message
			require.NoError(t, json.Unmarshal([]byte(tc.js), &m))
			require.NotNil(t, m.ReplyMarkup)
			assert.Equal(t, tc.want, *m.ReplyMarkup)
		})
	}
}

const b65dRights8 = `{"is_anonymous":false,"can_manage_chat":false,"can_delete_messages":false,"can_manage_video_chats":false,` +
	`"can_restrict_members":false,"can_promote_members":false,"can_change_info":false,"can_invite_users":false,`

// T-rights-enc
func TestB65D_EncodeRights(t *testing.T) {
	assert.Equal(t, b65dRights8+`"can_send_welcome_messages":false}`, b65dMarshal(t, tg.ChatAdministratorRights{}))
	assert.Equal(t, b65dRights8+`"can_send_welcome_messages":true}`,
		b65dMarshal(t, tg.ChatAdministratorRights{CanSendWelcomeMessages: true}))
}

// T-member-dec: the right decodes on an administrator member, directly and inside my_chat_member.
func TestB65D_DecodeMember(t *testing.T) {
	member := func(welcome string) string {
		return `{"status":"administrator","user":{"id":456,"is_bot":false,"first_name":"u"},"can_be_edited":false,` +
			`"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":false,"can_manage_video_chats":false,` +
			`"can_restrict_members":false,"can_promote_members":false,"can_change_info":false,"can_invite_users":true` + welcome + `}`
	}
	want := func(welcome bool) tg.ChatMemberAdministrator {
		var m tg.ChatMemberAdministrator
		m.User = &tg.User{ID: 456, FirstName: "u"}
		m.CanManageChat, m.CanInviteUsers, m.CanSendWelcomeMessages = true, true, welcome
		return m
	}
	got, err := tg.UnmarshalChatMember([]byte(member(`,"can_send_welcome_messages":true`)))
	require.NoError(t, err)
	assert.Equal(t, want(true), got)

	var u tg.Update
	require.NoError(t, json.Unmarshal([]byte(`{"update_id":1,"my_chat_member":{"chat":{"id":-1009876543210,"type":"supergroup"},`+
		`"from":{"id":789,"is_bot":false,"first_name":"f"},"date":1,"old_chat_member":`+member("")+`,"new_chat_member":`+
		member(`,"can_send_welcome_messages":true`)+`}}`), &u))
	require.NotNil(t, u.MyChatMember)
	assert.Equal(t, want(true), u.MyChatMember.NewChatMember)
	assert.Equal(t, want(false), u.MyChatMember.OldChatMember)

	got, err = tg.UnmarshalChatMember([]byte(member("")))
	require.NoError(t, err)
	assert.Equal(t, want(false), got)
}

// T-dec-stop: the update decodes into the whole value and re-encodes to the same keys; the zero value keeps its two
// required keys.
func TestB65D_DecodeStopped(t *testing.T) {
	chat := &tg.Chat{ID: -1009876543210, Type: "supergroup"}
	for _, tc := range []struct {
		name, js, again string
		want            tg.MessageGenerationStopped
	}{
		{"thread", `{"update_id":1,"stopped_message_generation":{"chat":{"id":-1009876543210,"type":"supergroup"},"message_thread_id":7,"draft_id":9}}`,
			`{"chat":{"id":-1009876543210,"type":"supergroup"},"message_thread_id":7,"draft_id":9}`,
			tg.MessageGenerationStopped{Chat: chat, MessageThreadID: 7, DraftID: 9}},
		{"no thread", `{"update_id":1,"stopped_message_generation":{"chat":{"id":-1009876543210,"type":"supergroup"},"draft_id":9}}`,
			`{"chat":{"id":-1009876543210,"type":"supergroup"},"draft_id":9}`,
			tg.MessageGenerationStopped{Chat: chat, DraftID: 9}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var u tg.Update
			require.NoError(t, json.Unmarshal([]byte(tc.js), &u))
			want := tc.want
			assert.Equal(t, tg.Update{UpdateID: 1, StoppedMessageGeneration: &want}, u)
			assert.Equal(t, tc.again, b65dMarshal(t, u.StoppedMessageGeneration))
		})
	}
	t.Run("zero", func(t *testing.T) {
		assert.Equal(t, `{"chat":null,"draft_id":0}`, b65dMarshal(t, tg.MessageGenerationStopped{}))
	})
}

// T-dec-joined
func TestB65D_DecodeJoined(t *testing.T) {
	js := `{"message_id":42,"date":1,"chat":{"id":-1009876543210,"type":"supergroup"},` +
		`"community_chat_joined":{"community":{"id":4503599627370495,"name":"c"}}}`
	var m tg.Message
	require.NoError(t, json.Unmarshal([]byte(js), &m))
	assert.Equal(t, tg.Message{MessageID: 42, Date: 1, Chat: &tg.Chat{ID: -1009876543210, Type: "supergroup"},
		CommunityChatJoined: &tg.CommunityChatJoined{Community: tg.Community{ID: 4503599627370495, Name: "c"}}}, m)
	assert.Equal(t, `{"community":{"id":4503599627370495,"name":"c"}}`, b65dMarshal(t, m.CommunityChatJoined))
}

// T-shapes: the new fields (name, Go type, tag, place, not embedded) and the whole field lists of the two new types.
func TestB65D_Shapes(t *testing.T) {
	type row struct{ name, typ, tag string }
	t.Run("new types", func(t *testing.T) {
		for _, tc := range []struct {
			v    any
			rows []row
		}{
			{tg.MessageGenerationStopped{}, []row{
				{"Chat", "*tg.Chat", "chat"}, {"MessageThreadID", "int", "message_thread_id,omitempty"}, {"DraftID", "int", "draft_id"},
			}},
			{tg.CommunityChatJoined{}, []row{{"Community", "tg.Community", "community"}}},
		} {
			rt := reflect.TypeOf(tc.v)
			var got []row
			for i := range rt.NumField() {
				f := rt.Field(i)
				assert.False(t, f.Anonymous, "%s.%s must be a named field, not embedded", rt, f.Name)
				got = append(got, row{f.Name, f.Type.String(), f.Tag.Get("json")})
			}
			assert.Equal(t, tc.rows, got, rt.String())
		}
	})
	t.Run("new fields", func(t *testing.T) {
		for _, tc := range []struct {
			v                      any
			field, typ, tag, after string // after: the field just before it ("" = the last field)
			last                   bool
		}{
			{tg.InlineKeyboardButton{}, "Disabled", "*tg.DisabledButton", "disabled,omitempty", "Pay", true},
			{tg.InlineKeyboardMarkup{}, "ForceReply", "bool", "force_reply,omitempty", "InlineKeyboard", true},
			{tg.ReplyKeyboardMarkup{}, "ForceReply", "bool", "force_reply,omitempty", "Selective", true},
			{tg.ChatAdministratorRights{}, "CanSendWelcomeMessages", "bool", "can_send_welcome_messages", "CanManageTags", true},
			{tg.ChatMemberAdministrator{}, "CanSendWelcomeMessages", "bool", "can_send_welcome_messages", "CanManageTags", false},
			{tg.Update{}, "StoppedMessageGeneration", "*tg.MessageGenerationStopped", "stopped_message_generation,omitempty", "", true},
			{tg.Message{}, "CommunityChatJoined", "*tg.CommunityChatJoined", "community_chat_joined,omitempty", "CommunityChatAdded", false},
		} {
			rt := reflect.TypeOf(tc.v)
			f, ok := rt.FieldByName(tc.field)
			if !assert.True(t, ok, "%s.%s", rt, tc.field) {
				continue
			}
			assert.False(t, f.Anonymous, "%s.%s must be a named field, not embedded", rt, tc.field)
			assert.Equal(t, tc.typ, f.Type.String(), "%s.%s", rt, tc.field)
			assert.Equal(t, tc.tag, f.Tag.Get("json"), "%s.%s", rt, tc.field)
			require.Len(t, f.Index, 1, "%s.%s is a direct field", rt, tc.field)
			i := f.Index[0]
			if tc.after != "" {
				assert.Equal(t, tc.after, rt.Field(i-1).Name, "the field before %s.%s", rt, tc.field)
			}
			assert.Equal(t, tc.last, i == rt.NumField()-1, "%s.%s is the last field", rt, tc.field)
		}
		// Message: CommunityChatJoined sits between CommunityChatAdded and CommunityChatRemoved
		f, _ := reflect.TypeOf(tg.Message{}).FieldByName("CommunityChatJoined")
		assert.Equal(t, "CommunityChatRemoved", reflect.TypeOf(tg.Message{}).Field(f.Index[0]+1).Name)
	})
}

// T-zero (guard): the zero markups encode as before 65d.
func TestB65D_ZeroMarkup(t *testing.T) {
	assert.Equal(t, `{"text":""}`, b65dMarshal(t, tg.InlineKeyboardButton{}))
	assert.Equal(t, `{"inline_keyboard":null}`, b65dMarshal(t, tg.InlineKeyboardMarkup{}))
	assert.Equal(t, `{"keyboard":null}`, b65dMarshal(t, tg.ReplyKeyboardMarkup{}))
}
