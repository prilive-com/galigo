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

// Bot API 10.2 part a: communities and subscription updates (read side). Expected values are literals checked
// through field selectors and JSONEq, never composite literals of the new types, so a wrong field type still
// compiles and fails by assertion. Ids are fake.
const (
	// api102aCommunity has the largest id the spec allows (2^52-1: above 2^32, exact in float64).
	api102aCommunity = `{"id":4503599627370495,"name":"Test Community"}`
	api102aMsgBase   = `{"message_id":1001,"date":1700000000,"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}`
	api102aMsgAdded  = api102aMsgBase + `,"community_chat_added":{"community":` + api102aCommunity + `}}`
	// api102aMsgRemoved carries the empty service object.
	api102aMsgRemoved   = api102aMsgBase + `,"community_chat_removed":{}}`
	api102aChatFullNone = `{"id":-1000000000102,"title":"Test Group","type":"supergroup","max_reaction_count":11,"accent_color_id":1}`
	api102aChatFull     = `{"id":-1000000000102,"title":"Test Group","type":"supergroup","max_reaction_count":11,"accent_color_id":1,` +
		`"community":` + api102aCommunity + `}`
)

func api102aSubscription(state string) string {
	return `{"update_id":7,"subscription":{"user":{"id":123456789,"is_bot":false,"first_name":"Test"},` +
		`"invoice_payload":"sub-42","state":"` + state + `"}}`
}

func api102aMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func api102aKeys(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(api102aMarshal(t, v)), &m))
	return m
}

// T-com-1: Community decodes and encodes id and name; the id keeps all 52 bits; an unknown key is ignored.
func TestAPI102A_Community(t *testing.T) {
	for _, tc := range []struct {
		in, name string
		id       int64
	}{
		{api102aCommunity, "Test Community", 4503599627370495},
		{`{"id":5000000000,"name":"a"}`, "a", 5000000000},
		{`{"id":-4503599627370495,"name":"n"}`, "n", -4503599627370495},
	} {
		var c tg.Community
		require.NoError(t, api96Strict(t, []byte(tc.in), &c), tc.in)
		assert.Equal(t, tc.id, int64(c.ID), tc.in)
		assert.Equal(t, tc.name, c.Name, tc.in)
		assert.JSONEq(t, tc.in, api102aMarshal(t, c), tc.in)
	}

	var c tg.Community
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"name":"x","future":true}`), &c))
	assert.Equal(t, int64(1), int64(c.ID))
	assert.Equal(t, "x", c.Name)
	assert.JSONEq(t, `{"id":1,"name":"x"}`, api102aMarshal(t, c))
}

// T-cca-1: Message.community_chat_added holds a Community value.
func TestAPI102A_ChatAdded(t *testing.T) {
	var msg tg.Message
	require.NoError(t, api96Strict(t, []byte(api102aMsgAdded), &msg))
	require.NotNil(t, msg.CommunityChatAdded)
	assert.Equal(t, int64(4503599627370495), int64(msg.CommunityChatAdded.Community.ID))
	assert.Equal(t, "Test Community", msg.CommunityChatAdded.Community.Name)
	assert.JSONEq(t, `{"community":`+api102aCommunity+`}`, api102aMarshal(t, msg.CommunityChatAdded))
	assert.Nil(t, msg.CommunityChatRemoved)
	assert.JSONEq(t, api102aMsgAdded, api102aMarshal(t, msg))
}

// T-ccr-1: Message.community_chat_removed is set for {} and unset for null and for an absent key.
func TestAPI102A_ChatRemoved(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var msg tg.Message
		require.NoError(t, api96Strict(t, []byte(api102aMsgRemoved), &msg))
		assert.NotNil(t, msg.CommunityChatRemoved)
		assert.Nil(t, msg.CommunityChatAdded)
		assert.JSONEq(t, api102aMsgRemoved, api102aMarshal(t, msg))
	})
	t.Run("null", func(t *testing.T) {
		var msg tg.Message
		require.NoError(t, api96Strict(t, []byte(api102aMsgBase+`,"community_chat_removed":null}`), &msg))
		assert.Nil(t, msg.CommunityChatRemoved)
	})
	t.Run("absent", func(t *testing.T) {
		var msg tg.Message
		require.NoError(t, api96Strict(t, []byte(api102aMsgBase+`}`), &msg))
		assert.Nil(t, msg.CommunityChatRemoved)
	})
}

// T-cfi-1: ChatFullInfo.community decodes when present, stays nil when absent, and is omitted when nil.
func TestAPI102A_ChatFullInfo(t *testing.T) {
	var c tg.ChatFullInfo
	require.NoError(t, api96Strict(t, []byte(api102aChatFull), &c))
	require.NotNil(t, c.Community)
	assert.Equal(t, int64(4503599627370495), int64(c.Community.ID))
	assert.Equal(t, "Test Community", c.Community.Name)
	m := api102aKeys(t, c)
	require.Contains(t, m, "community")
	assert.JSONEq(t, api102aCommunity, string(m["community"]))

	var none tg.ChatFullInfo
	require.NoError(t, api96Strict(t, []byte(api102aChatFullNone), &none))
	assert.Nil(t, none.Community)
	assert.NotContains(t, api102aKeys(t, none), "community")
}

// T-sub-1: Update.subscription decodes every state as given (an open set: "paused" is not a spec value).
func TestAPI102A_Subscription(t *testing.T) {
	for _, state := range []string{"canceled", "active", "failed", "paused"} {
		t.Run(state, func(t *testing.T) {
			in := api102aSubscription(state)
			var u tg.Update
			require.NoError(t, api96Strict(t, []byte(in), &u))
			assert.Equal(t, 7, u.UpdateID)
			assert.Nil(t, u.Message)
			require.NotNil(t, u.Subscription)
			require.NotNil(t, u.Subscription.User)
			assert.Equal(t, int64(123456789), u.Subscription.User.ID)
			assert.Equal(t, "Test", u.Subscription.User.FirstName)
			assert.False(t, u.Subscription.User.IsBot)
			assert.Equal(t, "sub-42", u.Subscription.InvoicePayload)
			assert.Equal(t, state, u.Subscription.State)
			assert.JSONEq(t, in, api102aMarshal(t, u))
		})
	}
}

// T-zero: the zero values encode the required keys of the new types, and the holders omit the new keys.
func TestAPI102A_ZeroEncode(t *testing.T) {
	assert.Equal(t, `{"id":0,"name":""}`, api102aMarshal(t, tg.Community{}))
	assert.Equal(t, `{"community":{"id":0,"name":""}}`, api102aMarshal(t, tg.CommunityChatAdded{}))
	assert.Equal(t, `{}`, api102aMarshal(t, tg.CommunityChatRemoved{}))
	assert.Equal(t, `{"user":null,"invoice_payload":"","state":""}`, api102aMarshal(t, tg.BotSubscriptionUpdated{}))
	for _, holder := range []any{tg.Message{}, tg.ChatFullInfo{}, tg.Update{}} {
		m := api102aKeys(t, holder)
		for _, key := range []string{"community_chat_added", "community_chat_removed", "community", "subscription"} {
			assert.NotContains(t, m, key, "%T", holder)
		}
	}
}

// T-shape: the new types have exactly the planned fields and no exported methods, and the holders have the planned
// fields. Type strings come from reflect, so a float64 id or a value in place of a pointer fails.
func TestAPI102A_Shapes(t *testing.T) {
	type row struct{ field, typ, tag string }
	t.Run("new types", func(t *testing.T) {
		for _, tc := range []struct {
			v    any
			rows []row
		}{
			{tg.Community{}, []row{{"ID", "int64", "id"}, {"Name", "string", "name"}}},
			{tg.CommunityChatAdded{}, []row{{"Community", "tg.Community", "community"}}},
			{tg.CommunityChatRemoved{}, nil},
			{tg.BotSubscriptionUpdated{}, []row{
				{"User", "*tg.User", "user"}, {"InvoicePayload", "string", "invoice_payload"}, {"State", "string", "state"},
			}},
		} {
			rt := reflect.TypeOf(tc.v)
			var got []row
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				got = append(got, row{f.Name, f.Type.String(), f.Tag.Get("json")})
			}
			assert.Equal(t, tc.rows, got, rt.String())
		}
	})
	t.Run("methods", func(t *testing.T) {
		for _, v := range []any{tg.Community{}, tg.CommunityChatAdded{}, tg.CommunityChatRemoved{}, tg.BotSubscriptionUpdated{}} {
			for _, rt := range []reflect.Type{reflect.TypeOf(v), reflect.PointerTo(reflect.TypeOf(v))} {
				var names []string
				for i := 0; i < rt.NumMethod(); i++ {
					names = append(names, rt.Method(i).Name)
				}
				assert.Empty(t, names, "exported methods of %s", rt)
			}
		}
	})
	t.Run("holders", func(t *testing.T) {
		for _, tc := range []struct {
			v               any
			field, typ, tag string
		}{
			{tg.Message{}, "CommunityChatAdded", "*tg.CommunityChatAdded", "community_chat_added,omitempty"},
			{tg.Message{}, "CommunityChatRemoved", "*tg.CommunityChatRemoved", "community_chat_removed,omitempty"},
			{tg.ChatFullInfo{}, "Community", "*tg.Community", "community,omitempty"},
			{tg.Update{}, "Subscription", "*tg.BotSubscriptionUpdated", "subscription,omitempty"},
		} {
			f, ok := reflect.TypeOf(tc.v).FieldByName(tc.field)
			if assert.True(t, ok, "%T.%s", tc.v, tc.field) {
				assert.Equal(t, tc.typ, f.Type.String(), "%T.%s", tc.v, tc.field)
				assert.Equal(t, tc.tag, f.Tag.Get("json"), "%T.%s", tc.v, tc.field)
			}
		}
	})
}

// T-own: walk every struct type reachable from the decode roots (generated, not a list) and check that the new
// keys live only on their planned holders, so a duplicate on a near neighbour (Chat, Message) fails.
func TestAPI102A_KeyOwners(t *testing.T) {
	watched := map[string]bool{"community_chat_added": true, "community_chat_removed": true, "subscription": true, "community": true}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{}), reflect.TypeOf(tg.ChatFullInfo{})}
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
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if key, _, _ := strings.Cut(f.Tag.Get("json"), ","); watched[key] {
				owners = append(owners, rt.Name()+"."+f.Name)
			}
			queue = append(queue, f.Type)
		}
	}
	sort.Strings(owners)
	assert.Equal(t, []string{
		"ChatFullInfo.Community", "CommunityChatAdded.Community", "Message.CommunityChatAdded",
		"Message.CommunityChatRemoved", "Update.Subscription",
	}, owners)
	for _, v := range []any{tg.Message{}, tg.Chat{}, tg.ChatFullInfo{}, tg.Update{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
