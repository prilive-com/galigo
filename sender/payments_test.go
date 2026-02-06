package sender_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== SendInvoice ====================

func TestSendInvoice(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendInvoice", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"message_id": 1,
			"chat":       map[string]any{"id": int64(123), "type": "private"},
			"date":       1700000000,
			"text":       "",
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendInvoice(context.Background(), sender.SendInvoiceRequest{
		ChatID:      int64(123),
		Title:       "Test Product",
		Description: "A test product",
		Payload:     "test_payload",
		Currency:    "XTR",
		Prices:      []tg.LabeledPrice{{Label: "Price", Amount: 100}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestSendInvoice_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.SendInvoiceRequest
		want string
	}{
		{
			name: "missing chat_id",
			req:  sender.SendInvoiceRequest{Title: "T", Description: "D", Payload: "P", Currency: "XTR", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "chat_id",
		},
		{
			name: "missing title",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Description: "D", Payload: "P", Currency: "XTR", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "title",
		},
		{
			name: "title too long",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Title: "AAAAAAAAAABBBBBBBBBBCCCCCCCCCCDDD", Description: "D", Payload: "P", Currency: "XTR", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "title",
		},
		{
			name: "missing description",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Title: "T", Payload: "P", Currency: "XTR", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "description",
		},
		{
			name: "missing payload",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Title: "T", Description: "D", Currency: "XTR", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "payload",
		},
		{
			name: "missing currency",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Title: "T", Description: "D", Payload: "P", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}},
			want: "currency",
		},
		{
			name: "missing prices",
			req:  sender.SendInvoiceRequest{ChatID: int64(1), Title: "T", Description: "D", Payload: "P", Currency: "XTR"},
			want: "prices",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.SendInvoice(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Equal(t, 0, server.CaptureCount(), "validation should fail before HTTP call")
		})
	}
}

// ==================== CreateInvoiceLink ====================

func TestCreateInvoiceLink(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/createInvoiceLink", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, "https://t.me/invoice/abc123")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	link, err := client.CreateInvoiceLink(context.Background(), sender.CreateInvoiceLinkRequest{
		Title:       "Test",
		Description: "Test desc",
		Payload:     "test",
		Currency:    "XTR",
		Prices:      []tg.LabeledPrice{{Label: "Price", Amount: 100}},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://t.me/invoice/abc123", link)
}

func TestCreateInvoiceLink_Validation_MissingTitle(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.CreateInvoiceLink(context.Background(), sender.CreateInvoiceLinkRequest{
		Description: "D", Payload: "P", Currency: "XTR",
		Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title")
}

// ==================== AnswerShippingQuery ====================

func TestAnswerShippingQuery_OK(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerShippingQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerShippingQuery(context.Background(), sender.AnswerShippingQueryRequest{
		ShippingQueryID: "sq123",
		OK:              true,
		ShippingOptions: []tg.ShippingOption{{ID: "1", Title: "Standard", Prices: []tg.LabeledPrice{{Label: "Shipping", Amount: 500}}}},
	})
	require.NoError(t, err)
}

func TestAnswerShippingQuery_Error(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerShippingQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerShippingQuery(context.Background(), sender.AnswerShippingQueryRequest{
		ShippingQueryID: "sq123",
		OK:              false,
		ErrorMessage:    "Cannot ship to this address",
	})
	require.NoError(t, err)
}

func TestAnswerShippingQuery_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.AnswerShippingQueryRequest
		want string
	}{
		{
			name: "missing shipping_query_id",
			req:  sender.AnswerShippingQueryRequest{OK: true, ShippingOptions: []tg.ShippingOption{{ID: "1", Title: "S", Prices: []tg.LabeledPrice{{Label: "L", Amount: 1}}}}},
			want: "shipping_query_id",
		},
		{
			name: "ok=true but no options",
			req:  sender.AnswerShippingQueryRequest{ShippingQueryID: "sq123", OK: true},
			want: "shipping_options",
		},
		{
			name: "ok=false but no error",
			req:  sender.AnswerShippingQueryRequest{ShippingQueryID: "sq123", OK: false},
			want: "error_message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AnswerShippingQuery(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== AnswerPreCheckoutQuery ====================

func TestAnswerPreCheckoutQuery_OK(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerPreCheckoutQuery", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerPreCheckoutQuery(context.Background(), sender.AnswerPreCheckoutQueryRequest{
		PreCheckoutQueryID: "pcq123",
		OK:                 true,
	})
	require.NoError(t, err)
}

func TestAnswerPreCheckoutQuery_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.AnswerPreCheckoutQueryRequest
		want string
	}{
		{
			name: "missing pre_checkout_query_id",
			req:  sender.AnswerPreCheckoutQueryRequest{OK: true},
			want: "pre_checkout_query_id",
		},
		{
			name: "ok=false but no error",
			req:  sender.AnswerPreCheckoutQueryRequest{PreCheckoutQueryID: "pcq123", OK: false},
			want: "error_message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AnswerPreCheckoutQuery(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== RefundStarPayment ====================

func TestRefundStarPayment(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/refundStarPayment", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.RefundStarPayment(context.Background(), sender.RefundStarPaymentRequest{
		UserID:                  123,
		TelegramPaymentChargeID: "tpci_abc",
	})
	require.NoError(t, err)
}

func TestRefundStarPayment_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name string
		req  sender.RefundStarPaymentRequest
		want string
	}{
		{
			name: "missing user_id",
			req:  sender.RefundStarPaymentRequest{TelegramPaymentChargeID: "abc"},
			want: "user_id",
		},
		{
			name: "missing charge_id",
			req:  sender.RefundStarPaymentRequest{UserID: 123},
			want: "telegram_payment_charge_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.RefundStarPayment(context.Background(), tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

// ==================== GetStarTransactions ====================

func TestGetStarTransactions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getStarTransactions", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"transactions": []map[string]any{
				{"id": "tx_1", "amount": 100, "date": 1700000000},
			},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.GetStarTransactions(context.Background(), sender.GetStarTransactionsRequest{Limit: 10})
	require.NoError(t, err)
	require.Len(t, result.Transactions, 1)
	assert.Equal(t, "tx_1", result.Transactions[0].ID)
	assert.Equal(t, 100, result.Transactions[0].Amount)
}

func TestGetStarTransactions_Validation_InvalidLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	_, err := client.GetStarTransactions(context.Background(), sender.GetStarTransactionsRequest{Limit: 200})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "limit")
}

func TestGetStarTransactions_DefaultLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getStarTransactions", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"transactions": []map[string]any{}})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	// Limit=0 means default — should not error
	_, err := client.GetStarTransactions(context.Background(), sender.GetStarTransactionsRequest{})
	require.NoError(t, err)
}

// ==================== GetMyStarBalance ====================

func TestGetMyStarBalance(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyStarBalance", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{"amount": 500, "nanostar_amount": 42})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	balance, err := client.GetMyStarBalance(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 500, balance.Amount)
	assert.Equal(t, 42, balance.NanostarAmount)
}

// ==================== Error handling ====================

func TestSendInvoice_APIError(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendInvoice", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyBadRequest(w, "Bad Request: PAYMENT_PROVIDER_INVALID")
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.SendInvoice(context.Background(), sender.SendInvoiceRequest{
		ChatID:      int64(123),
		Title:       "T",
		Description: "D",
		Payload:     "P",
		Currency:    "XTR",
		Prices:      []tg.LabeledPrice{{Label: "L", Amount: 1}},
	})
	require.Error(t, err)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.Code)
}
