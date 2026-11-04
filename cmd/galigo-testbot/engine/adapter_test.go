package engine

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

const (
	adapterSecretPre  = "ABCdefGHI"
	adapterSecretPost = "jklMNOpqr"
)

var errAdapterBoom = errors.New("boom")

type adapterCountingTransport struct{ calls atomic.Int32 }

func (c *adapterCountingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errAdapterBoom
}

func requireAdapterScrubbed(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	msg := err.Error()
	assert.NotContains(t, msg, adapterSecretPre)
	assert.NotContains(t, msg, adapterSecretPost)
	assert.Contains(t, msg, "[REDACTED]")
	var urlErr *url.Error
	assert.True(t, errors.As(err, &urlErr), "error chain must keep *url.Error")
}

func TestGetUpdates_RequestCreationError_Scrubbed(t *testing.T) {
	for _, x := range []string{"%zz", "\x7f"} {
		t.Run(x, func(t *testing.T) {
			tok := tg.SecretToken("123456789:" + adapterSecretPre + x + adapterSecretPost)
			_, err := NewSenderAdapter(nil).WithToken(tok).GetUpdates(context.Background(), 0, 1, 0)
			requireAdapterScrubbed(t, err)
		})
	}
}

func TestGetUpdates_DoError_Scrubbed(t *testing.T) {
	for _, x := range []string{"", " "} {
		t.Run("suffix "+x, func(t *testing.T) {
			tok := tg.SecretToken("123456789:" + adapterSecretPre + "STUVWXYZabcdefgh" + adapterSecretPost + x)
			rt := &adapterCountingTransport{}
			_, err := NewSenderAdapter(nil).WithToken(tok).WithHTTPClient(&http.Client{Transport: rt}).
				GetUpdates(context.Background(), 0, 1, 0)
			requireAdapterScrubbed(t, err)
			assert.ErrorIs(t, err, errAdapterBoom)
			assert.Equal(t, int32(1), rt.calls.Load())
		})
	}
}

// T-tb-5 (B-25 slice 1, Bot API 9.6): the adapter passes reply parameters, a whole poll request and a prepared
// button to the sender, with exact bodies.
func TestSenderAdapter96(t *testing.T) {
	newAdapter := func(t *testing.T, path string, reply func(http.ResponseWriter)) (*SenderAdapter, *testutil.MockTelegramServer) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/"+path, func(w http.ResponseWriter, _ *http.Request) { reply(w) })
		return NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), server
	}
	msg := func(w http.ResponseWriter) { testutil.ReplyMessage(w, 4242) }

	t.Run("SendMessage with WithReplyParameters", func(t *testing.T) {
		a, server := newAdapter(t, "sendMessage", msg)
		_, err := a.SendMessage(context.Background(), testutil.TestChatID, "t",
			WithReplyParameters(&tg.ReplyParameters{MessageID: 5, PollOptionID: "pb"}))
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		assert.JSONEq(t, `{"chat_id":123456789,"text":"t","reply_parameters":{"message_id":5,"poll_option_id":"pb"}}`,
			server.LastCapture().BodyString())
	})
	t.Run("SendMessage without the option", func(t *testing.T) {
		a, server := newAdapter(t, "sendMessage", msg)
		_, err := a.SendMessage(context.Background(), testutil.TestChatID, "t")
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		assert.JSONEq(t, `{"chat_id":123456789,"text":"t"}`, server.LastCapture().BodyString())
	})
	t.Run("SendPoll with S48's whole request", func(t *testing.T) {
		a, server := newAdapter(t, "sendPoll", msg)
		f := false
		_, err := a.SendPoll(context.Background(), sender.SendPollRequest{
			ChatID: testutil.TestChatID, Question: "galigo 9.6 poll",
			Options:     []sender.InputPollOption{{Text: "A"}, {Text: "B"}, {Text: "C"}},
			IsAnonymous: &f, AllowsRevoting: &f, ShuffleOptions: true, AllowAddingOptions: true,
			HideResultsUntilCloses: true, Description: "galigo 9.6 description", OpenPeriod: 2628000,
		})
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		assert.JSONEq(t, `{"chat_id":123456789,"question":"galigo 9.6 poll","options":[{"text":"A"},{"text":"B"},{"text":"C"}],`+
			`"is_anonymous":false,"allows_revoting":false,"shuffle_options":true,"allow_adding_options":true,`+
			`"hide_results_until_closes":true,"description":"galigo 9.6 description","open_period":2628000}`,
			server.LastCapture().BodyString())
	})
	t.Run("SavePreparedKeyboardButton", func(t *testing.T) {
		a, server := newAdapter(t, "savePreparedKeyboardButton", func(w http.ResponseWriter) {
			testutil.ReplyOK(w, map[string]any{"id": "prep-btn-1"})
		})
		got, err := a.SavePreparedKeyboardButton(context.Background(), 42,
			tg.KeyboardButton{Text: "galigo 9.6 users", RequestUsers: &tg.KeyboardButtonRequestUsers{RequestID: 96}})
		require.NoError(t, err)
		assert.Equal(t, &tg.PreparedKeyboardButton{ID: "prep-btn-1"}, got)
		require.Equal(t, 1, server.CaptureCount())
		server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/savePreparedKeyboardButton")
		assert.JSONEq(t, `{"user_id":42,"button":{"text":"galigo 9.6 users","request_users":{"request_id":96}}}`,
			server.LastCapture().BodyString())
	})
}

// T-tb-5 (B-25 slice 2a, Bot API 10.0): the adapter passes the whole delete requests (actor_chat_id included)
// and calls getChatAdministrators with return_bots, with exact bodies.
func TestSenderAdapter100(t *testing.T) {
	newAdapter := func(t *testing.T, path string, reply func(http.ResponseWriter)) (*SenderAdapter, *testutil.MockTelegramServer) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/"+path, func(w http.ResponseWriter, _ *http.Request) { reply(w) })
		return NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), server
	}
	ok := func(w http.ResponseWriter) { testutil.ReplyBool(w, true) }

	t.Run("DeleteMessageReaction with actor_chat_id", func(t *testing.T) {
		// API100 T-tb-5
		a, server := newAdapter(t, "deleteMessageReaction", ok)
		err := a.DeleteMessageReaction(context.Background(), sender.DeleteMessageReactionRequest{
			ChatID: int64(-1001234567890), MessageID: 4242, UserID: 7001, ActorChatID: -1009876543210})
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/deleteMessageReaction")
		assert.JSONEq(t, `{"chat_id":-1001234567890,"message_id":4242,"user_id":7001,"actor_chat_id":-1009876543210}`,
			server.LastCapture().BodyString())
	})
	t.Run("DeleteAllMessageReactions with actor_chat_id", func(t *testing.T) {
		// API100 T-tb-5
		a, server := newAdapter(t, "deleteAllMessageReactions", ok)
		err := a.DeleteAllMessageReactions(context.Background(), sender.DeleteAllMessageReactionsRequest{
			ChatID: int64(-1001234567890), UserID: 7001, ActorChatID: -1009876543210})
		require.NoError(t, err)
		require.Equal(t, 1, server.CaptureCount())
		server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/deleteAllMessageReactions")
		assert.JSONEq(t, `{"chat_id":-1001234567890,"user_id":7001,"actor_chat_id":-1009876543210}`,
			server.LastCapture().BodyString())
	})
	t.Run("GetChatAdministratorsWithBots", func(t *testing.T) {
		// API100 T-tb-5
		a, server := newAdapter(t, "getChatAdministrators", func(w http.ResponseWriter) {
			testutil.ReplyOK(w, []map[string]any{{"status": "administrator",
				"user": map[string]any{"id": 7001, "is_bot": true, "first_name": "b"}}})
		})
		got, err := a.GetChatAdministratorsWithBots(context.Background(), -1001234567890)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, int64(7001), got[0].GetUser().ID)
		require.Equal(t, 1, server.CaptureCount())
		server.LastCapture().AssertPath(t, "/bot"+testutil.TestToken+"/getChatAdministrators")
		assert.JSONEq(t, `{"chat_id":-1001234567890,"return_bots":true}`, server.LastCapture().BodyString())
	})
}
