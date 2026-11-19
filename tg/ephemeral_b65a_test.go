package tg_test

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-65 slice 65a: tg.EphemeralMessageParameters (Bot API 10.3/10.4). Rows of E65/TEST-PLAN.md §2.1.

// T-type: the three fields, their Go types and JSON tags, nothing else.
func TestB65A_TypeShape(t *testing.T) {
	type field struct{ name, typ, tag string }
	rt := reflect.TypeOf(tg.EphemeralMessageParameters{})
	var got []field
	for i := range rt.NumField() {
		f := rt.Field(i)
		assert.False(t, f.Anonymous, "%s is embedded", f.Name)
		got = append(got, field{f.Name, f.Type.String(), f.Tag.Get("json")})
	}
	assert.Equal(t, []field{
		{"ReceiverUserID", "int64", "receiver_user_id,omitempty"},
		{"CallbackQueryID", "string", "callback_query_id,omitempty"},
		{"ReplaceCallbackQueryMessage", "bool", "replace_callback_query_message,omitempty"},
	}, got)
}

// T-json: exact JSON text both ways (exact digits for the widths, E-0178).
func TestB65A_TypeJSON(t *testing.T) {
	type row struct {
		name string
		v    tg.EphemeralMessageParameters
		want string
	}
	rows := []row{
		{"zero", tg.EphemeralMessageParameters{}, `{}`},
		{"callback", tg.EphemeralMessageParameters{CallbackQueryID: "cq"}, `{"callback_query_id":"cq"}`},
		{"callback_replace", tg.EphemeralMessageParameters{CallbackQueryID: "cq", ReplaceCallbackQueryMessage: true},
			`{"callback_query_id":"cq","replace_callback_query_message":true}`},
		{"all", tg.EphemeralMessageParameters{ReceiverUserID: 7, CallbackQueryID: "cq", ReplaceCallbackQueryMessage: true},
			`{"receiver_user_id":7,"callback_query_id":"cq","replace_callback_query_message":true}`},
	}
	for _, w := range []int64{1<<31 + 1, 1<<32 + 1, 1<<53 + 1, math.MaxInt64} {
		rows = append(rows, row{"receiver_" + strconv.FormatInt(w, 10), tg.EphemeralMessageParameters{ReceiverUserID: w},
			`{"receiver_user_id":` + strconv.FormatInt(w, 10) + `}`})
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(b))
			var back tg.EphemeralMessageParameters
			require.NoError(t, json.Unmarshal([]byte(r.want), &back))
			assert.Equal(t, r.v, back)
		})
	}
}

// T-val: Validate refuses parameters with neither ReceiverUserID nor CallbackQueryID; nil is valid.
func TestB65A_Validate(t *testing.T) {
	var nilParams *tg.EphemeralMessageParameters
	assert.NoError(t, nilParams.Validate(), "nil: not an ephemeral message")
	for _, p := range []tg.EphemeralMessageParameters{{}, {ReplaceCallbackQueryMessage: true}} {
		err := p.Validate()
		var ve *tg.ValidationError
		require.True(t, errors.As(err, &ve), "%+v: want *tg.ValidationError, got %T: %v", p, err, err)
		assert.Equal(t, "ephemeral_message_parameters", ve.Field)
	}
	for _, p := range []tg.EphemeralMessageParameters{
		{ReceiverUserID: 5},
		{ReceiverUserID: -1}, // the server checks the value (PLAN AC2)
		{CallbackQueryID: "cq"},
		{CallbackQueryID: " "}, // not trimmed: the server decides (E-0170, register O1-08)
		{ReceiverUserID: 5, CallbackQueryID: "cq"},
		{ReceiverUserID: 5, ReplaceCallbackQueryMessage: true},
	} {
		assert.NoError(t, p.Validate(), "%+v", p)
	}
}
