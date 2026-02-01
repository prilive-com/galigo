package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalChatBoostSource_Premium(t *testing.T) {
	data := `{"source":"premium","user":{"id":123,"is_bot":false,"first_name":"Alice"}}`
	result := unmarshalChatBoostSource(json.RawMessage(data))

	s, ok := result.(ChatBoostSourcePremium)
	require.True(t, ok)
	assert.Equal(t, "premium", s.GetSource())
	assert.Equal(t, int64(123), s.User.ID)
}

func TestUnmarshalChatBoostSource_GiftCode(t *testing.T) {
	data := `{"source":"gift_code","user":{"id":456,"is_bot":false,"first_name":"Bob"}}`
	result := unmarshalChatBoostSource(json.RawMessage(data))

	s, ok := result.(ChatBoostSourceGiftCode)
	require.True(t, ok)
	assert.Equal(t, "gift_code", s.GetSource())
}

func TestUnmarshalChatBoostSource_Giveaway(t *testing.T) {
	data := `{"source":"giveaway","giveaway_message_id":99,"is_unclaimed":true}`
	result := unmarshalChatBoostSource(json.RawMessage(data))

	s, ok := result.(ChatBoostSourceGiveaway)
	require.True(t, ok)
	assert.Equal(t, 99, s.GiveawayMessageID)
	assert.True(t, s.IsUnclaimed)
	assert.Nil(t, s.User)
}

func TestUnmarshalChatBoostSource_Unknown(t *testing.T) {
	data := `{"source":"future_type","extra":true}`
	result := unmarshalChatBoostSource(json.RawMessage(data))

	unknown, ok := result.(ChatBoostSourceUnknown)
	require.True(t, ok)
	assert.Equal(t, "future_type", unknown.GetSource())
	assert.NotEmpty(t, unknown.Raw)
}

func TestUnmarshalChatBoostSource_MalformedKnown(t *testing.T) {
	data := `{"source":"premium","user":"not_object"}`
	result := unmarshalChatBoostSource(json.RawMessage(data))

	unknown, ok := result.(ChatBoostSourceUnknown)
	require.True(t, ok)
	assert.Equal(t, "premium", unknown.Source)
}

func TestChatBoost_UnmarshalJSON(t *testing.T) {
	data := `{
		"boost_id": "b123",
		"add_date": 1700000000,
		"expiration_date": 1700100000,
		"source": {"source":"premium","user":{"id":1,"is_bot":false,"first_name":"X"}}
	}`

	var boost ChatBoost
	err := json.Unmarshal([]byte(data), &boost)
	require.NoError(t, err)

	assert.Equal(t, "b123", boost.BoostID)
	_, ok := boost.Source.(ChatBoostSourcePremium)
	assert.True(t, ok)
}
