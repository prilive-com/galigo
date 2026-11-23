package sender_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65i: getUniqueGift and deleteBusinessAccountGift (Bot API 10.4; TEST-PLAN section 3). Fake ids only.

const (
	b65iModel  = `"model":{"name":"m","sticker":{"file_id":"f1","file_unique_id":"u1","type":"custom_emoji","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":1}`
	b65iSymbol = `"symbol":{"name":"s","sticker":{"file_id":"f2","file_unique_id":"u2","type":"custom_emoji","width":1,"height":1,"is_animated":false,"is_video":false},"rarity_per_mille":2}`
	b65iBack   = `"backdrop":{"name":"b","colors":{"center_color":1,"edge_color":2,"symbol_color":3,"text_color":4},"rarity_per_mille":3}`
	b65iHead   = `{"gift_id":"5936013938331222567","base_name":"Plush Pepe","name":"PlushPepe-1","number":1,` + b65iModel + `,` + b65iSymbol + `,` + b65iBack
	b65iUF     = b65iHead + `,"is_premium":true,"colors":{"model_custom_emoji_id":"me","symbol_custom_emoji_id":"se","light_theme_main_color":5,"light_theme_other_colors":[6],"dark_theme_main_color":7,"dark_theme_other_colors":[8]},"is_burned":true,"publisher_chat":{"id":-100777,"type":"channel","title":"pub"}}`
	b65iUB     = b65iHead + `,"is_from_blockchain":true}`
	b65iNoID   = "galigo: getUniqueGift: empty gift_id in response"
)

// b65iBase is the hand-written value of the fixtures' required part (not a decode of the same bytes).
func b65iBase() tg.UniqueGift {
	return tg.UniqueGift{GiftID: "5936013938331222567", BaseName: "Plush Pepe", Name: "PlushPepe-1", Number: 1,
		Model: tg.UniqueGiftModel{Name: "m", Sticker: tg.Sticker{FileID: "f1", FileUniqueID: "u1", Type: "custom_emoji",
			Width: 1, Height: 1}, RarityPerMille: 1},
		Symbol: tg.UniqueGiftSymbol{Name: "s", Sticker: tg.Sticker{FileID: "f2", FileUniqueID: "u2", Type: "custom_emoji",
			Width: 1, Height: 1}, RarityPerMille: 2},
		Backdrop: tg.UniqueGiftBackdrop{Name: "b", Colors: tg.UniqueGiftBackdropColors{CenterColor: 1, EdgeColor: 2,
			SymbolColor: 3, TextColor: 4}, RarityPerMille: 3}}
}

func b65iPath(method string) string { return "/bot" + testutil.TestToken + "/" + method }

func b65iFast() []sender.Option {
	return []sender.Option{sender.WithRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100)}
}

// b65iMethods are the two new methods, each called with valid input.
var b65iMethods = []struct {
	method string
	call   func(context.Context, *sender.Client) error
}{
	{"getUniqueGift", func(ctx context.Context, c *sender.Client) error {
		_, err := c.GetUniqueGift(ctx, "PlushPepe-1")
		return err
	}},
	{"deleteBusinessAccountGift", func(ctx context.Context, c *sender.Client) error {
		return c.DeleteBusinessAccountGift(ctx, sender.DeleteBusinessAccountGiftRequest{BusinessConnectionID: "bc-fake", OwnedGiftID: "og-fake"})
	}},
}

// S-ug-call: one JSON POST with the name as given; the whole result (every field of UF, then of UB) comes back.
func TestB65I_GetUniqueGiftCall(t *testing.T) {
	uf := b65iBase()
	uf.IsPremium = true
	uf.Colors = &tg.UniqueGiftColors{ModelCustomEmojiID: "me", SymbolCustomEmojiID: "se", LightThemeMainColor: 5,
		LightThemeOtherColors: []int{6}, DarkThemeMainColor: 7, DarkThemeOtherColors: []int{8}}
	uf.IsBurned = true
	uf.PublisherChat = &tg.Chat{ID: -100777, Type: "channel", Title: "pub"}
	ub := b65iBase()
	ub.IsFromBlockchain = true
	for _, tc := range []struct {
		name, reply string
		want        tg.UniqueGift
	}{{"UF", b65iUF, uf}, {"UB", b65iUB, ub}} {
		t.Run(tc.name, func(t *testing.T) {
			server := b32Server(t, "getUniqueGift", b32Body(tc.reply))
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := client.GetUniqueGift(context.Background(), "PlushPepe-1")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, *got)
			b32Once(t, server, "getUniqueGift")
			c := server.LastCapture()
			c.AssertMethod(t, http.MethodPost)
			c.AssertContentType(t, "application/json")
			assert.JSONEq(t, `{"name":"PlushPepe-1"}`, c.BodyString())
		})
	}
}

// S-ug-name-asis: the name is sent unchanged, also one made only of spaces.
func TestB65I_GetUniqueGiftNameAsIs(t *testing.T) {
	for _, name := range []string{" a b ", "   "} {
		server := b32Server(t, "getUniqueGift", b32Body(`{"gift_id":"g"}`))
		client := testutil.NewTestClient(t, server.BaseURL())
		_, err := client.GetUniqueGift(context.Background(), name)
		require.NoError(t, err, "%q", name)
		b32Once(t, server, "getUniqueGift")
		assert.Equal(t, map[string]any{"name": name}, server.LastCapture().BodyMap(t))
	}
}

// S-ug-empty: an empty name is a validation error before any request.
func TestB65I_GetUniqueGiftEmptyName(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	got, err := client.GetUniqueGift(context.Background(), "")
	assert.Nil(t, got)
	var ve *tg.ValidationError
	require.True(t, errors.As(err, &ve), "error %v", err)
	assert.Equal(t, "name", ve.Field)
	assert.Equal(t, 0, server.CaptureCount())
}

// S-ug-noid: a result without a gift_id is an error with no value; a gift_id alone is a valid result.
func TestB65I_GetUniqueGiftNoID(t *testing.T) {
	for _, tc := range []struct{ name, result string }{
		{"null", `null`}, {"empty_object", `{}`}, {"empty_id", `{"gift_id":""}`}, {"populated_no_id", `{"name":"Fake-1","number":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := b32Server(t, "getUniqueGift", b32Body(tc.result))
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := client.GetUniqueGift(context.Background(), "PlushPepe-1")
			assert.Nil(t, got)
			require.EqualError(t, err, b65iNoID)
			var apiErr *tg.APIError
			assert.False(t, errors.As(err, &apiErr), "not an API error")
		})
	}
	t.Run("id_only", func(t *testing.T) {
		server := b32Server(t, "getUniqueGift", b32Body(`{"gift_id":"x"}`))
		client := testutil.NewTestClient(t, server.BaseURL())
		got, err := client.GetUniqueGift(context.Background(), "PlushPepe-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, tg.UniqueGift{GiftID: "x"}, *got)
	})
}

// S-ug-result-bad: an absent or wrong-type result, or a bad field after gift_id, is callJSON's parse error and no value.
func TestB65I_GetUniqueGiftBadResult(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"absent", b32Absent}, {"string", b32Body(`"x"`)}, {"array", b32Body(`[]`)}, {"bad_number", b32Body(`{"gift_id":"x","number":"bad"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := b32Server(t, "getUniqueGift", tc.body)
			client := testutil.NewTestClient(t, server.BaseURL())
			got, err := client.GetUniqueGift(context.Background(), "PlushPepe-1")
			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "galigo: getUniqueGift: failed to parse response")
			assert.NotEqual(t, b65iNoID, err.Error())
		})
	}
}

// S-del-call: one JSON POST with both ids unchanged (also with spaces, also made only of spaces).
func TestB65I_DeleteBusinessAccountGiftCall(t *testing.T) {
	for _, req := range []sender.DeleteBusinessAccountGiftRequest{
		{BusinessConnectionID: " bc fake ", OwnedGiftID: " og fake "},
		{BusinessConnectionID: "   ", OwnedGiftID: "og"},
		{BusinessConnectionID: "bc", OwnedGiftID: "   "},
	} {
		server := b32Server(t, "deleteBusinessAccountGift", b32Body(`true`))
		client := testutil.NewTestClient(t, server.BaseURL())
		require.NoError(t, client.DeleteBusinessAccountGift(context.Background(), req), "%+v", req)
		b32Once(t, server, "deleteBusinessAccountGift")
		c := server.LastCapture()
		c.AssertMethod(t, http.MethodPost)
		c.AssertContentType(t, "application/json")
		assert.Equal(t, map[string]any{"business_connection_id": req.BusinessConnectionID, "owned_gift_id": req.OwnedGiftID}, c.BodyMap(t))
	}
}

// S-del-valid: both ids are required, the connection id first; no request.
func TestB65I_DeleteBusinessAccountGiftValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		req   sender.DeleteBusinessAccountGiftRequest
		field string
	}{
		{"both_empty", sender.DeleteBusinessAccountGiftRequest{}, "business_connection_id"},
		{"no_connection", sender.DeleteBusinessAccountGiftRequest{OwnedGiftID: "og"}, "business_connection_id"},
		{"no_owned_gift", sender.DeleteBusinessAccountGiftRequest{BusinessConnectionID: "bc"}, "owned_gift_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.DeleteBusinessAccountGift(context.Background(), tc.req)
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "error %v", err)
			assert.Equal(t, tc.field, ve.Field)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// S-noretry: neither method retries a 429, a 5xx or a network timeout; SendMessage, which retries, is the control.
func TestB65I_NoRetry(t *testing.T) {
	for _, set := range []struct {
		name  string
		first func(http.ResponseWriter)
		code  int
	}{
		{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }, 429},
		{"500", func(w http.ResponseWriter) { testutil.ReplyServerError(w, 500, "Internal Server Error") }, 500},
	} {
		t.Run(set.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			for _, p := range []string{"sendMessage", "getUniqueGift", "deleteBusinessAccountGift"} {
				var n atomic.Int32
				server.On(b65iPath(p), func(w http.ResponseWriter, _ *http.Request) {
					if n.Add(1) == 1 {
						set.first(w)
						return
					}
					testutil.ReplyMessage(w, 4242)
				})
			}
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, b65iFast()...)
			for _, m := range b65iMethods {
				server.ResetCaptures()
				err := m.call(context.Background(), client)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "%s: error %v", m.method, err)
				assert.Equal(t, set.code, apiErr.Code, m.method)
				assert.Equal(t, m.method, apiErr.Method)
				assert.False(t, errors.Is(err, tg.ErrMaxRetries), "%s: not retried", m.method)
				assert.Equal(t, 1, server.CaptureCount(), "%s: exactly one request", m.method)
			}
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")
			server.ResetCaptures()
			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.NoError(t, err)
			assert.Equal(t, 2, server.CaptureCount(), "the control call is retried once")
		})
	}
	t.Run("network_timeout", func(t *testing.T) {
		rt := &api101bFailTransport{}
		sleeper := &testutil.FakeSleeper{}
		client := testutil.NewRetryTestClient(t, "http://galigo.invalid", sleeper,
			append(b65iFast(), sender.WithHTTPClient(&http.Client{Transport: rt}))...)
		for _, m := range b65iMethods {
			rt.n.Store(0)
			err := m.call(context.Background(), client)
			var ne net.Error
			require.True(t, errors.As(err, &ne) && ne.Timeout(), "%s: error %v", m.method, err)
			assert.False(t, errors.Is(err, tg.ErrMaxRetries), m.method)
			assert.Equal(t, int32(1), rt.n.Load(), "%s: exactly one round trip", m.method)
		}
		assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep before the control")
		rt.n.Store(0)
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
		require.Error(t, err)
		assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
	})
}

// S-gates: both methods take a global token and go through the circuit breaker.
func TestB65I_Gates(t *testing.T) {
	for _, m := range b65iMethods {
		t.Run(m.method+"/global_burst_0", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0),
				sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
			err := m.call(b51Ctx(t), client)
			assert.EqualError(t, err, b51Burst0)
			assert.Equal(t, 0, server.CaptureCount())
		})
		t.Run(m.method+"/breaker", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(b65iPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, 500, "Internal Server Error")
			})
			client := testutil.NewBreakerTestClient(t, server.BaseURL(), b65iFast()...)
			for i := range 2 {
				err := m.call(context.Background(), client)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "call %d: %v", i+1, err)
				assert.Equal(t, 500, apiErr.Code)
			}
			err := m.call(context.Background(), client)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
}

// S-nochat: neither method uses a per-chat or group limiter (both have burst 0 here).
func TestB65I_NoChatLimiter(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On(b65iPath("getUniqueGift"), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b32Body(`{"gift_id":"g"}`)) })
	server.On(b65iPath("deleteBusinessAccountGift"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
		sender.WithPerChatRateLimit(1, 0), sender.WithGroupRateLimit(1, 0))
	for _, m := range b65iMethods {
		require.NoError(t, m.call(context.Background(), client), m.method)
	}
	assert.Equal(t, 2, server.CaptureCount())
}

// S-ctx: a canceled context stops both methods before any request.
func TestB65I_CanceledContext(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, m := range b65iMethods {
		err := m.call(ctx, client)
		assert.True(t, errors.Is(err, context.Canceled), "%s: %v", m.method, err)
	}
	assert.Equal(t, 0, server.CaptureCount())
}
