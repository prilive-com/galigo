package tg

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzUnmarshalTransactionPartner fuzzes the polymorphic TransactionPartner unmarshaler.
// Targets panic/crash on malformed JSON — the function should never panic.
func FuzzUnmarshalTransactionPartner(f *testing.F) {
	// Seeds: valid types, unknown type, edge cases
	f.Add([]byte(`{"type":"user","user":{"id":1,"is_bot":false,"first_name":"A"}}`))
	f.Add([]byte(`{"type":"user","user":{"id":1,"is_bot":false,"first_name":"A"},"paid_media":[{"type":"photo","photo":[]},{"type":"x","width":"w"}]}`))
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
	f.Add([]byte(`{"id":"tx3","amount":1,"date":1,"source":{"type":"user","user":{"id":1,"is_bot":false,"first_name":"A"},"paid_media":[{"type":"video","video":{"file_id":"v","file_unique_id":"u","width":1,"height":1,"duration":1}},{"type":"photo","photo":"x"}]}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{invalid`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var tx StarTransaction
		_ = json.Unmarshal(data, &tx)
	})
}

// B-145 (TEST-PLAN T16): the OwnedGift codec never panics; a value it decodes encodes and decodes again with the same
// Type and the same gift pointers set. A Marshal error is allowed only for the encoder's depth limit (nested rich
// message blocks). The two variant seeds decode into their own pointer.
func FuzzOwnedGiftUnmarshalJSON(f *testing.F) {
	sticker := `{"file_id":"f","file_unique_id":"u","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false}`
	reg := `{"type":"regular","gift":{"id":"g-1","sticker":` + sticker + `,"star_count":5},"owned_gift_id":"o","send_date":1,"entities":[{"type":"bold","offset":0,"length":1}],"is_private":true}`
	uni := `{"type":"unique","gift":{"gift_id":"u-1","base_name":"P","name":"P-9","number":9},"can_be_transferred":true,"next_transfer_date":4102444800}`
	f.Add([]byte(reg))
	f.Add([]byte(uni))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"type":"future"}`))
	f.Add([]byte(`{"gift":5}`))
	f.Add([]byte(`{"type":"regular","entities":[]}`))
	f.Add([]byte(`{"type":"regular","gift":{"id":"g-2","sticker":` + sticker + `,"star_count":1,"publisher_chat":{"id":-1,"type":"channel","active_usernames":[]}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var v OwnedGift
		if json.Unmarshal(data, &v) != nil {
			return
		}
		// The two variant seeds decode into their own pointer (B-145 O-T3-02).
		if string(data) == reg && (v.Gift == nil || v.UniqueGift != nil) {
			t.Fatalf("regular seed: Gift %v, UniqueGift %v", v.Gift, v.UniqueGift)
		}
		if string(data) == uni && (v.UniqueGift == nil || v.Gift != nil) {
			t.Fatalf("unique seed: Gift %v, UniqueGift %v", v.Gift, v.UniqueGift)
		}
		b1, err := json.Marshal(v)
		if err != nil {
			if !strings.Contains(err.Error(), "exceeded max depth") {
				t.Fatalf("Marshal after a successful decode: %v", err)
			}
			return
		}
		var v2 OwnedGift
		if err := json.Unmarshal(b1, &v2); err != nil {
			t.Fatalf("decode of %s: %v", b1, err)
		}
		if v2.Type != v.Type || (v2.Gift == nil) != (v.Gift == nil) || (v2.UniqueGift == nil) != (v.UniqueGift == nil) {
			t.Fatalf("round trip changed the variant: %+v -> %+v", v, v2)
		}
		// No byte rule: rich_message blocks re-encode differently on a second pass (B-145 O-T3-01).
	})
}
