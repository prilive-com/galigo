package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-145: getUserGifts, getChatGifts, getBusinessAccountGifts and the deprecated GetOwnedGifts (TEST-PLAN section 3).
// Fake ids only. Expected bodies are literals, not built from the request tags.

const (
	b145UID    = int64(7770007770)
	b145CID    = int64(-1009876543210)
	b145Off    = " O 1/é<&> "
	b145OffJS  = `" O 1/é\u003c\u0026\u003e "` // encoding/json escapes <, > and & in a string
	b145Page   = `{"total_count":7,"gifts":[{"type":"regular","gift":{"id":"g-1","sticker":{"file_id":"f","file_unique_id":"u","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"star_count":5},"send_date":1},{"type":"unique","gift":{"gift_id":"u-1","base_name":"P","name":"P-9","number":9},"send_date":2}],"next_offset":"n"}`
	b145BadRes = `{"total_count":1,"gifts":[{"type":"regular","send_date":"x"}]}`
)

// b145Method is one public method, called with valid input; path is the Bot API method it sends.
type b145Method struct {
	name, path string
	call       func(context.Context, *sender.Client) (*tg.OwnedGifts, error)
	chat       bool
}

var b145Methods = []b145Method{
	{"GetUserGifts", "getUserGifts", func(ctx context.Context, c *sender.Client) (*tg.OwnedGifts, error) {
		return c.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: b145UID})
	}, false},
	{"GetChatGifts", "getChatGifts", func(ctx context.Context, c *sender.Client) (*tg.OwnedGifts, error) {
		return c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: b145CID})
	}, true},
	{"GetBusinessAccountGifts", "getBusinessAccountGifts", func(ctx context.Context, c *sender.Client) (*tg.OwnedGifts, error) {
		return c.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "bc-1"})
	}, false},
	{"GetOwnedGifts", "getUserGifts", func(ctx context.Context, c *sender.Client) (*tg.OwnedGifts, error) {
		return c.GetOwnedGifts(ctx, sender.GetOwnedGiftsRequest{UserID: b145UID}) //nolint:staticcheck // the deprecated method is under test
	}, false},
}

func b145Server(t *testing.T) *testutil.MockTelegramServer {
	t.Helper()
	server := testutil.NewMockServer(t)
	for _, p := range []string{"getUserGifts", "getChatGifts", "getBusinessAccountGifts"} {
		server.On(b65iPath(p), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b32Body(b145Page)) })
	}
	server.On(b65iPath("getOwnedGifts"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyNotFound(w, "Not Found") })
	for _, p := range []string{"unbanChatMember"} {
		server.On(b65iPath(p), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
	}
	server.On(b65iPath("sendMessage"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 42) })
	return server
}

// b145Body runs call on a fresh client and returns the one request's path and exact body.
func b145Body(t *testing.T, call func(context.Context, *sender.Client) error) (string, string) {
	t.Helper()
	server := b145Server(t)
	client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
	require.NoError(t, call(context.Background(), client))
	require.Equal(t, 1, server.CaptureCount(), "exactly one request")
	c := server.LastCapture()
	return strings.TrimPrefix(c.Path, "/bot"+testutil.TestToken+"/"), strings.TrimSpace(string(c.Body))
}

// S1: the smallest valid request of each method.
func TestB145_MinimalBodies(t *testing.T) {
	for _, c := range []struct {
		name, path, body string
		call             func(context.Context, *sender.Client) error
	}{
		{"user", "getUserGifts", `{"user_id":7770007770}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: b145UID})
			return err
		}},
		{"chat int64", "getChatGifts", `{"chat_id":-1009876543210}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: b145CID})
			return err
		}},
		{"chat string", "getChatGifts", `{"chat_id":"@chan_x"}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: "@chan_x"})
			return err
		}},
		{"chat string upper", "getChatGifts", `{"chat_id":"@Chan_X"}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: "@Chan_X"})
			return err
		}},
		{"chat int", "getChatGifts", `{"chat_id":-100}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: int(-100)})
			return err
		}},
		{"business", "getBusinessAccountGifts", `{"business_connection_id":"bc-1"}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "bc-1"})
			return err
		}},
		{"business spaces", "getBusinessAccountGifts", `{"business_connection_id":" Bc-1 "}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: " Bc-1 "})
			return err
		}},
		{"business blank", "getBusinessAccountGifts", `{"business_connection_id":"   "}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "   "})
			return err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			path, body := b145Body(t, c.call)
			assert.Equal(t, c.path, path)
			assert.Equal(t, c.body, body)
		})
	}
}

// b145FlagCase is one request with exactly one flag set, and the key it must add.
type b145FlagCase struct {
	key  string
	call func(context.Context, *sender.Client) error
}

// b145UserFlags, b145ChatFlags and b145BizFlags set each spec flag alone (6, 8 and 8 flags). The adapter test
// (engine) runs the same flags.
func b145UserFlags() []b145FlagCase {
	r := func(key string, req sender.GetUserGiftsRequest) b145FlagCase {
		req.UserID = b145UID
		return b145FlagCase{key, func(ctx context.Context, c *sender.Client) error { _, err := c.GetUserGifts(ctx, req); return err }}
	}
	return []b145FlagCase{
		r("exclude_unlimited", sender.GetUserGiftsRequest{ExcludeUnlimited: true}),
		r("exclude_limited_upgradable", sender.GetUserGiftsRequest{ExcludeLimitedUpgradable: true}),
		r("exclude_limited_non_upgradable", sender.GetUserGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		r("exclude_from_blockchain", sender.GetUserGiftsRequest{ExcludeFromBlockchain: true}),
		r("exclude_unique", sender.GetUserGiftsRequest{ExcludeUnique: true}),
		r("sort_by_price", sender.GetUserGiftsRequest{SortByPrice: true}),
	}
}

func b145ChatFlags() []b145FlagCase {
	r := func(key string, req sender.GetChatGiftsRequest) b145FlagCase {
		req.ChatID = b145CID
		return b145FlagCase{key, func(ctx context.Context, c *sender.Client) error { _, err := c.GetChatGifts(ctx, req); return err }}
	}
	return []b145FlagCase{
		r("exclude_unsaved", sender.GetChatGiftsRequest{ExcludeUnsaved: true}),
		r("exclude_saved", sender.GetChatGiftsRequest{ExcludeSaved: true}),
		r("exclude_unlimited", sender.GetChatGiftsRequest{ExcludeUnlimited: true}),
		r("exclude_limited_upgradable", sender.GetChatGiftsRequest{ExcludeLimitedUpgradable: true}),
		r("exclude_limited_non_upgradable", sender.GetChatGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		r("exclude_from_blockchain", sender.GetChatGiftsRequest{ExcludeFromBlockchain: true}),
		r("exclude_unique", sender.GetChatGiftsRequest{ExcludeUnique: true}),
		r("sort_by_price", sender.GetChatGiftsRequest{SortByPrice: true}),
	}
}

func b145BizFlags() []b145FlagCase {
	r := func(key string, req sender.GetBusinessAccountGiftsRequest) b145FlagCase {
		req.BusinessConnectionID = "bc-1"
		return b145FlagCase{key, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetBusinessAccountGifts(ctx, req)
			return err
		}}
	}
	return []b145FlagCase{
		r("exclude_unsaved", sender.GetBusinessAccountGiftsRequest{ExcludeUnsaved: true}),
		r("exclude_saved", sender.GetBusinessAccountGiftsRequest{ExcludeSaved: true}),
		r("exclude_unlimited", sender.GetBusinessAccountGiftsRequest{ExcludeUnlimited: true}),
		r("exclude_limited_upgradable", sender.GetBusinessAccountGiftsRequest{ExcludeLimitedUpgradable: true}),
		r("exclude_limited_non_upgradable", sender.GetBusinessAccountGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		r("exclude_unique", sender.GetBusinessAccountGiftsRequest{ExcludeUnique: true}),
		r("exclude_from_blockchain", sender.GetBusinessAccountGiftsRequest{ExcludeFromBlockchain: true}),
		r("sort_by_price", sender.GetBusinessAccountGiftsRequest{SortByPrice: true}),
	}
}

// S2: every field as given, and each flag alone adds exactly its own key.
func TestB145_Fields(t *testing.T) {
	full := []struct {
		name, path, body string
		call             func(context.Context, *sender.Client) error
	}{
		{"user", "getUserGifts", `{"user_id":7770007770,"exclude_unlimited":true,"exclude_limited_upgradable":true,` +
			`"exclude_limited_non_upgradable":true,"exclude_from_blockchain":true,"exclude_unique":true,"sort_by_price":true,` +
			`"offset":` + b145OffJS + `,"limit":100}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: b145UID, ExcludeUnlimited: true,
				ExcludeLimitedUpgradable: true, ExcludeLimitedNonUpgradable: true, ExcludeFromBlockchain: true,
				ExcludeUnique: true, SortByPrice: true, Offset: b145Off, Limit: 100})
			return err
		}},
		{"chat", "getChatGifts", `{"chat_id":-1009876543210,"exclude_unsaved":true,"exclude_saved":true,` +
			`"exclude_unlimited":true,"exclude_limited_upgradable":true,"exclude_limited_non_upgradable":true,` +
			`"exclude_from_blockchain":true,"exclude_unique":true,"sort_by_price":true,"offset":` + b145OffJS + `,"limit":100}`,
			func(ctx context.Context, c *sender.Client) error {
				_, err := c.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: b145CID, ExcludeUnsaved: true,
					ExcludeSaved: true, ExcludeUnlimited: true, ExcludeLimitedUpgradable: true,
					ExcludeLimitedNonUpgradable: true, ExcludeFromBlockchain: true, ExcludeUnique: true, SortByPrice: true,
					Offset: b145Off, Limit: 100})
				return err
			}},
		{"business", "getBusinessAccountGifts", `{"business_connection_id":"bc-1","exclude_unsaved":true,` +
			`"exclude_saved":true,"exclude_unlimited":true,"exclude_limited_upgradable":true,` +
			`"exclude_limited_non_upgradable":true,"exclude_unique":true,"exclude_from_blockchain":true,"sort_by_price":true,` +
			`"offset":` + b145OffJS + `,"limit":100}`, func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "bc-1",
				ExcludeUnsaved: true, ExcludeSaved: true, ExcludeUnlimited: true, ExcludeLimitedUpgradable: true,
				ExcludeLimitedNonUpgradable: true, ExcludeUnique: true, ExcludeFromBlockchain: true, SortByPrice: true,
				Offset: b145Off, Limit: 100})
			return err
		}},
	}
	for _, c := range full {
		t.Run("full "+c.name, func(t *testing.T) {
			path, body := b145Body(t, c.call)
			assert.Equal(t, c.path, path)
			assert.Equal(t, c.body, body)
		})
	}
	for _, set := range []struct {
		name, head string
		cases      []b145FlagCase
	}{
		{"user", `{"user_id":7770007770,`, b145UserFlags()},
		{"chat", `{"chat_id":-1009876543210,`, b145ChatFlags()},
		{"business", `{"business_connection_id":"bc-1",`, b145BizFlags()},
	} {
		for _, f := range set.cases {
			t.Run(set.name+" "+f.key, func(t *testing.T) {
				_, body := b145Body(t, f.call)
				assert.Equal(t, set.head+`"`+f.key+`":true}`, body)
			})
		}
	}
}

// S3, S4: the result, and a null result.
func TestB145_Result(t *testing.T) {
	for _, m := range b145Methods {
		t.Run(m.name, func(t *testing.T) {
			server := b145Server(t)
			client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
			got, err := m.call(context.Background(), client)
			require.NoError(t, err)
			require.Len(t, got.Gifts, 2)
			assert.Equal(t, 7, got.TotalCount)
			assert.Equal(t, "n", got.NextOffset)
			require.NotNil(t, got.Gifts[0].Gift)
			assert.Equal(t, "g-1", got.Gifts[0].Gift.ID)
			assert.Nil(t, got.Gifts[0].UniqueGift)
			assert.Nil(t, got.Gifts[1].Gift)
			require.NotNil(t, got.Gifts[1].UniqueGift)
			assert.Equal(t, 9, got.Gifts[1].UniqueGift.Number)
			c := server.LastCapture()
			assert.Equal(t, b65iPath(m.path), c.Path)
		})
	}
	t.Run("null result", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		server.On(b65iPath("getUserGifts"), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b32Body(`null`)) })
		client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
		got, err := client.GetUserGifts(context.Background(), sender.GetUserGiftsRequest{UserID: b145UID})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Nil(t, got.Gifts)
	})
}

// S5: validation before any request; the id is checked before the limit.
func TestB145_Validation(t *testing.T) {
	server := b145Server(t)
	client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
	ctx := context.Background()
	field := func(t *testing.T, err error, want string) {
		t.Helper()
		var ve *tg.ValidationError
		require.True(t, errors.As(err, &ve), "want *tg.ValidationError, got %v", err)
		assert.Equal(t, want, ve.Field)
	}
	for _, id := range []int64{0, -1} {
		_, err := client.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: id})
		field(t, err, "user_id")
		_, err = client.GetOwnedGifts(ctx, sender.GetOwnedGiftsRequest{UserID: id}) //nolint:staticcheck // deprecated, under test
		field(t, err, "user_id")
	}
	for _, id := range []tg.ChatID{nil, int64(0), int(0), "", 3.5} {
		_, want := client.GetChat(ctx, id)
		require.Error(t, want)
		_, err := client.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: id})
		require.Error(t, err)
		assert.Equal(t, want.Error(), err.Error(), "chat id %#v", id)
	}
	_, bcErr := client.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{})
	field(t, bcErr, "business_connection_id")
	limits := []struct {
		name string
		call func(limit int) error
	}{
		{"user", func(l int) error {
			_, err := client.GetUserGifts(ctx, sender.GetUserGiftsRequest{UserID: b145UID, Limit: l})
			return err
		}},
		{"chat", func(l int) error {
			_, err := client.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: b145CID, Limit: l})
			return err
		}},
		{"business", func(l int) error {
			_, err := client.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "bc-1", Limit: l})
			return err
		}},
		{"owned", func(l int) error {
			_, err := client.GetOwnedGifts(ctx, sender.GetOwnedGiftsRequest{UserID: b145UID, Limit: l}) //nolint:staticcheck // deprecated, under test
			return err
		}},
	}
	for _, l := range limits {
		for _, bad := range []int{-1, 101} {
			err := l.call(bad)
			field(t, err, "limit")
			assert.Contains(t, err.Error(), "must be 1-100", "%s %d", l.name, bad)
		}
	}
	require.Equal(t, 0, server.CaptureCount(), "no request for an invalid input")
	for _, l := range limits {
		for _, ok := range []int{1, 100} {
			require.NoError(t, l.call(ok), "%s %d", l.name, ok)
		}
	}
	assert.Equal(t, 8, server.CaptureCount(), "limits 1 and 100 are sent")
	server.ResetCaptures()
	_, err := client.GetUserGifts(ctx, sender.GetUserGiftsRequest{Limit: 101})
	field(t, err, "user_id")
	_, err = client.GetChatGifts(ctx, sender.GetChatGiftsRequest{Limit: 101})
	assert.NotContains(t, err.Error(), "limit", "the chat id is checked first")
	_, err = client.GetBusinessAccountGifts(ctx, sender.GetBusinessAccountGiftsRequest{Limit: 101})
	field(t, err, "business_connection_id")
	_, err = client.GetOwnedGifts(ctx, sender.GetOwnedGiftsRequest{Limit: 101}) //nolint:staticcheck // deprecated, under test
	field(t, err, "user_id")
	assert.Equal(t, 0, server.CaptureCount())
}

// S6: no method retries a 429, a 500 or a timeout; SendMessage on the same client, with the fault armed again, is the
// control. Each method and fault gets a fresh server, transport and client (GetOwnedGifts shares getUserGifts' path).
func TestB145_NoRetry(t *testing.T) {
	for _, m := range b145Methods {
		for _, f := range []struct {
			name  string
			first func(http.ResponseWriter)
			code  int
		}{
			{"429", func(w http.ResponseWriter) { testutil.ReplyRateLimit(w, 1) }, 429},
			{"500", func(w http.ResponseWriter) { testutil.ReplyServerError(w, 500, "Internal Server Error") }, 500},
		} {
			t.Run(m.name+"/"+f.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				var n atomic.Int32
				for _, p := range []string{"sendMessage", m.path} {
					server.On(b65iPath(p), func(w http.ResponseWriter, _ *http.Request) {
						if n.Add(1) == 1 {
							f.first(w)
							return
						}
						if p == "sendMessage" {
							testutil.ReplyMessage(w, 42)
							return
						}
						api96ReplyRaw(w, b32Body(b145Page))
					})
				}
				sleeper := &testutil.FakeSleeper{}
				client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, b65iFast()...)
				_, err := m.call(context.Background(), client)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, f.code, apiErr.Code)
				assert.Equal(t, 1, server.CaptureCount(), "exactly one request")
				n.Store(0)
				server.ResetCaptures()
				_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
				require.NoError(t, err)
				assert.Equal(t, 2, server.CaptureCount(), "the control call is retried")
			})
		}
		t.Run(m.name+"/timeout", func(t *testing.T) {
			rt := &api101bFailTransport{}
			client := testutil.NewRetryTestClient(t, "http://galigo.invalid", &testutil.FakeSleeper{},
				append(b65iFast(), sender.WithHTTPClient(&http.Client{Transport: rt}))...)
			_, err := m.call(context.Background(), client)
			var ne net.Error
			require.True(t, errors.As(err, &ne) && ne.Timeout(), "error %v", err)
			assert.Equal(t, int32(1), rt.n.Load(), "exactly one round trip")
			rt.n.Store(0)
			_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
			require.Error(t, err)
			assert.GreaterOrEqual(t, rt.n.Load(), int32(2), "the control call is retried")
		})
	}
}

func b145ChatGifts(client *sender.Client, id tg.ChatID) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := client.GetChatGifts(ctx, sender.GetChatGiftsRequest{ChatID: id})
		return err
	}
}

func b145Blocked(t *testing.T, call func(context.Context) error) {
	t.Helper()
	err := b65fBound(t, call)
	require.Error(t, err, "the chat's bucket is empty")
	assert.Contains(t, err.Error(), "would exceed context deadline")
}

// S7: GetChatGifts uses the chat's own bucket, shared with the other chat methods; the chat-less methods use none.
func TestB145_ChatLimiter(t *testing.T) {
	slow := func(t *testing.T, opts ...sender.Option) (*testutil.MockTelegramServer, *sender.Client) {
		server := b145Server(t)
		return server, testutil.NewTestClient(t, server.BaseURL(), append([]sender.Option{sender.WithRateLimit(1000, 100)}, opts...)...)
	}
	both := []sender.Option{sender.WithPerChatRateLimit(0.001, 1), sender.WithGroupRateLimit(0.001, 1)}
	for _, k := range []struct {
		name                string
		first, again, other tg.ChatID
	}{
		{"group", b145CID, b145CID, int64(-200)},
		{"per_chat", int64(100), int64(100), int64(200)},
		{"string", "@chan_x", "@chan_x", "@other"},
	} {
		t.Run("a key "+k.name, func(t *testing.T) {
			server, client := slow(t, both...)
			require.NoError(t, b65fBound(t, b145ChatGifts(client, k.first)))
			b145Blocked(t, b145ChatGifts(client, k.again))
			assert.Equal(t, 1, server.CaptureCount(), "no request for the blocked call")
			require.NoError(t, b65fBound(t, b145ChatGifts(client, k.other)), "another chat has its own bucket")
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
	for _, s := range []struct {
		name  string
		seed  func(context.Context, *sender.Client) error
		again tg.ChatID
	}{
		{"group", func(ctx context.Context, c *sender.Client) error { return c.UnbanChatMember(ctx, b145CID, 7) }, b145CID},
		{"per_chat", func(ctx context.Context, c *sender.Client) error { return c.UnbanChatMember(ctx, 300, 7) }, int64(300)},
		{"int", func(ctx context.Context, c *sender.Client) error { return c.UnbanChatMember(ctx, 300, 7) }, int(300)},
		{"string", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: "@chan_x", Text: "hi"})
			return err
		}, "@chan_x"},
	} {
		t.Run("b shared "+s.name, func(t *testing.T) {
			server, client := slow(t, both...)
			require.NoError(t, b65fBound(t, func(ctx context.Context) error { return s.seed(ctx, client) }))
			b145Blocked(t, b145ChatGifts(client, s.again))
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
	t.Run("c group limiter only", func(t *testing.T) {
		server, client := slow(t, sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(0.001, 1))
		require.NoError(t, b65fBound(t, b145ChatGifts(client, b145CID)))
		b145Blocked(t, b145ChatGifts(client, b145CID))
		assert.Equal(t, 1, server.CaptureCount())
	})
	t.Run("d chat-less", func(t *testing.T) {
		server, client := slow(t, sender.WithPerChatRateLimit(1, 0), sender.WithGroupRateLimit(1, 0))
		for _, m := range b145Methods {
			if m.chat {
				continue
			}
			_, err := m.call(context.Background(), client)
			require.NoError(t, err, m.name)
		}
		assert.Equal(t, 3, server.CaptureCount())
		assert.Equal(t, 0, client.ChatLimiterCount(), "no per-chat limiter")
	})
}

// S8: the deprecated GetOwnedGifts sends getUserGifts with its three fields.
func TestB145_GetOwnedGifts(t *testing.T) {
	server := b145Server(t)
	client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
	_, err := client.GetOwnedGifts(context.Background(), sender.GetOwnedGiftsRequest{UserID: b145UID, Offset: "o", Limit: 5}) //nolint:staticcheck // deprecated, under test
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	assert.Equal(t, b65iPath("getUserGifts"), c.Path)
	assert.Equal(t, `{"user_id":7770007770,"offset":"o","limit":5}`, strings.TrimSpace(string(c.Body)))
}

// S10: every method takes a global token and goes through the circuit breaker.
func TestB145_Gates(t *testing.T) {
	for _, m := range b145Methods {
		t.Run(m.name+"/global_burst_0", func(t *testing.T) {
			server := b145Server(t)
			client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0),
				sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
			_, err := m.call(b51Ctx(t), client)
			assert.EqualError(t, err, b51Burst0)
			assert.Equal(t, 0, server.CaptureCount())
		})
		t.Run(m.name+"/breaker", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(b65iPath(m.path), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, 500, "Internal Server Error")
			})
			client := testutil.NewBreakerTestClient(t, server.BaseURL(), b65iFast()...)
			for i := range 2 {
				_, err := m.call(context.Background(), client)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "call %d: %v", i+1, err)
				assert.Equal(t, 500, apiErr.Code)
			}
			_, err := m.call(context.Background(), client)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount())
		})
	}
}

// S11: a result that does not decode gives no value and the decode error.
func TestB145_BadResult(t *testing.T) {
	for _, m := range b145Methods {
		t.Run(m.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(b65iPath(m.path), func(w http.ResponseWriter, _ *http.Request) { api96ReplyRaw(w, b32Body(b145BadRes)) })
			client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
			got, err := m.call(context.Background(), client)
			assert.Nil(t, got)
			require.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "galigo: "+m.path+": failed to parse response:"), "error %v", err)
			var te *json.UnmarshalTypeError
			assert.True(t, errors.As(err, &te), "error %v", err)
		})
	}
}

// S12: a canceled context stops every method before any request.
func TestB145_CanceledContext(t *testing.T) {
	server := b145Server(t)
	client := testutil.NewTestClient(t, server.BaseURL(), b65iFast()...)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, m := range b145Methods {
		_, err := m.call(ctx, client)
		assert.True(t, errors.Is(err, context.Canceled), "%s: %v", m.name, err)
	}
	assert.Equal(t, 0, server.CaptureCount())
}
