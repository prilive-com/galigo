package tg_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 9.6 decode and encode tests (B-25 slice 1, TEST-PLAN §2, rows T-tg-1 … T-tg-14).
//
// Live fixtures: replies of 2026-11-04 (E7/lookups/probe/01, 02, 04, 05, 06). The only edit is the identity
// fields, replaced by internal/testutil values because the repository is public: bot id 8434949118 ->
// testutil.TestBotID, bot first_name -> "Test Bot", bot username -> testutil.TestBotUsername, chat id ->
// testutil.TestChatID, chat title -> "Test Chat". Every 9.6 field is byte-exact. Fixtures named "synthetic"
// are built from the API reference tables (E7/lookups/sec-*.txt); there is no live sample for them.

const (
	api96From = `{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"}`
	api96Chat = `{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"}`

	// probe 02: quiz with one correct answer (sendPoll with correct_option_ids [1]).
	api96Probe02 = `{"message_id":996,"from":` + api96From + `,"chat":` + api96Chat + `,"date":1793758513,` +
		`"poll":{"id":"5429169679568477049","question":"probe96 quiz ids","options":[` +
		`{"persistent_id":"0","text":"A","voter_count":0},{"persistent_id":"1","text":"B","voter_count":0},` +
		`{"persistent_id":"2","text":"C","voter_count":0}],"total_voter_count":0,"is_closed":false,` +
		`"is_anonymous":false,"allows_multiple_answers":false,"allows_revoting":false,"members_only":false,` +
		`"type":"quiz","correct_option_id":1,"correct_option_ids":[1]}}`

	// probe 04: quiz with two correct answers; Telegram leaves correct_option_id out.
	api96Probe04 = `{"message_id":998,"from":` + api96From + `,"chat":` + api96Chat + `,"date":1793758513,` +
		`"poll":{"id":"5429169679568477051","question":"probe96 quiz multi","options":[` +
		`{"persistent_id":"0","text":"A","voter_count":0},{"persistent_id":"1","text":"B","voter_count":0},` +
		`{"persistent_id":"2","text":"C","voter_count":0}],"total_voter_count":0,"is_closed":false,` +
		`"is_anonymous":false,"allows_multiple_answers":true,"allows_revoting":false,"members_only":false,` +
		`"type":"quiz","correct_option_ids":[0,2]}}`

	api96Probe05Poll = `{"id":"5429169679568477052","question":"probe96 regular","options":[` +
		`{"persistent_id":"0","text":"A","voter_count":0},{"persistent_id":"1","text":"B","voter_count":0},` +
		`{"persistent_id":"2","text":"C","voter_count":0}],"total_voter_count":0,"open_period":2628000,` +
		`"close_date":1796386513,"is_closed":false,"is_anonymous":false,"allows_multiple_answers":false,` +
		`"allows_revoting":true,"members_only":false,"type":"regular","description":"probe96 description",` +
		`"description_entities":[{"offset":8,"length":11,"type":"bold"}]}`

	// probe 05: regular poll with the 9.6 options.
	api96Probe05 = `{"message_id":999,"from":` + api96From + `,"chat":` + api96Chat + `,"date":1793758513,` +
		`"poll":` + api96Probe05Poll + `}`

	// probe 06: sendMessage reply to option "1" of probe 05's poll.
	api96Probe06 = `{"message_id":1000,"from":` + api96From + `,"chat":` + api96Chat + `,"date":1793758513,` +
		`"reply_to_message":` + api96Probe05 + `,"reply_to_poll_option_id":"1","text":"probe96 reply to option B"}`

	// probe 01: getMe.
	api96Probe01 = `{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot",` +
		`"can_join_groups":true,"can_read_all_group_messages":false,"supports_inline_queries":false,` +
		`"supports_guest_queries":false,"can_connect_to_business":false,"has_main_web_app":false,` +
		`"has_topics_enabled":false,"allows_users_to_create_topics":false,"can_manage_bots":false,` +
		`"supports_join_request_queries":false}`

	// synthetic users A (owner) and B (managed bot), distinct ids and names.
	api96UserA = `{"id":987654321,"is_bot":false,"first_name":"Test","username":"testuser"}`
	api96UserB = `{"id":555000111,"is_bot":true,"first_name":"Managed","username":"managed_test_bot"}`

	// synthetic: Message with managed_bot_created.
	api96ManagedCreated = `{"message_id":7,"date":1793758513,"chat":{"id":987654321,"type":"private"},` +
		`"managed_bot_created":{"bot":` + api96UserB + `}}`

	// synthetic: Update with managed_bot.
	api96ManagedUpdate = `{"update_id":42,"managed_bot":{"user":` + api96UserA + `,"bot":` + api96UserB + `}}`

	// synthetic: option added (accessible poll message) and deleted (inaccessible poll message: date 0).
	api96OptionAdded = `{"message_id":11,"date":1793758600,"chat":{"id":123456789,"type":"supergroup","title":"Test Chat"},` +
		`"poll_option_added":{"poll_message":{"message_id":999,"date":1793758513,` +
		`"chat":{"id":123456789,"type":"supergroup","title":"Test Chat"}},"option_persistent_id":"3",` +
		`"option_text":"D added","option_text_entities":[{"type":"italic","offset":0,"length":1}]}}`
	api96OptionDeleted = `{"message_id":12,"date":1793758700,"chat":{"id":123456789,"type":"supergroup","title":"Test Chat"},` +
		`"poll_option_deleted":{"poll_message":{"message_id":998,"date":0,` +
		`"chat":{"id":123456789,"type":"supergroup","title":"Test Chat"}},"option_persistent_id":"4",` +
		`"option_text":"E deleted","option_text_entities":[{"type":"bold","offset":2,"length":7}]}}`

	// synthetic: Update with poll_answer.
	api96PollAnswer = `{"update_id":43,"poll_answer":{"poll_id":"5429169679568477052","user":` + api96UserA +
		`,"option_ids":[0,2],"option_persistent_ids":["a","c"]}}`

	// synthetic: an option added by a user, in a chat.
	api96AddedOption = `{"persistent_id":"5","text":"F","voter_count":3,"added_by_user":` + api96UserA +
		`,"added_by_chat":{"id":123456789,"type":"supergroup","title":"Test Chat"},"addition_date":1793758800}`
)

func api96Bot() *tg.User {
	return &tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot", Username: testutil.TestBotUsername}
}

func api96ChatValue() *tg.Chat {
	return &tg.Chat{ID: testutil.TestChatID, Title: "Test Chat", IsForum: true, Type: "supergroup"}
}

func api96GroupChat() *tg.Chat {
	return &tg.Chat{ID: testutil.TestChatID, Type: "supergroup", Title: "Test Chat"}
}

func api96A() *tg.User {
	return &tg.User{ID: testutil.TestUserID, FirstName: "Test", Username: testutil.TestUsername}
}

func api96B() *tg.User {
	return &tg.User{ID: 555000111, IsBot: true, FirstName: "Managed", Username: "managed_test_bot"}
}

func api96Options() []tg.PollOption {
	return []tg.PollOption{
		{PersistentID: "0", Text: "A"},
		{PersistentID: "1", Text: "B"},
		{PersistentID: "2", Text: "C"},
	}
}

func api96Probe05Message() *tg.Message {
	return &tg.Message{
		MessageID: 999, From: api96Bot(), Chat: api96ChatValue(), Date: 1793758513,
		Poll: &tg.Poll{
			ID: "5429169679568477052", Question: "probe96 regular", Options: api96Options(),
			OpenPeriod: 2628000, CloseDate: 1796386513, Type: "regular",
			AllowsRevoting: true, Description: "probe96 description",
			DescriptionEntities: []tg.MessageEntity{{Type: "bold", Offset: 8, Length: 11}},
		},
	}
}

func api96Decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

// T-tg-1, T-tg-2, T-tg-3, T-tg-4: live poll and reply messages decode to the whole expected Message.
func TestAPI96_DecodeLivePollMessages(t *testing.T) {
	t.Run("T-tg-1 single-answer quiz", func(t *testing.T) {
		want := tg.Message{
			MessageID: 996, From: api96Bot(), Chat: api96ChatValue(), Date: 1793758513,
			Poll: &tg.Poll{
				ID: "5429169679568477049", Question: "probe96 quiz ids", Options: api96Options(),
				Type: "quiz", CorrectOptionID: 1, CorrectOptionIDs: []int{1}, AllowsRevoting: false,
			},
		}
		assert.Equal(t, want, api96Decode[tg.Message](t, api96Probe02))
	})
	t.Run("T-tg-2 quiz with two correct answers", func(t *testing.T) {
		want := tg.Message{
			MessageID: 998, From: api96Bot(), Chat: api96ChatValue(), Date: 1793758513,
			Poll: &tg.Poll{
				ID: "5429169679568477051", Question: "probe96 quiz multi", Options: api96Options(),
				Type: "quiz", AllowsMultipleAnswers: true, CorrectOptionID: 0, CorrectOptionIDs: []int{0, 2},
			},
		}
		assert.Equal(t, want, api96Decode[tg.Message](t, api96Probe04))
	})
	t.Run("T-tg-3 regular poll with 9.6 options", func(t *testing.T) {
		assert.Equal(t, *api96Probe05Message(), api96Decode[tg.Message](t, api96Probe05))
	})
	t.Run("T-tg-4 reply to a poll option", func(t *testing.T) {
		want := tg.Message{
			MessageID: 1000, From: api96Bot(), Chat: api96ChatValue(), Date: 1793758513,
			ReplyToMessage: api96Probe05Message(), ReplyToPollOptionID: "1", Text: "probe96 reply to option B",
		}
		assert.Equal(t, want, api96Decode[tg.Message](t, api96Probe06))
	})
}

// T-tg-5: getMe decodes can_manage_bots, both false (live) and true (synthetic copy).
func TestAPI96_DecodeGetMeCanManageBots(t *testing.T) {
	base := tg.User{ID: testutil.TestBotID, IsBot: true, FirstName: "Test Bot", Username: testutil.TestBotUsername, CanJoinGroups: true}

	assert.Equal(t, base, api96Decode[tg.User](t, api96Probe01), "live can_manage_bots:false")

	withTrue := strings.Replace(api96Probe01, `"can_manage_bots":false`, `"can_manage_bots":true`, 1)
	require.NotEqual(t, api96Probe01, withTrue)
	want := base
	want.CanManageBots = true
	assert.Equal(t, want, api96Decode[tg.User](t, withTrue), "synthetic can_manage_bots:true")
}

// T-tg-6: Message.managed_bot_created decodes to the whole ManagedBotCreated.
func TestAPI96_DecodeManagedBotCreated(t *testing.T) {
	want := tg.Message{
		MessageID: 7, Date: 1793758513, Chat: &tg.Chat{ID: testutil.TestUserID, Type: "private"},
		ManagedBotCreated: &tg.ManagedBotCreated{Bot: api96B()},
	}
	assert.Equal(t, want, api96Decode[tg.Message](t, api96ManagedCreated))
}

// T-tg-7: Update.managed_bot decodes user and bot to the right fields (A != B).
func TestAPI96_DecodeManagedBotUpdate(t *testing.T) {
	want := tg.Update{UpdateID: 42, ManagedBot: &tg.ManagedBotUpdated{User: api96A(), Bot: api96B()}}
	assert.Equal(t, want, api96Decode[tg.Update](t, api96ManagedUpdate))
}

// T-tg-8: poll_option_added and poll_option_deleted land in their own fields, whole values.
func TestAPI96_DecodePollOptionAddedDeleted(t *testing.T) {
	added := tg.Message{
		MessageID: 11, Date: 1793758600, Chat: api96GroupChat(),
		PollOptionAdded: &tg.PollOptionAdded{
			PollMessage:        &tg.Message{MessageID: 999, Date: 1793758513, Chat: api96GroupChat()},
			OptionPersistentID: "3", OptionText: "D added",
			OptionTextEntities: []tg.MessageEntity{{Type: "italic", Offset: 0, Length: 1}},
		},
	}
	assert.Equal(t, added, api96Decode[tg.Message](t, api96OptionAdded))

	deleted := tg.Message{
		MessageID: 12, Date: 1793758700, Chat: api96GroupChat(),
		PollOptionDeleted: &tg.PollOptionDeleted{
			PollMessage:        &tg.Message{MessageID: 998, Date: 0, Chat: api96GroupChat()},
			OptionPersistentID: "4", OptionText: "E deleted",
			OptionTextEntities: []tg.MessageEntity{{Type: "bold", Offset: 2, Length: 7}},
		},
	}
	assert.Equal(t, deleted, api96Decode[tg.Message](t, api96OptionDeleted))
}

// T-tg-9: poll_answer option_persistent_ids, and an option's added_by_user, added_by_chat, addition_date.
func TestAPI96_DecodePollAnswerAndAddedOption(t *testing.T) {
	wantAnswer := tg.Update{UpdateID: 43, PollAnswer: &tg.PollAnswer{
		PollID: "5429169679568477052", User: api96A(), OptionIDs: []int{0, 2}, OptionPersistentIDs: []string{"a", "c"},
	}}
	assert.Equal(t, wantAnswer, api96Decode[tg.Update](t, api96PollAnswer))

	wantOption := tg.PollOption{
		PersistentID: "5", Text: "F", VoterCount: 3, AddedByUser: api96A(), AddedByChat: api96GroupChat(),
		AdditionDate: 1793758800,
	}
	assert.Equal(t, wantOption, api96Decode[tg.PollOption](t, api96AddedOption))
}

// T-tg-10: KeyboardButton.request_managed_bot encodes exactly; request_id 0 is still sent (required field).
func TestAPI96_EncodeRequestManagedBotButton(t *testing.T) {
	full := tg.KeyboardButton{Text: "Create bot", RequestManagedBot: &tg.KeyboardButtonRequestManagedBot{
		RequestID: 7, SuggestedName: "N", SuggestedUsername: "n_bot",
	}}
	b, err := json.Marshal(full)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"Create bot","request_managed_bot":{"request_id":7,"suggested_name":"N","suggested_username":"n_bot"}}`, string(b))

	zero := tg.KeyboardButton{Text: "Create bot", RequestManagedBot: &tg.KeyboardButtonRequestManagedBot{}}
	b, err = json.Marshal(zero)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"Create bot","request_managed_bot":{"request_id":0}}`, string(b))
}

// T-tg-11: ReplyParameters.poll_option_id encodes when set and is absent when empty.
func TestAPI96_EncodeReplyParametersPollOption(t *testing.T) {
	b, err := json.Marshal(tg.ReplyParameters{MessageID: 5, PollOptionID: "1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"message_id":5,"poll_option_id":"1"}`, string(b))

	b, err = json.Marshal(tg.ReplyParameters{MessageID: 5})
	require.NoError(t, err)
	assert.JSONEq(t, `{"message_id":5}`, string(b))
}

// api96Sub returns the JSON value at path (object keys and array indexes) inside raw, with the named keys of
// that value deleted (the keys galigo does not model; the base fixture is not edited).
func api96Sub(t *testing.T, raw string, path []any, del ...string) []byte {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			require.True(t, ok, "path %v: not an object at %q", path, k)
			v, ok = m[k]
			require.True(t, ok, "path %v: no key %q", path, k)
		case int:
			a, ok := v.([]any)
			require.True(t, ok, "path %v: not an array at %d", path, k)
			require.Less(t, k, len(a))
			v = a[k]
		}
	}
	if m, ok := v.(map[string]any); ok {
		for _, k := range del {
			_, had := m[k]
			require.True(t, had, "key %q to delete is not in the fixture", k)
			delete(m, k)
		}
	}
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func api96Strict(t *testing.T, b []byte, v any) error {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// T-tg-12: every 9.6 object decodes strictly (DisallowUnknownFields) into its galigo type, so a tag typo fails
// even when the value is zero.
func TestAPI96_StrictDecode(t *testing.T) {
	// API100 T-tg-12: supports_guest_queries is modelled since 10.0 (tg.User.SupportsGuestQueries), so it is no
	// longer deleted before the strict decode.
	getMeUnmodelled := []string{"can_connect_to_business", "has_main_web_app", "has_topics_enabled",
		"supports_join_request_queries"}
	rows := []struct {
		name string
		b    []byte
		v    any
	}{
		{"User getMe (probe 01)", api96Sub(t, api96Probe01, nil, getMeUnmodelled...), &tg.User{}},
		{"Poll quiz (probe 02)", api96Sub(t, api96Probe02, []any{"poll"}, "members_only"), &tg.Poll{}},
		{"Poll quiz multi (probe 04)", api96Sub(t, api96Probe04, []any{"poll"}, "members_only"), &tg.Poll{}},
		{"Poll regular (probe 05)", api96Sub(t, api96Probe05, []any{"poll"}, "members_only"), &tg.Poll{}},
		{"PollOption (probe 02)", api96Sub(t, api96Probe02, []any{"poll", "options", 0}), &tg.PollOption{}},
		{"PollOption added (synthetic)", api96Sub(t, api96AddedOption, nil), &tg.PollOption{}},
		{"PollAnswer (synthetic)", api96Sub(t, api96PollAnswer, []any{"poll_answer"}), &tg.PollAnswer{}},
		{"PollOptionAdded (synthetic)", api96Sub(t, api96OptionAdded, []any{"poll_option_added"}), &tg.PollOptionAdded{}},
		{"PollOptionDeleted (synthetic)", api96Sub(t, api96OptionDeleted, []any{"poll_option_deleted"}), &tg.PollOptionDeleted{}},
		{"ManagedBotCreated (synthetic)", api96Sub(t, api96ManagedCreated, []any{"managed_bot_created"}), &tg.ManagedBotCreated{}},
		{"ManagedBotUpdated (synthetic)", api96Sub(t, api96ManagedUpdate, []any{"managed_bot"}), &tg.ManagedBotUpdated{}},
		{"KeyboardButtonRequestManagedBot (synthetic)", []byte(`{"request_id":7,"suggested_name":"N","suggested_username":"n_bot"}`), &tg.KeyboardButtonRequestManagedBot{}},
		{"PreparedKeyboardButton (probe 08 shape)", []byte(`{"id":"prepared-button-id"}`), &tg.PreparedKeyboardButton{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			assert.NoError(t, api96Strict(t, r.b, r.v))
		})
	}
}

// T-tg-13: zero values encode with the new non-omitempty keys (allows_revoting, persistent_id,
// option_persistent_ids): a compatibility fact for AC4, recorded exactly.
func TestAPI96_EncodeZeroValues(t *testing.T) {
	rows := []struct {
		name string
		v    any
		want string
	}{
		{"Poll", tg.Poll{}, `{"id":"","question":"","options":null,"total_voter_count":0,"is_closed":false,` +
			`"is_anonymous":false,"type":"","allows_multiple_answers":false,"allows_revoting":false}`},
		{"PollOption", tg.PollOption{}, `{"text":"","voter_count":0,"persistent_id":""}`},
		{"PollAnswer", tg.PollAnswer{}, `{"poll_id":"","option_ids":null,"option_persistent_ids":null}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			assert.JSONEq(t, r.want, string(b))
		})
	}
}

// T-tg-14: round trip. Each decoded 9.6 object is encoded again and its 9.6 keys must appear with the fixture's
// exact names and values. Decoding matches names without case, so only encoding sees a case-only tag typo.
func TestAPI96_RoundTripKeys(t *testing.T) {
	type row struct {
		name    string
		fixture string
		path    []any
		decode  func([]byte) (any, error)
		keys    []string
	}
	into := func(newV func() any) func([]byte) (any, error) {
		return func(b []byte) (any, error) { v := newV(); return v, json.Unmarshal(b, v) }
	}
	rows := []row{
		{"Poll regular (probe 05)", api96Probe05, []any{"poll"}, into(func() any { return &tg.Poll{} }),
			[]string{"allows_revoting", "description", "description_entities"}},
		{"Poll quiz multi (probe 04)", api96Probe04, []any{"poll"}, into(func() any { return &tg.Poll{} }),
			[]string{"correct_option_ids", "allows_revoting"}},
		{"PollOption (probe 02)", api96Probe02, []any{"poll", "options", 1}, into(func() any { return &tg.PollOption{} }),
			[]string{"persistent_id"}},
		{"PollOption added (synthetic)", api96AddedOption, nil, into(func() any { return &tg.PollOption{} }),
			[]string{"persistent_id", "added_by_user", "added_by_chat", "addition_date"}},
		{"PollAnswer (synthetic)", api96PollAnswer, []any{"poll_answer"}, into(func() any { return &tg.PollAnswer{} }),
			[]string{"option_persistent_ids"}},
		{"Message poll_option_added (synthetic)", api96OptionAdded, nil, into(func() any { return &tg.Message{} }),
			[]string{"poll_option_added"}},
		{"Message poll_option_deleted (synthetic)", api96OptionDeleted, nil, into(func() any { return &tg.Message{} }),
			[]string{"poll_option_deleted"}},
		{"Message managed_bot_created (synthetic)", api96ManagedCreated, nil, into(func() any { return &tg.Message{} }),
			[]string{"managed_bot_created"}},
		{"Update managed_bot (synthetic)", api96ManagedUpdate, nil, into(func() any { return &tg.Update{} }),
			[]string{"managed_bot"}},
		{"User can_manage_bots (synthetic true)", strings.Replace(api96Probe01, `"can_manage_bots":false`, `"can_manage_bots":true`, 1),
			nil, into(func() any { return &tg.User{} }), []string{"can_manage_bots"}},
		{"Message reply_to_poll_option_id (probe 06)", api96Probe06, nil, into(func() any { return &tg.Message{} }),
			[]string{"reply_to_poll_option_id"}},
	}
	pick := func(t *testing.T, b []byte, keys []string) string {
		t.Helper()
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &m))
		out := map[string]json.RawMessage{}
		for _, k := range keys {
			if v, ok := m[k]; ok {
				out[k] = v
			}
		}
		r, err := json.Marshal(out)
		require.NoError(t, err)
		return string(r)
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			src := api96Sub(t, r.fixture, r.path)
			v, err := r.decode(src)
			require.NoError(t, err)
			enc, err := json.Marshal(v)
			require.NoError(t, err)
			want := pick(t, src, r.keys)
			require.NotEqual(t, "{}", want, "fixture has none of the keys")
			assert.JSONEq(t, want, pick(t, enc, r.keys))
		})
	}
}
