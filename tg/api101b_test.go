package tg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 part b: join request queries on the read side (B-25 10.1b, TEST-PLAN §2, rows T-tg-1..T-tg-5).
//
// api101bGetMe is the getMe reply of 2026-11-07 (E10/lookups/probe101b/01-getMe.json), key order and values
// byte-exact except the bot's own id, first_name and username, which are fakes (the repo is public).
const api101bGetMe = `{"id":1000000101,"is_bot":true,"first_name":"Test Bot","username":"test_101b_bot",` +
	`"can_join_groups":true,"can_read_all_group_messages":false,"supports_inline_queries":false,` +
	`"supports_guest_queries":false,"can_connect_to_business":false,"has_main_web_app":false,` +
	`"has_topics_enabled":false,"allows_users_to_create_topics":false,"can_manage_bots":false,` +
	`"supports_join_request_queries":false}`

// T-tg-1: User.supports_join_request_queries decodes (false as fetched, true when set) and encodes only when true.
func TestAPI101B_UserSupportsJoinRequestQueries(t *testing.T) {
	var asFetched tg.User
	require.NoError(t, json.Unmarshal([]byte(api101bGetMe), &asFetched))
	assert.False(t, asFetched.SupportsJoinRequestQueries)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(api101bGetMe), &m))
	m["supports_join_request_queries"] = true
	b, err := json.Marshal(m)
	require.NoError(t, err)
	var on tg.User
	require.NoError(t, json.Unmarshal(b, &on))
	assert.True(t, on.SupportsJoinRequestQueries)
	other := on
	other.SupportsJoinRequestQueries = false
	assert.Equal(t, asFetched, other, "every other field equals the as-fetched decode")

	enc, err := json.Marshal(on)
	require.NoError(t, err)
	assert.Contains(t, string(enc), `"supports_join_request_queries":true`)
	enc, err = json.Marshal(asFetched)
	require.NoError(t, err)
	assert.NotContains(t, string(enc), `supports_join_request_queries`)
}

// T-tg-3: today's getMe reply, without the three keys galigo still does not model, decodes strictly into tg.User.
func TestAPI101B_StrictDecodeGetMe(t *testing.T) {
	b := api96Sub(t, api101bGetMe, nil, "can_connect_to_business", "has_main_web_app", "has_topics_enabled")
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	require.Len(t, m, 11, "the reply's other 11 keys stay")
	require.Contains(t, m, "supports_join_request_queries")
	var u tg.User
	require.NoError(t, api96Strict(t, b, &u))
}

// api101bGetChat is a synthetic getChat result built from the shape of probe 02 (fake id, title and link).
const api101bGetChat = `{"id":-1000000000101,"title":"Test Group","is_forum":true,"type":"supergroup",` +
	`"invite_link":"https://t.me/+fake101b","has_visible_history":true,"join_to_send_messages":true,` +
	`"max_reaction_count":11,"accent_color_id":1}`

// T-tg-4: ChatFullInfo.guard_bot decodes into a *User, is nil when absent, and encodes only when set.
func TestAPI101B_ChatFullInfoGuardBot(t *testing.T) {
	guard := `{"id":4242,"is_bot":true,"first_name":"Guard","username":"guard_bot"}`
	withGuard := api101bGetChat[:len(api101bGetChat)-1] + `,"guard_bot":` + guard + `}`

	var c tg.ChatFullInfo
	require.NoError(t, json.Unmarshal([]byte(withGuard), &c))
	assert.Equal(t, &tg.User{ID: 4242, IsBot: true, FirstName: "Guard", Username: "guard_bot"}, c.GuardBot)

	var none tg.ChatFullInfo
	require.NoError(t, json.Unmarshal([]byte(api101bGetChat), &none))
	assert.Nil(t, none.GuardBot)

	b, err := json.Marshal(tg.ChatFullInfo{ID: 1, Type: "supergroup"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "guard_bot")

	b, err = json.Marshal(tg.ChatFullInfo{ID: 1, Type: "supergroup",
		GuardBot: &tg.User{ID: 4242, IsBot: true, FirstName: "Guard", Username: "guard_bot"}})
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &got))
	require.Contains(t, got, "guard_bot")
	assert.JSONEq(t, guard, string(got["guard_bot"]))
}

// T-tg-5: ChatJoinRequest.query_id decodes byte for byte (strictly, with every spec key) and encodes only when set.
func TestAPI101B_ChatJoinRequestQueryID(t *testing.T) {
	const qid = " jQ-Ab+%2fx/Y=="
	upd := `{"update_id":7,"chat_join_request":{` +
		`"chat":{"id":-1000000000101,"title":"Test Group","type":"supergroup"},` +
		`"from":{"id":1000000202,"is_bot":false,"first_name":"Joiner"},` +
		`"user_chat_id":1000000202,"date":1762500000,"bio":"hi",` +
		`"invite_link":{"invite_link":"https://t.me/+fake101b","creator":{"id":1000000303,"is_bot":false,"first_name":"Admin"},` +
		`"creates_join_request":true,"is_primary":false,"is_revoked":false},` +
		`"query_id":" jQ-Ab+%2fx/Y=="}}`

	var u tg.Update
	require.NoError(t, json.Unmarshal([]byte(upd), &u))
	require.NotNil(t, u.ChatJoinRequest)
	assert.Equal(t, qid, u.ChatJoinRequest.QueryID)

	obj := api96Sub(t, upd, []any{"chat_join_request"})
	var r tg.ChatJoinRequest
	require.NoError(t, api96Strict(t, obj, &r))
	assert.Equal(t, qid, r.QueryID)

	noQ := api96Sub(t, upd, []any{"chat_join_request"}, "query_id")
	var r2 tg.ChatJoinRequest
	require.NoError(t, json.Unmarshal(noQ, &r2))
	assert.Equal(t, "", r2.QueryID)

	b, err := json.Marshal(tg.ChatJoinRequest{UserChatID: 1, Date: 2})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "query_id")

	b, err = json.Marshal(tg.ChatJoinRequest{UserChatID: 1, Date: 2, QueryID: qid})
	require.NoError(t, err)
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &got))
	require.Contains(t, got, "query_id")
	assert.Equal(t, `" jQ-Ab+%2fx/Y=="`, string(got["query_id"]))
}
