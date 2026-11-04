package sender_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.0 reaction deletion and no-retry rows (B-25 slice 2a, TEST-PLAN §3, rows T-s-5 … T-s-8, T-s-14).

const api100MsgID = 4242

// api100Exact sends call against a mock that replies True and checks the path and the exact body.
func api100Exact(t *testing.T, method, body string, call func(*sender.Client) error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) {
		api96ReplyRaw(w, `{"ok":true,"result":true}`)
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	require.NoError(t, call(client))
	require.Equal(t, 1, server.CaptureCount())
	server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/"+method)
	assert.JSONEq(t, body, server.LastCapture().BodyString())
}

type api100InvalidRow struct {
	name  string
	field string // a *tg.ValidationError on this field; "" = the shared validateChatID plain error
	text  string // the error's whole text
}

// api100Invalid checks one validation row: the error type and text, and no request.
func api100Invalid(t *testing.T, r api100InvalidRow, call func(*sender.Client) error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	err := call(client)
	require.Error(t, err)
	var ve *tg.ValidationError
	if r.field == "" {
		assert.False(t, errors.As(err, &ve), "a chat id error is the shared plain error: %v", err)
	} else if assert.True(t, errors.As(err, &ve), "error %v", err) {
		assert.Equal(t, r.field, ve.Field)
	}
	assert.Equal(t, r.text, err.Error())
	assert.Equal(t, 0, server.CaptureCount())
}

func TestAPI100_DeleteMessageReaction(t *testing.T) {
	// API100 T-s-5
	rows := []struct {
		name string
		req  sender.DeleteMessageReactionRequest
		body string
	}{
		{"(a) user", sender.DeleteMessageReactionRequest{ChatID: api100Chat, MessageID: api100MsgID, UserID: 7001},
			`{"chat_id":-1001234567890,"message_id":4242,"user_id":7001}`},
		{"(b) actor", sender.DeleteMessageReactionRequest{ChatID: api100Chat, MessageID: api100MsgID, ActorChatID: api100ActorChat},
			`{"chat_id":-1001234567890,"message_id":4242,"actor_chat_id":-1009876543210}`},
		{"(c) both", sender.DeleteMessageReactionRequest{ChatID: api100Chat, MessageID: api100MsgID, UserID: 7001, ActorChatID: api100ActorChat},
			`{"chat_id":-1001234567890,"message_id":4242,"user_id":7001,"actor_chat_id":-1009876543210}`},
		{"(d) int chat id", sender.DeleteMessageReactionRequest{ChatID: int(-1001234567890), MessageID: api100MsgID, UserID: 7001},
			`{"chat_id":-1001234567890,"message_id":4242,"user_id":7001}`},
		{"(e) username chat id", sender.DeleteMessageReactionRequest{ChatID: "@galigo_test", MessageID: api100MsgID, UserID: 7001},
			`{"chat_id":"@galigo_test","message_id":4242,"user_id":7001}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api100Exact(t, "deleteMessageReaction", r.body, func(c *sender.Client) error {
				return c.DeleteMessageReaction(context.Background(), r.req)
			})
		})
	}
	t.Run("limiter (a)", func(t *testing.T) {
		api100LimiterRow(t, "deleteMessageReaction", "true", func(c *sender.Client, id tg.ChatID) error {
			return c.DeleteMessageReaction(context.Background(),
				sender.DeleteMessageReactionRequest{ChatID: id, MessageID: api100MsgID, UserID: 7001})
		})
	})
}

func TestAPI100_DeleteMessageReaction_Validation(t *testing.T) {
	// API100 T-s-6
	ok := sender.DeleteMessageReactionRequest{ChatID: api100Chat, MessageID: api100MsgID, UserID: 7001}
	with := func(f func(*sender.DeleteMessageReactionRequest)) sender.DeleteMessageReactionRequest {
		r := ok
		f(&r)
		return r
	}
	rows := []struct {
		api100InvalidRow
		req sender.DeleteMessageReactionRequest
	}{
		{api100InvalidRow{"chat nil", "", "galigo: chat_id is required"}, with(func(r *sender.DeleteMessageReactionRequest) { r.ChatID = nil })},
		{api100InvalidRow{"chat empty", "", "galigo: chat_id cannot be empty string"}, with(func(r *sender.DeleteMessageReactionRequest) { r.ChatID = "" })},
		{api100InvalidRow{"chat zero", "", "galigo: chat_id cannot be zero"}, with(func(r *sender.DeleteMessageReactionRequest) { r.ChatID = int64(0) })},
		{api100InvalidRow{"message 0", "message_id", "galigo: validation: message_id - must be positive"}, with(func(r *sender.DeleteMessageReactionRequest) { r.MessageID = 0 })},
		{api100InvalidRow{"message -1", "message_id", "galigo: validation: message_id - must be positive"}, with(func(r *sender.DeleteMessageReactionRequest) { r.MessageID = -1 })},
		{api100InvalidRow{"neither id", "user_id", "galigo: validation: user_id - user_id or actor_chat_id required"}, with(func(r *sender.DeleteMessageReactionRequest) { r.UserID = 0 })},
		{api100InvalidRow{"user -5", "user_id", "galigo: validation: user_id - must be positive"}, with(func(r *sender.DeleteMessageReactionRequest) { r.UserID = -5 })},
		{api100InvalidRow{"user -5 with actor", "user_id", "galigo: validation: user_id - must be positive"}, with(func(r *sender.DeleteMessageReactionRequest) { r.UserID = -5; r.ActorChatID = api100ActorChat })},
		{api100InvalidRow{"user 0 actor 0", "user_id", "galigo: validation: user_id - user_id or actor_chat_id required"}, with(func(r *sender.DeleteMessageReactionRequest) { r.UserID = 0; r.ActorChatID = 0 })},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api100Invalid(t, r.api100InvalidRow, func(c *sender.Client) error {
				return c.DeleteMessageReaction(context.Background(), r.req)
			})
		})
	}
}

func TestAPI100_DeleteAllMessageReactions(t *testing.T) {
	// API100 T-s-7
	rows := []struct {
		name string
		req  sender.DeleteAllMessageReactionsRequest
		body string
	}{
		{"(a) user", sender.DeleteAllMessageReactionsRequest{ChatID: api100Chat, UserID: 7001},
			`{"chat_id":-1001234567890,"user_id":7001}`},
		{"(b) actor", sender.DeleteAllMessageReactionsRequest{ChatID: api100Chat, ActorChatID: api100ActorChat},
			`{"chat_id":-1001234567890,"actor_chat_id":-1009876543210}`},
		{"(c) both", sender.DeleteAllMessageReactionsRequest{ChatID: api100Chat, UserID: 7001, ActorChatID: api100ActorChat},
			`{"chat_id":-1001234567890,"user_id":7001,"actor_chat_id":-1009876543210}`},
		{"(d) int chat id", sender.DeleteAllMessageReactionsRequest{ChatID: int(-1001234567890), UserID: 7001},
			`{"chat_id":-1001234567890,"user_id":7001}`},
		{"(e) username chat id", sender.DeleteAllMessageReactionsRequest{ChatID: "@galigo_test", UserID: 7001},
			`{"chat_id":"@galigo_test","user_id":7001}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api100Exact(t, "deleteAllMessageReactions", r.body, func(c *sender.Client) error {
				return c.DeleteAllMessageReactions(context.Background(), r.req)
			})
		})
	}
	t.Run("limiter (a)", func(t *testing.T) {
		api100LimiterRow(t, "deleteAllMessageReactions", "true", func(c *sender.Client, id tg.ChatID) error {
			return c.DeleteAllMessageReactions(context.Background(), sender.DeleteAllMessageReactionsRequest{ChatID: id, UserID: 7001})
		})
	})
}

func TestAPI100_DeleteAllMessageReactions_Validation(t *testing.T) {
	// API100 T-s-8
	ok := sender.DeleteAllMessageReactionsRequest{ChatID: api100Chat, UserID: 7001}
	with := func(f func(*sender.DeleteAllMessageReactionsRequest)) sender.DeleteAllMessageReactionsRequest {
		r := ok
		f(&r)
		return r
	}
	rows := []struct {
		api100InvalidRow
		req sender.DeleteAllMessageReactionsRequest
	}{
		{api100InvalidRow{"chat nil", "", "galigo: chat_id is required"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.ChatID = nil })},
		{api100InvalidRow{"chat empty", "", "galigo: chat_id cannot be empty string"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.ChatID = "" })},
		{api100InvalidRow{"chat zero", "", "galigo: chat_id cannot be zero"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.ChatID = int64(0) })},
		{api100InvalidRow{"neither id", "user_id", "galigo: validation: user_id - user_id or actor_chat_id required"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.UserID = 0 })},
		{api100InvalidRow{"user -5", "user_id", "galigo: validation: user_id - must be positive"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.UserID = -5 })},
		{api100InvalidRow{"user -5 with actor", "user_id", "galigo: validation: user_id - must be positive"}, with(func(r *sender.DeleteAllMessageReactionsRequest) { r.UserID = -5; r.ActorChatID = api100ActorChat })},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			api100Invalid(t, r.api100InvalidRow, func(c *sender.Client) error {
				return c.DeleteAllMessageReactions(context.Background(), r.req)
			})
		})
	}
}

// api100NewMethods are the seven new methods, each called with valid input, with the path it must use.
var api100NewMethods = []struct {
	method string
	call   func(*sender.Client) error
}{
	{"answerGuestQuery", func(c *sender.Client) error {
		_, err := c.AnswerGuestQuery(context.Background(), sender.AnswerGuestQueryRequest{GuestQueryID: "gq-100", Result: api100Article()})
		return err
	}},
	{"getChatAdministrators", func(c *sender.Client) error {
		_, err := c.GetChatAdministratorsWithBots(context.Background(), api100Chat)
		return err
	}},
	{"deleteMessageReaction", func(c *sender.Client) error {
		return c.DeleteMessageReaction(context.Background(), sender.DeleteMessageReactionRequest{ChatID: api100Chat, MessageID: api100MsgID, UserID: 7001})
	}},
	{"deleteAllMessageReactions", func(c *sender.Client) error {
		return c.DeleteAllMessageReactions(context.Background(), sender.DeleteAllMessageReactionsRequest{ChatID: api100Chat, UserID: 7001})
	}},
	{"getManagedBotAccessSettings", func(c *sender.Client) error {
		_, err := c.GetManagedBotAccessSettings(context.Background(), 42)
		return err
	}},
	{"setManagedBotAccessSettings", func(c *sender.Client) error {
		return c.SetManagedBotAccessSettings(context.Background(), sender.SetManagedBotAccessSettingsRequest{UserID: 42})
	}},
	{"getUserPersonalChatMessages", func(c *sender.Client) error {
		_, err := c.GetUserPersonalChatMessages(context.Background(), sender.GetUserPersonalChatMessagesRequest{UserID: 42, Limit: 5})
		return err
	}},
}

func TestAPI100_NoRetry(t *testing.T) {
	// API100 T-s-14: no new method retries (502 and 429); SendMessage on the same client is the control
	sets := []struct {
		name  string
		first func(http.ResponseWriter)
		check func(*testing.T, error)
	}{
		{"502", func(w http.ResponseWriter) { testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway") },
			func(t *testing.T, err error) { require.Error(t, err) }},
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) },
			func(t *testing.T, err error) {
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, 429, apiErr.Code)
			}},
	}
	for _, set := range sets {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			paths := append([]string{"sendMessage"}, func() []string {
				var p []string
				for _, m := range api100NewMethods {
					p = append(p, m.method)
				}
				return p
			}()...)
			for _, p := range paths {
				var n atomic.Int32
				server.On("/bot"+testutil.TestToken+"/"+p, func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					testutil.ReplyMessage(w, 4242)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))

			for _, m := range api100NewMethods {
				server.ResetCaptures()
				set.check(t, m.call(client))
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")

			// control, last: SendMessage is wrapped in withRetry
			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
			assert.GreaterOrEqual(t, sleeper.CallCount(), 1, "the sleeper is wired")
		})
	}
}
