package tg

import "encoding/json"

// UserChatBoosts represents a list of boosts added to a chat by a user.
type UserChatBoosts struct {
	Boosts []ChatBoost `json:"boosts"`
}

// ChatBoost represents a boost added to a chat.
type ChatBoost struct {
	BoostID        string          `json:"boost_id"`
	AddDate        int64           `json:"add_date"`
	ExpirationDate int64           `json:"expiration_date"`
	Source         ChatBoostSource `json:"source"`
}

// UnmarshalJSON handles the polymorphic Source field.
func (b *ChatBoost) UnmarshalJSON(data []byte) error {
	type Alias ChatBoost
	aux := &struct {
		Source json.RawMessage `json:"source"`
		*Alias
	}{Alias: (*Alias)(b)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Source) == 0 || string(aux.Source) == "null" {
		b.Source = ChatBoostSourceUnknown{Raw: aux.Source}
	} else {
		b.Source = unmarshalChatBoostSource(aux.Source)
	}
	return nil
}

// --- ChatBoostSource Union ---

// ChatBoostSource describes the source of a chat boost.
type ChatBoostSource interface {
	chatBoostSourceTag()
	GetSource() string
}

// ChatBoostSourcePremium represents a boost from a Premium subscriber.
type ChatBoostSourcePremium struct {
	Source string `json:"source"` // Always "premium"
	User   User   `json:"user"`
}

func (ChatBoostSourcePremium) chatBoostSourceTag()        {}
func (ChatBoostSourcePremium) GetSource() string          { return "premium" }

// ChatBoostSourceGiftCode represents a boost from a gift code.
type ChatBoostSourceGiftCode struct {
	Source string `json:"source"` // Always "gift_code"
	User   User   `json:"user"`
}

func (ChatBoostSourceGiftCode) chatBoostSourceTag()        {}
func (ChatBoostSourceGiftCode) GetSource() string          { return "gift_code" }

// ChatBoostSourceGiveaway represents a boost from a giveaway.
type ChatBoostSourceGiveaway struct {
	Source            string `json:"source"` // Always "giveaway"
	GiveawayMessageID int    `json:"giveaway_message_id"`
	User              *User  `json:"user,omitempty"`
	PrizeStarCount    int    `json:"prize_star_count,omitempty"`
	IsUnclaimed       bool   `json:"is_unclaimed,omitempty"`
}

func (ChatBoostSourceGiveaway) chatBoostSourceTag()        {}
func (ChatBoostSourceGiveaway) GetSource() string          { return "giveaway" }

// ChatBoostSourceUnknown is a fallback for future boost source types.
type ChatBoostSourceUnknown struct {
	Source string          `json:"source"`
	Raw    json.RawMessage `json:"-"`
}

func (ChatBoostSourceUnknown) chatBoostSourceTag()        {}
func (s ChatBoostSourceUnknown) GetSource() string        { return s.Source }

// unmarshalChatBoostSource decodes a ChatBoostSource from JSON.
// Returns ChatBoostSourceUnknown on any error.
func unmarshalChatBoostSource(data json.RawMessage) ChatBoostSource {
	var probe struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return ChatBoostSourceUnknown{Raw: data}
	}

	switch probe.Source {
	case "premium":
		var s ChatBoostSourcePremium
		if err := json.Unmarshal(data, &s); err != nil {
			return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
		}
		return s
	case "gift_code":
		var s ChatBoostSourceGiftCode
		if err := json.Unmarshal(data, &s); err != nil {
			return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
		}
		return s
	case "giveaway":
		var s ChatBoostSourceGiveaway
		if err := json.Unmarshal(data, &s); err != nil {
			return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
		}
		return s
	default:
		return ChatBoostSourceUnknown{Source: probe.Source, Raw: data}
	}
}
