package tg_test

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 part b: ephemeral message fields (BotCommand.is_ephemeral, Message.receiver_user,
// Message.ephemeral_message_id, ReplyParameters.ephemeral_message_id; ReplyParameters.message_id is optional).
// IsEphemeral is never set in a Go literal and ids above 2^32 come from decoded JSON, so a wrong field type still
// compiles and fails by assertion. The helpers api102aMarshal and api102aKeys are 10.2a's (tg/api102a_test.go). Ids
// are fake.
const (
	api102bChat      = `"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}`
	api102bEphMsg    = `{"message_id":0,"date":1700000000,` + api102bChat + `,"receiver_user":{"id":987654321,"is_bot":false,"first_name":"Recv"},"ephemeral_message_id":5000000001}`
	api102bNormalMsg = `{"message_id":1001,"date":1700000000,` + api102bChat + `}`
	api102bNegMsg    = `{"message_id":0,"date":1,` + api102bChat + `,"ephemeral_message_id":-5}`
)

// T-msg-1: an ephemeral message decodes strictly; message_id 0 is kept on encode, both new keys round-trip.
func TestAPI102B_MessageEphemeral(t *testing.T) {
	var msg tg.Message
	require.NoError(t, api96Strict(t, []byte(api102bEphMsg), &msg))
	assert.Equal(t, 0, msg.MessageID)
	assert.Equal(t, 5000000001, msg.EphemeralMessageID)
	require.NotNil(t, msg.ReceiverUser)
	assert.Equal(t, int64(987654321), msg.ReceiverUser.ID)
	assert.Equal(t, "Recv", msg.ReceiverUser.FirstName)
	assert.False(t, msg.ReceiverUser.IsBot)
	assert.JSONEq(t, api102bEphMsg, api102aMarshal(t, msg))
}

// T-msg-2: a normal message has no new keys on encode; a negative ephemeral id is kept.
func TestAPI102B_MessageNormal(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		var msg tg.Message
		require.NoError(t, api96Strict(t, []byte(api102bNormalMsg), &msg))
		assert.Nil(t, msg.ReceiverUser)
		assert.Equal(t, 0, msg.EphemeralMessageID)
		m := api102aKeys(t, msg)
		assert.NotContains(t, m, "receiver_user")
		assert.NotContains(t, m, "ephemeral_message_id")
		assert.Equal(t, `1001`, string(m["message_id"]))
	})
	t.Run("negative", func(t *testing.T) {
		var msg tg.Message
		require.NoError(t, api96Strict(t, []byte(api102bNegMsg), &msg))
		assert.Equal(t, -5, msg.EphemeralMessageID)
		assert.JSONEq(t, api102bNegMsg, api102aMarshal(t, msg))
	})
}

// T-bc-1: is_ephemeral decodes strictly in its true, false and absent forms; false is omitted on encode.
func TestAPI102B_BotCommand(t *testing.T) {
	for _, tc := range []struct {
		name, in, out string
		want          bool
	}{
		{"true", `{"command":"a","description":"b","is_ephemeral":true}`, `{"command":"a","description":"b","is_ephemeral":true}`, true},
		{"false", `{"command":"a","description":"b","is_ephemeral":false}`, `{"command":"a","description":"b"}`, false},
		{"absent", `{"command":"a","description":"b"}`, `{"command":"a","description":"b"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bc tg.BotCommand
			require.NoError(t, api96Strict(t, []byte(tc.in), &bc))
			assert.Equal(t, tc.want, bc.IsEphemeral)
			assert.Equal(t, tc.out, api102aMarshal(t, bc))
		})
	}
}

// T-rp-1: ReplyParameters encodes message_id and ephemeral_message_id only when non-zero; negatives are kept.
func TestAPI102B_ReplyParametersEncode(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   tg.ReplyParameters
		want string
	}{
		{"ephemeral only", tg.ReplyParameters{EphemeralMessageID: 7}, `{"ephemeral_message_id":7}`},
		{"message only", tg.ReplyParameters{MessageID: 5}, `{"message_id":5}`},
		{"both", tg.ReplyParameters{MessageID: 5, EphemeralMessageID: 7}, `{"message_id":5,"ephemeral_message_id":7}`},
		{"zero", tg.ReplyParameters{}, `{}`},
		{"quote only", tg.ReplyParameters{Quote: "x"}, `{"quote":"x"}`},
		{"chat only", tg.ReplyParameters{ChatID: int64(1), AllowSendingWithoutReply: true}, `{"chat_id":1,"allow_sending_without_reply":true}`},
		{"negative message id", tg.ReplyParameters{MessageID: -1}, `{"message_id":-1}`},
		{"negative ephemeral id", tg.ReplyParameters{EphemeralMessageID: -1}, `{"ephemeral_message_id":-1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.JSONEq(t, tc.want, api102aMarshal(t, tc.in))
		})
	}
}

// T-rp-2: ReplyParameters decodes ephemeral_message_id strictly, above 2^32 too.
func TestAPI102B_ReplyParametersDecode(t *testing.T) {
	t.Run("ephemeral only", func(t *testing.T) {
		var rp tg.ReplyParameters
		require.NoError(t, api96Strict(t, []byte(`{"ephemeral_message_id":5000000001}`), &rp))
		assert.Equal(t, 5000000001, rp.EphemeralMessageID)
		assert.Equal(t, 0, rp.MessageID)
	})
	t.Run("both", func(t *testing.T) {
		var rp tg.ReplyParameters
		require.NoError(t, api96Strict(t, []byte(`{"message_id":5,"ephemeral_message_id":7}`), &rp))
		assert.Equal(t, 5, rp.MessageID)
		assert.Equal(t, 7, rp.EphemeralMessageID)
	})
}

// T-zero: the zero values omit every new key; Message keeps message_id 0.
func TestAPI102B_ZeroEncode(t *testing.T) {
	t.Run("BotCommand", func(t *testing.T) {
		assert.Equal(t, `{"command":"","description":""}`, api102aMarshal(t, tg.BotCommand{}))
	})
	t.Run("ReplyParameters", func(t *testing.T) {
		assert.Equal(t, `{}`, api102aMarshal(t, tg.ReplyParameters{}))
	})
	t.Run("Message", func(t *testing.T) {
		m := api102aKeys(t, tg.Message{})
		assert.Equal(t, `0`, string(m["message_id"]))
		assert.NotContains(t, m, "receiver_user")
		assert.NotContains(t, m, "ephemeral_message_id")
	})
}

// T-shape: each planned field exists with its type and tag, is not embedded, and the holders have exactly the
// planned number of fields (Message 62 at B-48b1, 69 at B-48c2; ReplyParameters 8 + 1; BotCommand 2 + 1).
func TestAPI102B_Shapes(t *testing.T) {
	for _, tc := range []struct {
		v               any
		field, typ, tag string
	}{
		{tg.BotCommand{}, "IsEphemeral", "bool", "is_ephemeral,omitempty"},
		{tg.Message{}, "ReceiverUser", "*tg.User", "receiver_user,omitempty"},
		{tg.Message{}, "EphemeralMessageID", "int", "ephemeral_message_id,omitempty"},
		{tg.ReplyParameters{}, "EphemeralMessageID", "int", "ephemeral_message_id,omitempty"},
		{tg.ReplyParameters{}, "MessageID", "int", "message_id,omitempty"},
		{tg.Message{}, "MessageID", "int", "message_id"},
	} {
		t.Run(fmt.Sprintf("%T.%s", tc.v, tc.field), func(t *testing.T) {
			f, ok := reflect.TypeOf(tc.v).FieldByName(tc.field)
			if assert.True(t, ok, "%T.%s", tc.v, tc.field) {
				assert.False(t, f.Anonymous, "%T.%s must be a named field, not embedded", tc.v, tc.field)
				assert.Equal(t, tc.typ, f.Type.String(), "%T.%s", tc.v, tc.field)
				assert.Equal(t, tc.tag, f.Tag.Get("json"), "%T.%s", tc.v, tc.field)
			}
		})
	}
	for _, tc := range []struct {
		v    any
		want int
	}{{tg.BotCommand{}, 3}, {tg.ReplyParameters{}, 9}, {tg.Message{}, 69}} {
		t.Run(fmt.Sprintf("NumField %T", tc.v), func(t *testing.T) {
			assert.Equal(t, tc.want, reflect.TypeOf(tc.v).NumField())
		})
	}
}

// T-own: walk every struct type reachable from the roots (generated, not a list) and check that the new keys live
// only on their planned holders, so a duplicate on a near neighbour (CallbackQuery, MessageEntity) fails. The walk
// stops at interface fields: an exported field added to a concrete type behind one is caught by G-api.
func TestAPI102B_KeyOwners(t *testing.T) {
	watched := map[string]bool{"is_ephemeral": true, "receiver_user": true, "ephemeral_message_id": true}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{}), reflect.TypeOf(tg.BotCommand{}), reflect.TypeOf(tg.ReplyParameters{})}
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
		"BotCommand.IsEphemeral", "Message.EphemeralMessageID", "Message.ReceiverUser", "ReplyParameters.EphemeralMessageID",
	}, owners)
	for _, v := range []any{tg.Update{}, tg.Message{}, tg.User{}, tg.CallbackQuery{}, tg.MessageEntity{}, tg.BotCommand{}, tg.ReplyParameters{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
