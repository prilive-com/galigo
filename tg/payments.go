package tg

import "encoding/json"

// LabeledPrice represents a portion of the price for goods or services.
type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int    `json:"amount"` // Smallest currency unit (cents, etc.)
}

// ShippingOption represents one shipping option.
type ShippingOption struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Prices []LabeledPrice `json:"prices"`
}

// SuccessfulPayment contains information about a successful payment.
type SuccessfulPayment struct {
	Currency                   string     `json:"currency"`
	TotalAmount                int        `json:"total_amount"`
	InvoicePayload             string     `json:"invoice_payload"`
	ShippingOptionID           string     `json:"shipping_option_id,omitempty"`
	OrderInfo                  *OrderInfo `json:"order_info,omitempty"`
	TelegramPaymentChargeID    string     `json:"telegram_payment_charge_id"`
	ProviderPaymentChargeID    string     `json:"provider_payment_charge_id"`
	SubscriptionExpirationDate int64      `json:"subscription_expiration_date,omitempty"`
	IsRecurring                bool       `json:"is_recurring,omitempty"`
	IsFirstRecurring           bool       `json:"is_first_recurring,omitempty"`
}

// StarAmount represents an amount of Telegram Stars.
type StarAmount struct {
	Amount         int `json:"amount"`
	NanostarAmount int `json:"nanostar_amount,omitempty"`
}

// StarTransactions contains a list of Star transactions.
type StarTransactions struct {
	Transactions []StarTransaction `json:"transactions"`
}

// StarTransaction describes a Telegram Star transaction.
type StarTransaction struct {
	ID             string             `json:"id"`
	Amount         int                `json:"amount"`
	NanostarAmount int                `json:"nanostar_amount,omitempty"`
	Date           int64              `json:"date"`
	Source         TransactionPartner `json:"source,omitempty"`
	Receiver       TransactionPartner `json:"receiver,omitempty"`
}

// UnmarshalJSON handles polymorphic Source/Receiver fields.
func (s *StarTransaction) UnmarshalJSON(data []byte) error {
	type Alias StarTransaction
	aux := &struct {
		Source   json.RawMessage `json:"source,omitempty"`
		Receiver json.RawMessage `json:"receiver,omitempty"`
		*Alias
	}{Alias: (*Alias)(s)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Source) > 0 && string(aux.Source) != "null" {
		s.Source = unmarshalTransactionPartner(aux.Source)
	}
	if len(aux.Receiver) > 0 && string(aux.Receiver) != "null" {
		s.Receiver = unmarshalTransactionPartner(aux.Receiver)
	}
	return nil
}

// --- TransactionPartner Union ---

// TransactionPartner describes the source or receiver of a Star transaction.
type TransactionPartner interface {
	transactionPartnerTag()
}

// TransactionPartnerUser represents a transaction with a user.
type TransactionPartnerUser struct {
	Type               string `json:"type"` // Always "user"
	User               User   `json:"user"`
	InvoicePayload     string `json:"invoice_payload,omitempty"`
	PaidMediaPayload   string `json:"paid_media_payload,omitempty"`
	SubscriptionPeriod int    `json:"subscription_period,omitempty"`
	Gift               *Gift  `json:"gift,omitempty"`
}

func (TransactionPartnerUser) transactionPartnerTag() {}

// TransactionPartnerFragment represents a withdrawal to Fragment.
type TransactionPartnerFragment struct {
	Type            string                 `json:"type"` // Always "fragment"
	WithdrawalState RevenueWithdrawalState `json:"withdrawal_state,omitempty"`
}

func (TransactionPartnerFragment) transactionPartnerTag() {}

// UnmarshalJSON handles the nested polymorphic WithdrawalState.
func (p *TransactionPartnerFragment) UnmarshalJSON(data []byte) error {
	type Alias TransactionPartnerFragment
	aux := &struct {
		WithdrawalState json.RawMessage `json:"withdrawal_state,omitempty"`
		*Alias
	}{Alias: (*Alias)(p)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.WithdrawalState) > 0 && string(aux.WithdrawalState) != "null" {
		p.WithdrawalState = unmarshalRevenueWithdrawalState(aux.WithdrawalState)
	}
	return nil
}

// TransactionPartnerTelegramAds represents a transfer to Telegram Ads.
type TransactionPartnerTelegramAds struct {
	Type string `json:"type"` // Always "telegram_ads"
}

func (TransactionPartnerTelegramAds) transactionPartnerTag() {}

// TransactionPartnerTelegramApi represents payment from Telegram API usage.
type TransactionPartnerTelegramApi struct {
	Type         string `json:"type"` // Always "telegram_api"
	RequestCount int    `json:"request_count"`
}

func (TransactionPartnerTelegramApi) transactionPartnerTag() {}

// TransactionPartnerOther represents an unknown transaction partner type.
type TransactionPartnerOther struct {
	Type string `json:"type"` // Always "other"
}

func (TransactionPartnerOther) transactionPartnerTag() {}

// TransactionPartnerUnknown is a fallback for future/unknown partner types.
type TransactionPartnerUnknown struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (TransactionPartnerUnknown) transactionPartnerTag() {}

// unmarshalTransactionPartner decodes a TransactionPartner from JSON.
// Returns TransactionPartnerUnknown on any error (including malformed known types).
func unmarshalTransactionPartner(data json.RawMessage) TransactionPartner {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return TransactionPartnerUnknown{Raw: data}
	}

	switch probe.Type {
	case "user":
		var p TransactionPartnerUser
		if err := json.Unmarshal(data, &p); err != nil {
			return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
		}
		return p
	case "fragment":
		var p TransactionPartnerFragment
		if err := json.Unmarshal(data, &p); err != nil {
			return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
		}
		return p
	case "telegram_ads":
		var p TransactionPartnerTelegramAds
		if err := json.Unmarshal(data, &p); err != nil {
			return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
		}
		return p
	case "telegram_api":
		var p TransactionPartnerTelegramApi
		if err := json.Unmarshal(data, &p); err != nil {
			return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
		}
		return p
	case "other":
		var p TransactionPartnerOther
		if err := json.Unmarshal(data, &p); err != nil {
			return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
		}
		return p
	default:
		return TransactionPartnerUnknown{Type: probe.Type, Raw: data}
	}
}

// --- RevenueWithdrawalState Union ---

// RevenueWithdrawalState describes the state of a revenue withdrawal.
type RevenueWithdrawalState interface {
	revenueWithdrawalStateTag()
}

// RevenueWithdrawalStatePending represents a pending withdrawal.
type RevenueWithdrawalStatePending struct {
	Type string `json:"type"` // Always "pending"
}

func (RevenueWithdrawalStatePending) revenueWithdrawalStateTag() {}

// RevenueWithdrawalStateSucceeded represents a successful withdrawal.
type RevenueWithdrawalStateSucceeded struct {
	Type string `json:"type"` // Always "succeeded"
	Date int64  `json:"date"`
	URL  string `json:"url"`
}

func (RevenueWithdrawalStateSucceeded) revenueWithdrawalStateTag() {}

// RevenueWithdrawalStateFailed represents a failed withdrawal.
type RevenueWithdrawalStateFailed struct {
	Type string `json:"type"` // Always "failed"
}

func (RevenueWithdrawalStateFailed) revenueWithdrawalStateTag() {}

// RevenueWithdrawalStateUnknown is a fallback for future withdrawal states.
type RevenueWithdrawalStateUnknown struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (RevenueWithdrawalStateUnknown) revenueWithdrawalStateTag() {}

// unmarshalRevenueWithdrawalState decodes a RevenueWithdrawalState from JSON.
// Returns RevenueWithdrawalStateUnknown on any error.
func unmarshalRevenueWithdrawalState(data json.RawMessage) RevenueWithdrawalState {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return RevenueWithdrawalStateUnknown{Raw: data}
	}

	switch probe.Type {
	case "pending":
		var s RevenueWithdrawalStatePending
		if err := json.Unmarshal(data, &s); err != nil {
			return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
		}
		return s
	case "succeeded":
		var s RevenueWithdrawalStateSucceeded
		if err := json.Unmarshal(data, &s); err != nil {
			return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
		}
		return s
	case "failed":
		var s RevenueWithdrawalStateFailed
		if err := json.Unmarshal(data, &s); err != nil {
			return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
		}
		return s
	default:
		return RevenueWithdrawalStateUnknown{Type: probe.Type, Raw: data}
	}
}
