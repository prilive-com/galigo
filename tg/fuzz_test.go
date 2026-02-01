package tg

import (
	"encoding/json"
	"testing"
)

// FuzzUnmarshalTransactionPartner fuzzes the polymorphic TransactionPartner unmarshaler.
// Targets panic/crash on malformed JSON — the function should never panic.
func FuzzUnmarshalTransactionPartner(f *testing.F) {
	// Seeds: valid types, unknown type, edge cases
	f.Add([]byte(`{"type":"user","user":{"id":1,"is_bot":false,"first_name":"A"}}`))
	f.Add([]byte(`{"type":"fragment","withdrawal_state":{"type":"pending"}}`))
	f.Add([]byte(`{"type":"telegram_ads"}`))
	f.Add([]byte(`{"type":"telegram_api","request_count":1}`))
	f.Add([]byte(`{"type":"other"}`))
	f.Add([]byte(`{"type":"unknown_future"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic
		_ = unmarshalTransactionPartner(json.RawMessage(data))
	})
}

// FuzzUnmarshalChatBoostSource fuzzes the polymorphic ChatBoostSource unmarshaler.
func FuzzUnmarshalChatBoostSource(f *testing.F) {
	f.Add([]byte(`{"source":"premium","user":{"id":1,"is_bot":false,"first_name":"A"}}`))
	f.Add([]byte(`{"source":"gift_code","user":{"id":1,"is_bot":false,"first_name":"A"}}`))
	f.Add([]byte(`{"source":"giveaway","giveaway_message_id":1}`))
	f.Add([]byte(`{"source":"unknown"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = unmarshalChatBoostSource(json.RawMessage(data))
	})
}

// FuzzUnmarshalRevenueWithdrawalState fuzzes the polymorphic RevenueWithdrawalState unmarshaler.
func FuzzUnmarshalRevenueWithdrawalState(f *testing.F) {
	f.Add([]byte(`{"type":"pending"}`))
	f.Add([]byte(`{"type":"succeeded","date":1700000000,"url":"https://example.com"}`))
	f.Add([]byte(`{"type":"failed"}`))
	f.Add([]byte(`{"type":"unknown"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = unmarshalRevenueWithdrawalState(json.RawMessage(data))
	})
}

// FuzzChatBoostUnmarshalJSON fuzzes the full ChatBoost UnmarshalJSON path.
func FuzzChatBoostUnmarshalJSON(f *testing.F) {
	f.Add([]byte(`{"boost_id":"b1","add_date":1,"expiration_date":2,"source":{"source":"premium","user":{"id":1,"is_bot":false,"first_name":"A"}}}`))
	f.Add([]byte(`{"boost_id":"b2","source":null}`))
	f.Add([]byte(`{"boost_id":"b3"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var boost ChatBoost
		_ = json.Unmarshal(data, &boost)
	})
}

// FuzzStarTransactionUnmarshalJSON fuzzes the full StarTransaction UnmarshalJSON path.
func FuzzStarTransactionUnmarshalJSON(f *testing.F) {
	f.Add([]byte(`{"id":"tx1","amount":100,"date":1,"source":{"type":"user","user":{"id":1,"is_bot":false,"first_name":"A"}}}`))
	f.Add([]byte(`{"id":"tx2","amount":0,"date":0,"source":null,"receiver":null}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var tx StarTransaction
		_ = json.Unmarshal(data, &tx)
	})
}
