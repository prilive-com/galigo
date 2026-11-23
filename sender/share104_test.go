package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65h: getChatShareThemeBackdrops and the new ChatFullInfo holders on the sender path (TEST-PLAN section 3).
// Ids and addresses are fake.

const b65hBackdropsWire = `{"backdrops":[{"id":0,"name":"Black","colors":{"center_color":1,"edge_color":2,"symbol_color":0,"text_color":16772551}},` +
	`{"id":79,"name":"","colors":{"center_color":16777215,"edge_color":3,"symbol_color":4,"text_color":5}}]}`

const b65hPath = "/bot" + testutil.TestToken + "/getChatShareThemeBackdrops"

// S-call: one POST with the body {}, and every id, name and color of the literal wire JSON decoded.
func TestB65H_GetChatShareThemeBackdrops(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(b65hBackdropsWire)) })
	got, err := testutil.NewTestClient(t, server.BaseURL()).GetChatShareThemeBackdrops(context.Background())
	require.NoError(t, err)
	assert.Equal(t, &tg.ChatShareThemeBackdrops{Backdrops: []tg.ChatShareThemeBackdrop{
		{ID: 0, Name: "Black", Colors: tg.UniqueGiftBackdropColors{CenterColor: 1, EdgeColor: 2, SymbolColor: 0, TextColor: 16772551}},
		{ID: 79, Name: "", Colors: tg.UniqueGiftBackdropColors{CenterColor: 16777215, EdgeColor: 3, SymbolColor: 4, TextColor: 5}},
	}}, got)
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	c.AssertPath(t, b65hPath)
	c.AssertMethod(t, http.MethodPost)
	c.AssertContentType(t, "application/json")
	assert.JSONEq(t, `{}`, c.BodyString())
}

// S-null: a null or empty object result is an empty success; an empty list stays a non-nil empty list (A-P1-08).
func TestB65H_BackdropsEmptyResults(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		nonNil       bool
	}{{"null", `null`, false}, {"empty_object", `{}`, false}, {"empty_list", `{"backdrops":[]}`, true}} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(tc.result)) })
			got, err := testutil.NewTestClient(t, server.BaseURL()).GetChatShareThemeBackdrops(context.Background())
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Empty(t, got.Backdrops)
			assert.Equal(t, tc.nonNil, got.Backdrops != nil)
		})
	}
}

// S-apierr: an API error is a *tg.APIError naming the method, with one request and no sleep, also when the client
// would retry (NewTestClient sets 0 retries; this client has 2).
func TestB65H_BackdropsAPIError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  int
		reply func(http.ResponseWriter)
	}{
		{"400", 400, func(w http.ResponseWriter) { testutil.ReplyBadRequest(w, "x") }},
		{"429", 429, func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }},
		{"500", 500, func(w http.ResponseWriter) { testutil.ReplyServerError(w, 500, "Internal Server Error") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { tc.reply(w) })
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(2))
			got, err := client.GetChatShareThemeBackdrops(context.Background())
			assert.Nil(t, got)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "%T: %v", err, err)
			assert.Equal(t, "getChatShareThemeBackdrops", apiErr.Method)
			assert.Equal(t, tc.code, apiErr.Code)
			assert.Equal(t, 1, server.CaptureCount(), "no retry")
			assert.Equal(t, 0, sleeper.CallCount(), "no sleep")
			assert.NotErrorIs(t, err, gobreaker.ErrOpenState) // A-T3-02: the error is not rewrapped
			assert.NotErrorIs(t, err, sender.ErrCircuitOpen)
		})
	}
}

// S-badresult: a result of the wrong shape is a decode error, not an empty success (O-T1-09).
func TestB65H_BackdropsBadResult(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(`{"backdrops":{}}`)) })
	got, err := testutil.NewTestClient(t, server.BaseURL()).GetChatShareThemeBackdrops(context.Background())
	assert.Nil(t, got)
	require.Error(t, err)
	var apiErr *tg.APIError
	assert.False(t, errors.As(err, &apiErr), "not an API error")
	var ute *json.UnmarshalTypeError
	require.True(t, errors.As(err, &ute), "%T: %v", err, err)
	assert.Equal(t, "backdrops", ute.Field)
}

// S-ctx: a canceled context stops the call before any request.
func TestB65H_BackdropsCanceled(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(b65hBackdropsWire)) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := testutil.NewTestClient(t, server.BaseURL()).GetChatShareThemeBackdrops(ctx)
	assert.Nil(t, got)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, server.CaptureCount())
}

// S-ratelimit: the getter goes through the global limiter: one token, the next in 1000 s, so the second call is refused
// before a request (O-T2-06; as sender/ratelimit_b51_test.go).
func TestB65H_BackdropsRateLimited(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(b65hBackdropsWire)) })
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0.001, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.GetChatShareThemeBackdrops(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	got, err := client.GetChatShareThemeBackdrops(ctx)
	assert.Nil(t, got)
	assert.EqualError(t, err, b51Refused) // A-T3-03
	assert.Equal(t, 1, server.CaptureCount(), "the second call sends nothing")
}

// S-nochat: the call has no chat id, so a spent per-chat limiter does not stop it (PLAN section 4, A-T3-04).
func TestB65H_BackdropsNoChatLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, json.RawMessage(b65hBackdropsWire)) })
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for i := range 2 {
		_, err := client.GetChatShareThemeBackdrops(ctx)
		require.NoError(t, err, "call %d", i+1)
	}
	assert.Equal(t, 2, server.CaptureCount())
}

// S-breaker: the getter goes through the circuit breaker: after two 500s the third call is gobreaker's ErrOpenState
// with no request (O-T2-06; A-T3-01: callJSON returns the breaker error as it is, as sender/ratelimit_b51_test.go R12).
func TestB65H_BackdropsBreaker(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65hPath, func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})
	client := testutil.NewBreakerTestClient(t, server.BaseURL())
	for i := range 2 {
		_, err := client.GetChatShareThemeBackdrops(context.Background())
		var apiErr *tg.APIError
		require.True(t, errors.As(err, &apiErr), "call %d: %T: %v", i, err, err)
		assert.Equal(t, 500, apiErr.Code)
		assert.NotErrorIs(t, err, gobreaker.ErrOpenState, "call %d", i)
		assert.NotErrorIs(t, err, sender.ErrCircuitOpen, "call %d", i)
	}
	got, err := client.GetChatShareThemeBackdrops(context.Background())
	assert.Nil(t, got)
	assert.ErrorIs(t, err, gobreaker.ErrOpenState)
	assert.Equal(t, 2, server.CaptureCount())
}

// S-getchat: GetChat decodes the new holders, and a share_theme of the wrong JSON type is a parse error (PLAN §4).
func TestB65H_GetChatHolders(t *testing.T) {
	const head = `{"id":77,"type":"private","accent_color_id":0,"max_reaction_count":0`
	t.Run("holders", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyOK(w, json.RawMessage(head+`,"ton_wallet_card":{"address":"EQfakeCard1","amount":"1000","stickers":[]},`+
				`"share_theme":{"type":"emoji","emojis":[{"type":"emoji","emoji":"🙂"}],"background_backdrop_id":0}}`))
		})
		chat, err := testutil.NewTestClient(t, server.BaseURL()).GetChat(context.Background(), int64(77))
		require.NoError(t, err)
		zero := 0
		assert.Equal(t, &tg.TonWalletCard{Address: "EQfakeCard1", Amount: "1000", Stickers: []tg.TonWalletCardSticker{}}, chat.TonWalletCard)
		assert.Equal(t, &tg.ChatShareTheme{Type: "emoji", Emojis: []tg.EmojiType{{Type: "emoji", Emoji: "🙂"}}, BackgroundBackdropID: &zero}, chat.ShareTheme)
	})
	t.Run("bad_share_theme", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On("/bot"+testutil.TestToken+"/getChat", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyOK(w, json.RawMessage(head+`,"share_theme":[]}`))
		})
		chat, err := testutil.NewTestClient(t, server.BaseURL()).GetChat(context.Background(), int64(77))
		assert.Nil(t, chat)
		require.Error(t, err)
		var apiErr *tg.APIError
		assert.False(t, errors.As(err, &apiErr), "not an API error")
		var ute *json.UnmarshalTypeError
		require.True(t, errors.As(err, &ute), "%T: %v", err, err)
		assert.Equal(t, "share_theme", ute.Field)
	})
}
