package tg_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-48c2: the Bot API 9.2 suggested post read side (SuggestedPostInfo and the five service messages), the six
// Message fields and Message.IsPaidPost. No test builds a new type with a keyed composite literal (only zero values): values come from decoded
// JSON and are read by selectors, so a field mutant compiles and fails by assertion. Ids are fake. Fixtures hold only
// keys galigo models, so a strict decode tests the new keys. The helpers api96Strict
// (tg/api96_test.go), api102aMarshal and api102aKeys (tg/api102a_test.go) are reused.
const (
	b48c2Chat   = `"chat":{"id":-1001234567890,"type":"supergroup","title":"Test Chat"}`
	b48c2Post   = `{"message_id":42,"date":1700000000,` + b48c2Chat + `,"text":"post","suggested_post_info":{"state":"approved","price":{"currency":"XTR","amount":250}}}`
	b48c2BigTON = `{"currency":"TON","amount":10000000000000}`
)

// b48c2Decode decodes in strictly (DisallowUnknownFields) into v and checks that v re-encodes to the same JSON.
func b48c2Decode(t *testing.T, in string, v any) {
	t.Helper()
	require.NoError(t, api96Strict(t, []byte(in), v))
	assert.JSONEq(t, in, api102aMarshal(t, v))
}

// T1: SuggestedPostInfo.
func TestB48C2_Info(t *testing.T) {
	t.Run("full", func(t *testing.T) {
		var v tg.SuggestedPostInfo
		b48c2Decode(t, `{"state":"pending","price":{"currency":"XTR","amount":250},"send_date":1900000000}`, &v)
		assert.Equal(t, "pending", v.State)
		require.NotNil(t, v.Price)
		assert.Equal(t, "XTR", v.Price.Currency)
		assert.Equal(t, int64(250), v.Price.Amount)
		assert.Equal(t, int64(1900000000), v.SendDate)
	})
	t.Run("ton", func(t *testing.T) {
		var v tg.SuggestedPostInfo
		b48c2Decode(t, `{"state":"approved","price":`+b48c2BigTON+`}`, &v)
		assert.Equal(t, "approved", v.State)
		require.NotNil(t, v.Price)
		assert.Equal(t, "TON", v.Price.Currency)
		assert.Equal(t, int64(10000000000000), v.Price.Amount)
		assert.Zero(t, v.SendDate)
	})
	t.Run("unpaid", func(t *testing.T) {
		var v tg.SuggestedPostInfo
		b48c2Decode(t, `{"state":"declined"}`, &v)
		assert.Equal(t, "declined", v.State)
		assert.Nil(t, v.Price)
		assert.Zero(t, v.SendDate)
	})
	t.Run("future_state", func(t *testing.T) {
		var v tg.SuggestedPostInfo
		b48c2Decode(t, `{"state":"x_future"}`, &v)
		assert.Equal(t, "x_future", v.State)
	})
}

// b48c2CheckPost checks the nested suggested_post_message of a service message (b48c2Post).
func b48c2CheckPost(t *testing.T, m *tg.Message) {
	t.Helper()
	require.NotNil(t, m)
	assert.Equal(t, 42, m.MessageID)
	assert.Equal(t, "post", m.Text)
	require.NotNil(t, m.SuggestedPostInfo)
	assert.Equal(t, "approved", m.SuggestedPostInfo.State)
	require.NotNil(t, m.SuggestedPostInfo.Price)
	assert.Equal(t, int64(250), m.SuggestedPostInfo.Price.Amount)
}

// T2–T7: the five service messages.
func TestB48C2_Service(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		var v tg.SuggestedPostApproved
		b48c2Decode(t, `{"suggested_post_message":`+b48c2Post+`,"price":`+b48c2BigTON+`,"send_date":1900000000}`, &v)
		b48c2CheckPost(t, v.SuggestedPostMessage)
		require.NotNil(t, v.Price)
		assert.Equal(t, "TON", v.Price.Currency)
		assert.Equal(t, int64(10000000000000), v.Price.Amount)
		assert.Equal(t, int64(1900000000), v.SendDate)
	})
	t.Run("approved_minimal", func(t *testing.T) {
		var v tg.SuggestedPostApproved
		b48c2Decode(t, `{"send_date":1900000000}`, &v)
		assert.Nil(t, v.SuggestedPostMessage)
		assert.Nil(t, v.Price)
		assert.Equal(t, int64(1900000000), v.SendDate)
	})
	t.Run("approval_failed", func(t *testing.T) {
		var v tg.SuggestedPostApprovalFailed
		b48c2Decode(t, `{"suggested_post_message":`+b48c2Post+`,"price":{"currency":"XTR","amount":100000}}`, &v)
		b48c2CheckPost(t, v.SuggestedPostMessage)
		assert.Equal(t, "XTR", v.Price.Currency)
		assert.Equal(t, int64(100000), v.Price.Amount)
	})
	t.Run("declined", func(t *testing.T) {
		var v tg.SuggestedPostDeclined
		b48c2Decode(t, `{"suggested_post_message":`+b48c2Post+`,"comment":"слишком длинно 🙂"}`, &v)
		b48c2CheckPost(t, v.SuggestedPostMessage)
		assert.Equal(t, "слишком длинно 🙂", v.Comment)
	})
	t.Run("declined_no_comment", func(t *testing.T) {
		var v tg.SuggestedPostDeclined
		b48c2Decode(t, `{}`, &v)
		assert.Nil(t, v.SuggestedPostMessage)
		assert.Empty(t, v.Comment)
	})
	t.Run("paid_ton", func(t *testing.T) {
		var v tg.SuggestedPostPaid
		b48c2Decode(t, `{"suggested_post_message":`+b48c2Post+`,"currency":"TON","amount":10000000000000}`, &v)
		b48c2CheckPost(t, v.SuggestedPostMessage)
		assert.Equal(t, "TON", v.Currency)
		assert.Equal(t, int64(10000000000000), v.Amount)
		assert.Nil(t, v.StarAmount)
	})
	t.Run("paid_xtr", func(t *testing.T) {
		var v tg.SuggestedPostPaid
		b48c2Decode(t, `{"currency":"XTR","star_amount":{"amount":250,"nanostar_amount":5}}`, &v)
		assert.Nil(t, v.SuggestedPostMessage)
		assert.Equal(t, "XTR", v.Currency)
		assert.Zero(t, v.Amount)
		require.NotNil(t, v.StarAmount)
		assert.Equal(t, 250, v.StarAmount.Amount)
		assert.Equal(t, 5, v.StarAmount.NanostarAmount)
	})
	t.Run("refunded_post_deleted", func(t *testing.T) {
		var v tg.SuggestedPostRefunded
		b48c2Decode(t, `{"suggested_post_message":`+b48c2Post+`,"reason":"post_deleted"}`, &v)
		b48c2CheckPost(t, v.SuggestedPostMessage)
		assert.Equal(t, "post_deleted", v.Reason)
	})
	t.Run("refunded_payment_refunded", func(t *testing.T) {
		var v tg.SuggestedPostRefunded
		b48c2Decode(t, `{"reason":"payment_refunded"}`, &v)
		assert.Nil(t, v.SuggestedPostMessage)
		assert.Equal(t, "payment_refunded", v.Reason)
	})
}

// b48c2Holders reads the six new Message fields, in PLAN D2 order, as "set" or "nil".
func b48c2Holders(m *tg.Message) map[string]bool {
	return map[string]bool{
		"suggested_post_info":            m.SuggestedPostInfo != nil,
		"suggested_post_approved":        m.SuggestedPostApproved != nil,
		"suggested_post_approval_failed": m.SuggestedPostApprovalFailed != nil,
		"suggested_post_declined":        m.SuggestedPostDeclined != nil,
		"suggested_post_paid":            m.SuggestedPostPaid != nil,
		"suggested_post_refunded":        m.SuggestedPostRefunded != nil,
	}
}

// T8, T9: each Message key decodes into its own field only; a three-level nested message decodes.
func TestB48C2_Message(t *testing.T) {
	base := `{"message_id":43,"date":1700000001,` + b48c2Chat
	for _, tc := range []struct{ key, value string }{
		{"suggested_post_info", `{"state":"pending"}`},
		{"suggested_post_approved", `{"suggested_post_message":` + b48c2Post + `,"send_date":1900000000}`},
		{"suggested_post_approval_failed", `{"suggested_post_message":` + b48c2Post + `,"price":{"currency":"XTR","amount":5}}`},
		{"suggested_post_declined", `{"suggested_post_message":` + b48c2Post + `,"comment":"no"}`},
		{"suggested_post_paid", `{"suggested_post_message":` + b48c2Post + `,"currency":"XTR","star_amount":{"amount":5}}`},
		{"suggested_post_refunded", `{"suggested_post_message":` + b48c2Post + `,"reason":"post_deleted"}`},
	} {
		t.Run(tc.key, func(t *testing.T) {
			var m tg.Message
			b48c2Decode(t, base+`,"`+tc.key+`":`+tc.value+`}`, &m)
			for key, set := range b48c2Holders(&m) {
				assert.Equal(t, key == tc.key, set, "Message field for %s", key)
			}
		})
	}
	t.Run("values", func(t *testing.T) {
		var m tg.Message
		b48c2Decode(t, base+`,"suggested_post_approved":{"suggested_post_message":`+b48c2Post+`,"send_date":1900000000}}`, &m)
		require.NotNil(t, m.SuggestedPostApproved)
		b48c2CheckPost(t, m.SuggestedPostApproved.SuggestedPostMessage)
		assert.Equal(t, int64(1900000000), m.SuggestedPostApproved.SendDate)
	})
	t.Run("nested", func(t *testing.T) {
		inner := `{"message_id":41,"date":1700000000,` + b48c2Chat + `,"suggested_post_info":{"state":"pending","send_date":1900000001}}`
		mid := `{"message_id":42,"date":1700000000,` + b48c2Chat + `,"suggested_post_paid":{"suggested_post_message":` + inner + `,"currency":"XTR"}}`
		var m tg.Message
		b48c2Decode(t, base+`,"suggested_post_refunded":{"suggested_post_message":`+mid+`,"reason":"payment_refunded"}}`, &m)
		require.NotNil(t, m.SuggestedPostRefunded)
		require.NotNil(t, m.SuggestedPostRefunded.SuggestedPostMessage)
		require.NotNil(t, m.SuggestedPostRefunded.SuggestedPostMessage.SuggestedPostPaid)
		got := m.SuggestedPostRefunded.SuggestedPostMessage.SuggestedPostPaid.SuggestedPostMessage
		require.NotNil(t, got)
		assert.Equal(t, 41, got.MessageID)
		require.NotNil(t, got.SuggestedPostInfo)
		assert.Equal(t, "pending", got.SuggestedPostInfo.State)
		assert.Equal(t, int64(1900000001), got.SuggestedPostInfo.SendDate)
	})
}

// T8b: Message.IsPaidPost (9.2, "is_paid_post": True, optional) decodes; absent is false.
func TestB48C2_PaidPost(t *testing.T) {
	base := `{"message_id":43,"date":1700000001,` + b48c2Chat
	t.Run("true", func(t *testing.T) {
		var m tg.Message
		b48c2Decode(t, base+`,"text":"paid","is_paid_post":true}`, &m)
		assert.True(t, m.IsPaidPost)
		assert.Equal(t, "paid", m.Text)
	})
	t.Run("absent", func(t *testing.T) {
		var m tg.Message
		b48c2Decode(t, base+`,"text":"free"}`, &m)
		assert.False(t, m.IsPaidPost)
	})
}

// T10: a zero value encodes only the required keys; Message{} encodes none of the six keys and no is_paid_post.
func TestB48C2_ZeroEncode(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{tg.SuggestedPostInfo{}, `{"state":""}`},
		{tg.SuggestedPostApproved{}, `{"send_date":0}`},
		{tg.SuggestedPostApprovalFailed{}, `{"price":{"currency":"","amount":0}}`},
		{tg.SuggestedPostDeclined{}, `{}`},
		{tg.SuggestedPostPaid{}, `{"currency":""}`},
		{tg.SuggestedPostRefunded{}, `{"reason":""}`},
	} {
		assert.Equal(t, tc.want, api102aMarshal(t, tc.v), "%T", tc.v)
	}
	keys := api102aKeys(t, tg.Message{})
	for key := range b48c2Holders(&tg.Message{}) {
		assert.NotContains(t, keys, key)
	}
	assert.NotContains(t, keys, "is_paid_post")
}

// The spec key lists, in spec order (E29/lookups/api.html).
var b48c2SpecKeys = map[string][]string{
	"SuggestedPostInfo":           {"state", "price", "send_date"},
	"SuggestedPostApproved":       {"suggested_post_message", "price", "send_date"},
	"SuggestedPostApprovalFailed": {"suggested_post_message", "price"},
	"SuggestedPostDeclined":       {"suggested_post_message", "comment"},
	"SuggestedPostPaid":           {"suggested_post_message", "currency", "amount", "star_amount"},
	"SuggestedPostRefunded":       {"suggested_post_message", "reason"},
}

// T11, T12: each new type's fields, in order, equal the planned rows exactly (name, Go type, JSON tag, not embedded);
// the tag keys equal the spec key list; no methods. The six Message holders and IsPaidPost, and their places (PLAN D2).
func TestB48C2_Shapes(t *testing.T) {
	type row struct{ name, typ, tag string }
	msg := row{"SuggestedPostMessage", "*tg.Message", "suggested_post_message,omitempty"}
	shapes := []struct {
		v    any
		rows []row
	}{
		{tg.SuggestedPostInfo{}, []row{{"State", "string", "state"}, {"Price", "*tg.SuggestedPostPrice", "price,omitempty"}, {"SendDate", "int64", "send_date,omitempty"}}},
		{tg.SuggestedPostApproved{}, []row{msg, {"Price", "*tg.SuggestedPostPrice", "price,omitempty"}, {"SendDate", "int64", "send_date"}}},
		{tg.SuggestedPostApprovalFailed{}, []row{msg, {"Price", "tg.SuggestedPostPrice", "price"}}},
		{tg.SuggestedPostDeclined{}, []row{msg, {"Comment", "string", "comment,omitempty"}}},
		{tg.SuggestedPostPaid{}, []row{msg, {"Currency", "string", "currency"}, {"Amount", "int64", "amount,omitempty"}, {"StarAmount", "*tg.StarAmount", "star_amount,omitempty"}}},
		{tg.SuggestedPostRefunded{}, []row{msg, {"Reason", "string", "reason"}}},
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
			assert.Equal(t, b48c2SpecKeys[rt.Name()], keys, "the spec keys in spec order")
			assert.Equal(t, 0, rt.NumMethod(), "no value methods")
			assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod(), "no pointer methods")
		})
	}
	t.Run("message_holders", func(t *testing.T) {
		rt := reflect.TypeOf(tg.Message{})
		holders := []row{
			{"SuggestedPostInfo", "*tg.SuggestedPostInfo", "suggested_post_info,omitempty"},
			{"SuggestedPostApproved", "*tg.SuggestedPostApproved", "suggested_post_approved,omitempty"},
			{"SuggestedPostApprovalFailed", "*tg.SuggestedPostApprovalFailed", "suggested_post_approval_failed,omitempty"},
			{"SuggestedPostDeclined", "*tg.SuggestedPostDeclined", "suggested_post_declined,omitempty"},
			{"SuggestedPostPaid", "*tg.SuggestedPostPaid", "suggested_post_paid,omitempty"},
			{"SuggestedPostRefunded", "*tg.SuggestedPostRefunded", "suggested_post_refunded,omitempty"},
			{"IsPaidPost", "bool", "is_paid_post,omitempty"},
		}
		after := []string{"Entities", "GuestQueryID", "SuggestedPostApproved", "SuggestedPostApprovalFailed", "SuggestedPostDeclined", "SuggestedPostPaid",
			"HasProtectedContent"}
		for i, h := range holders {
			f, ok := rt.FieldByName(h.name)
			p, okp := rt.FieldByName(after[i])
			if assert.True(t, ok, "Message.%s", h.name) && assert.True(t, okp, "Message.%s", after[i]) {
				assert.False(t, f.Anonymous, "Message.%s must be a named field", h.name)
				assert.Equal(t, h, row{f.Name, f.Type.String(), f.Tag.Get("json")})
				assert.Equal(t, []int{p.Index[0] + 1}, f.Index, "Message.%s must follow %s", h.name, after[i])
			}
		}
		assert.Equal(t, "SuggestedPostRefunded", rt.Field(rt.NumField()-1).Name, "the last Message field")
	})
}

// T13: the six types stay comparable (no slice or map field).
func TestB48C2_Comparable(t *testing.T) {
	for _, v := range []any{tg.SuggestedPostInfo{}, tg.SuggestedPostApproved{}, tg.SuggestedPostApprovalFailed{},
		tg.SuggestedPostDeclined{}, tg.SuggestedPostPaid{}, tg.SuggestedPostRefunded{}} {
		assert.True(t, reflect.TypeOf(v).Comparable(), "%T", v)
	}
}

// T14: walk every struct type reachable from tg.Update and check that the new keys live only on their planned owners.
func TestB48C2_KeyOwners(t *testing.T) {
	watched := map[string]bool{"star_amount": true, "suggested_post_message": true, "is_paid_post": true}
	for key := range b48c2Holders(&tg.Message{}) {
		watched[key] = true
	}
	seen := map[reflect.Type]bool{}
	var owners []string
	queue := []reflect.Type{reflect.TypeOf(tg.Update{})}
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
		"Message.IsPaidPost", "Message.SuggestedPostApprovalFailed", "Message.SuggestedPostApproved", "Message.SuggestedPostDeclined",
		"Message.SuggestedPostInfo", "Message.SuggestedPostPaid", "Message.SuggestedPostRefunded",
		"SuggestedPostApprovalFailed.SuggestedPostMessage", "SuggestedPostApproved.SuggestedPostMessage",
		"SuggestedPostDeclined.SuggestedPostMessage", "SuggestedPostPaid.StarAmount", "SuggestedPostPaid.SuggestedPostMessage",
		"SuggestedPostRefunded.SuggestedPostMessage",
	}, owners)
	for _, v := range []any{tg.SuggestedPostInfo{}, tg.SuggestedPostApproved{}, tg.SuggestedPostApprovalFailed{},
		tg.SuggestedPostDeclined{}, tg.SuggestedPostPaid{}, tg.SuggestedPostRefunded{}} {
		assert.True(t, seen[reflect.TypeOf(v)], "the walk did not reach %T", v)
	}
}
