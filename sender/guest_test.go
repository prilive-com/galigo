package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.0 guest mode (B-25 slice 2a, TEST-PLAN §3, rows T-s-1, T-s-2).

func api100Article() *tg.InlineQueryResultArticle {
	// Type is a stored field that encoding does not fill (tg/inline.go), so the test sets it
	return &tg.InlineQueryResultArticle{Type: "article", ID: "1", Title: "t",
		InputMessageContent: tg.InputTextMessageContent{MessageText: "x"}}
}

func TestAPI100_AnswerGuestQuery(t *testing.T) {
	// API100 T-s-1
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/answerGuestQuery", func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":{"inline_message_id":"im-100"}}`)
	})
	client := testutil.NewTestClient(t, server.BaseURL())

	got, err := client.AnswerGuestQuery(context.Background(), sender.AnswerGuestQueryRequest{
		GuestQueryID: "gq-100", Result: api100Article()})
	require.NoError(t, err)
	assert.Equal(t, &tg.SentGuestMessage{InlineMessageID: "im-100"}, got)
	require.Equal(t, 1, server.CaptureCount())
	server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/answerGuestQuery")
	assert.JSONEq(t, `{"guest_query_id":"gq-100","result":{"type":"article","id":"1","title":"t",`+
		`"input_message_content":{"message_text":"x"}}}`, server.LastCapture().BodyString())

	bad := testutil.NewMockServer(t)
	bad.On("/bot"+testutil.TestToken+"/answerGuestQuery", func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":"x"}`)
	})
	_, err = testutil.NewTestClient(t, bad.BaseURL()).AnswerGuestQuery(context.Background(),
		sender.AnswerGuestQueryRequest{GuestQueryID: "gq-100", Result: api100Article()})
	// a malformed result: callJSON's wrap names the method and keeps the decode error (A-C1-03)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "galigo: answerGuestQuery: failed to parse response: "), err.Error())
	var ute *json.UnmarshalTypeError
	assert.True(t, errors.As(err, &ute), "error %v", err)
}

func TestAPI100_AnswerGuestQuery_Validation(t *testing.T) {
	// API100 T-s-2
	rows := []struct {
		name  string
		req   sender.AnswerGuestQueryRequest
		field string
		text  string
	}{
		{"empty id", sender.AnswerGuestQueryRequest{Result: api100Article()}, "guest_query_id", "galigo: validation: guest_query_id - required"},
		{"nil result", sender.AnswerGuestQueryRequest{GuestQueryID: "gq-100"}, "result", "galigo: validation: result - required"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			_, err := client.AnswerGuestQuery(context.Background(), r.req)
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "error %v", err)
			assert.Equal(t, r.field, ve.Field)
			assert.Equal(t, r.text, err.Error()) // the whole text (A-C1-02)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}
