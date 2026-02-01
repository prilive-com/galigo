package sender

import (
	"context"

	"github.com/prilive-com/galigo/tg"
)

// ================== Payment Request Types ==================

// SendInvoiceRequest represents a sendInvoice request.
type SendInvoiceRequest struct {
	ChatID                    any               `json:"chat_id"`
	MessageThreadID           int               `json:"message_thread_id,omitempty"`
	Title                     string            `json:"title"`
	Description               string            `json:"description"`
	Payload                   string            `json:"payload"`
	ProviderToken             string            `json:"provider_token,omitempty"`
	Currency                  string            `json:"currency"`
	Prices                    []tg.LabeledPrice `json:"prices"`
	MaxTipAmount              int               `json:"max_tip_amount,omitempty"`
	SuggestedTipAmounts       []int             `json:"suggested_tip_amounts,omitempty"`
	StartParameter            string            `json:"start_parameter,omitempty"`
	ProviderData              string            `json:"provider_data,omitempty"`
	PhotoURL                  string            `json:"photo_url,omitempty"`
	PhotoSize                 int               `json:"photo_size,omitempty"`
	PhotoWidth                int               `json:"photo_width,omitempty"`
	PhotoHeight               int               `json:"photo_height,omitempty"`
	NeedName                  bool              `json:"need_name,omitempty"`
	NeedPhoneNumber           bool              `json:"need_phone_number,omitempty"`
	NeedEmail                 bool              `json:"need_email,omitempty"`
	NeedShippingAddress       bool              `json:"need_shipping_address,omitempty"`
	SendPhoneNumberToProvider bool              `json:"send_phone_number_to_provider,omitempty"`
	SendEmailToProvider       bool              `json:"send_email_to_provider,omitempty"`
	IsFlexible                bool              `json:"is_flexible,omitempty"`
	DisableNotification       bool              `json:"disable_notification,omitempty"`
	ProtectContent            bool              `json:"protect_content,omitempty"`
	ReplyParameters           *tg.ReplyParameters      `json:"reply_parameters,omitempty"`
	ReplyMarkup               *tg.InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// CreateInvoiceLinkRequest represents a createInvoiceLink request.
type CreateInvoiceLinkRequest struct {
	Title               string            `json:"title"`
	Description         string            `json:"description"`
	Payload             string            `json:"payload"`
	ProviderToken       string            `json:"provider_token,omitempty"`
	Currency            string            `json:"currency"`
	Prices              []tg.LabeledPrice `json:"prices"`
	MaxTipAmount        int               `json:"max_tip_amount,omitempty"`
	SuggestedTipAmounts []int             `json:"suggested_tip_amounts,omitempty"`
	ProviderData        string            `json:"provider_data,omitempty"`
	PhotoURL            string            `json:"photo_url,omitempty"`
	PhotoSize           int               `json:"photo_size,omitempty"`
	PhotoWidth          int               `json:"photo_width,omitempty"`
	PhotoHeight         int               `json:"photo_height,omitempty"`
	NeedName            bool              `json:"need_name,omitempty"`
	NeedPhoneNumber     bool              `json:"need_phone_number,omitempty"`
	NeedEmail           bool              `json:"need_email,omitempty"`
	NeedShippingAddress bool              `json:"need_shipping_address,omitempty"`
	IsFlexible          bool              `json:"is_flexible,omitempty"`
	SubscriptionPeriod  int               `json:"subscription_period,omitempty"`
}

// AnswerShippingQueryRequest represents an answerShippingQuery request.
type AnswerShippingQueryRequest struct {
	ShippingQueryID string              `json:"shipping_query_id"`
	OK              bool                `json:"ok"`
	ShippingOptions []tg.ShippingOption `json:"shipping_options,omitempty"`
	ErrorMessage    string              `json:"error_message,omitempty"`
}

// AnswerPreCheckoutQueryRequest represents an answerPreCheckoutQuery request.
type AnswerPreCheckoutQueryRequest struct {
	PreCheckoutQueryID string `json:"pre_checkout_query_id"`
	OK                 bool   `json:"ok"`
	ErrorMessage       string `json:"error_message,omitempty"`
}

// RefundStarPaymentRequest represents a refundStarPayment request.
type RefundStarPaymentRequest struct {
	UserID                  int64  `json:"user_id"`
	TelegramPaymentChargeID string `json:"telegram_payment_charge_id"`
}

// GetStarTransactionsRequest represents a getStarTransactions request.
type GetStarTransactionsRequest struct {
	Offset int `json:"offset,omitempty"`
	Limit  int `json:"limit,omitempty"`
}

// ================== Payment Methods ==================

// SendInvoice sends an invoice.
func (c *Client) SendInvoice(ctx context.Context, req SendInvoiceRequest) (*tg.Message, error) {
	if err := validateChatID(req.ChatID); err != nil {
		return nil, err
	}
	if req.Title == "" {
		return nil, tg.NewValidationError("title", "required")
	}
	if len(req.Title) > 32 {
		return nil, tg.NewValidationError("title", "must be 1-32 characters")
	}
	if req.Description == "" {
		return nil, tg.NewValidationError("description", "required")
	}
	if len(req.Description) > 255 {
		return nil, tg.NewValidationError("description", "must be 1-255 characters")
	}
	if req.Payload == "" {
		return nil, tg.NewValidationError("payload", "required")
	}
	if len(req.Payload) > 128 {
		return nil, tg.NewValidationError("payload", "must be 1-128 bytes")
	}
	if req.Currency == "" {
		return nil, tg.NewValidationError("currency", "required")
	}
	if len(req.Prices) == 0 {
		return nil, tg.NewValidationError("prices", "at least one price required")
	}

	var result tg.Message
	if err := c.callJSON(ctx, "sendInvoice", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateInvoiceLink creates a link for an invoice.
func (c *Client) CreateInvoiceLink(ctx context.Context, req CreateInvoiceLinkRequest) (string, error) {
	if req.Title == "" {
		return "", tg.NewValidationError("title", "required")
	}
	if req.Description == "" {
		return "", tg.NewValidationError("description", "required")
	}
	if req.Payload == "" {
		return "", tg.NewValidationError("payload", "required")
	}
	if req.Currency == "" {
		return "", tg.NewValidationError("currency", "required")
	}
	if len(req.Prices) == 0 {
		return "", tg.NewValidationError("prices", "at least one price required")
	}

	var result string
	if err := c.callJSON(ctx, "createInvoiceLink", req, &result); err != nil {
		return "", err
	}
	return result, nil
}

// AnswerShippingQuery responds to a shipping query.
func (c *Client) AnswerShippingQuery(ctx context.Context, req AnswerShippingQueryRequest) error {
	if req.ShippingQueryID == "" {
		return tg.NewValidationError("shipping_query_id", "required")
	}
	if req.OK && len(req.ShippingOptions) == 0 {
		return tg.NewValidationError("shipping_options", "required when ok is true")
	}
	if !req.OK && req.ErrorMessage == "" {
		return tg.NewValidationError("error_message", "required when ok is false")
	}

	return c.callJSON(ctx, "answerShippingQuery", req, nil)
}

// AnswerPreCheckoutQuery responds to a pre-checkout query.
// Must be called within 10 seconds. NO RETRY — value operation.
func (c *Client) AnswerPreCheckoutQuery(ctx context.Context, req AnswerPreCheckoutQueryRequest) error {
	if req.PreCheckoutQueryID == "" {
		return tg.NewValidationError("pre_checkout_query_id", "required")
	}
	if !req.OK && req.ErrorMessage == "" {
		return tg.NewValidationError("error_message", "required when ok is false")
	}

	// NO RETRY — value operation (double-answer would fail)
	return c.callJSON(ctx, "answerPreCheckoutQuery", req, nil)
}

// RefundStarPayment refunds a Telegram Stars payment.
// NO RETRY — value operation to prevent double-refund.
func (c *Client) RefundStarPayment(ctx context.Context, req RefundStarPaymentRequest) error {
	if req.UserID <= 0 {
		return tg.NewValidationError("user_id", "must be positive")
	}
	if req.TelegramPaymentChargeID == "" {
		return tg.NewValidationError("telegram_payment_charge_id", "required")
	}

	// NO RETRY — value operation
	return c.callJSON(ctx, "refundStarPayment", req, nil)
}

// GetStarTransactions returns the bot's Star transactions.
func (c *Client) GetStarTransactions(ctx context.Context, req GetStarTransactionsRequest) (*tg.StarTransactions, error) {
	if req.Limit != 0 && (req.Limit < 1 || req.Limit > 100) {
		return nil, tg.NewValidationError("limit", "must be 1-100")
	}

	var result tg.StarTransactions
	if err := c.callJSON(ctx, "getStarTransactions", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetMyStarBalance returns the bot's current Star balance.
func (c *Client) GetMyStarBalance(ctx context.Context) (*tg.StarAmount, error) {
	var result tg.StarAmount
	if err := c.callJSON(ctx, "getMyStarBalance", struct{}{}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
