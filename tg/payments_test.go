package tg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalTransactionPartner_User(t *testing.T) {
	data := `{"type":"user","user":{"id":123,"is_bot":false,"first_name":"Alice"}}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	p, ok := result.(TransactionPartnerUser)
	require.True(t, ok)
	assert.Equal(t, "user", p.Type)
	assert.Equal(t, int64(123), p.User.ID)
	assert.Equal(t, "Alice", p.User.FirstName)
}

func TestUnmarshalTransactionPartner_Fragment(t *testing.T) {
	data := `{"type":"fragment","withdrawal_state":{"type":"succeeded","date":1700000000,"url":"https://example.com"}}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	p, ok := result.(TransactionPartnerFragment)
	require.True(t, ok)
	assert.Equal(t, "fragment", p.Type)

	ws, ok := p.WithdrawalState.(RevenueWithdrawalStateSucceeded)
	require.True(t, ok)
	assert.Equal(t, int64(1700000000), ws.Date)
	assert.Equal(t, "https://example.com", ws.URL)
}

func TestUnmarshalTransactionPartner_TelegramAds(t *testing.T) {
	data := `{"type":"telegram_ads"}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	_, ok := result.(TransactionPartnerTelegramAds)
	assert.True(t, ok)
}

func TestUnmarshalTransactionPartner_TelegramApi(t *testing.T) {
	data := `{"type":"telegram_api","request_count":42}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	p, ok := result.(TransactionPartnerTelegramApi)
	require.True(t, ok)
	assert.Equal(t, 42, p.RequestCount)
}

func TestUnmarshalTransactionPartner_Other(t *testing.T) {
	data := `{"type":"other"}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	_, ok := result.(TransactionPartnerOther)
	assert.True(t, ok)
}

func TestUnmarshalTransactionPartner_FutureType(t *testing.T) {
	data := `{"type":"future_partner","new_field":"value"}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	unknown, ok := result.(TransactionPartnerUnknown)
	require.True(t, ok)
	assert.Equal(t, "future_partner", unknown.Type)
	assert.NotEmpty(t, unknown.Raw)
}

func TestUnmarshalTransactionPartner_MalformedKnownType(t *testing.T) {
	data := `{"type":"user","user":"not_an_object"}`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	unknown, ok := result.(TransactionPartnerUnknown)
	require.True(t, ok, "malformed known type should decode to Unknown")
	assert.Equal(t, "user", unknown.Type)
	assert.NotEmpty(t, unknown.Raw)
}

func TestUnmarshalTransactionPartner_InvalidJSON(t *testing.T) {
	data := `{invalid`
	result := unmarshalTransactionPartner(json.RawMessage(data))

	unknown, ok := result.(TransactionPartnerUnknown)
	require.True(t, ok)
	assert.NotEmpty(t, unknown.Raw)
}

func TestUnmarshalRevenueWithdrawalState_Pending(t *testing.T) {
	data := `{"type":"pending"}`
	result := unmarshalRevenueWithdrawalState(json.RawMessage(data))

	_, ok := result.(RevenueWithdrawalStatePending)
	assert.True(t, ok)
}

func TestUnmarshalRevenueWithdrawalState_Succeeded(t *testing.T) {
	data := `{"type":"succeeded","date":1700000000,"url":"https://example.com"}`
	result := unmarshalRevenueWithdrawalState(json.RawMessage(data))

	s, ok := result.(RevenueWithdrawalStateSucceeded)
	require.True(t, ok)
	assert.Equal(t, int64(1700000000), s.Date)
	assert.Equal(t, "https://example.com", s.URL)
}

func TestUnmarshalRevenueWithdrawalState_Failed(t *testing.T) {
	data := `{"type":"failed"}`
	result := unmarshalRevenueWithdrawalState(json.RawMessage(data))

	_, ok := result.(RevenueWithdrawalStateFailed)
	assert.True(t, ok)
}

func TestUnmarshalRevenueWithdrawalState_Unknown(t *testing.T) {
	data := `{"type":"new_state","extra":123}`
	result := unmarshalRevenueWithdrawalState(json.RawMessage(data))

	unknown, ok := result.(RevenueWithdrawalStateUnknown)
	require.True(t, ok)
	assert.Equal(t, "new_state", unknown.Type)
}

func TestStarTransaction_UnmarshalJSON(t *testing.T) {
	data := `{
		"id": "tx_123",
		"amount": 100,
		"date": 1700000000,
		"source": {"type":"user","user":{"id":1,"is_bot":false,"first_name":"Bob"}},
		"receiver": {"type":"telegram_ads"}
	}`

	var tx StarTransaction
	err := json.Unmarshal([]byte(data), &tx)
	require.NoError(t, err)

	assert.Equal(t, "tx_123", tx.ID)
	assert.Equal(t, 100, tx.Amount)

	src, ok := tx.Source.(TransactionPartnerUser)
	require.True(t, ok)
	assert.Equal(t, "Bob", src.User.FirstName)

	_, ok = tx.Receiver.(TransactionPartnerTelegramAds)
	assert.True(t, ok)
}

func TestStarTransaction_UnmarshalJSON_NullFields(t *testing.T) {
	data := `{"id":"tx_456","amount":50,"date":1700000000,"source":null}`

	var tx StarTransaction
	err := json.Unmarshal([]byte(data), &tx)
	require.NoError(t, err)

	assert.Nil(t, tx.Source)
	assert.Nil(t, tx.Receiver)
}
