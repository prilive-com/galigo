package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-145: the S80 steps and the adapter (TEST-PLAN section 4, E1-E7). The real adapter on a mock server; fake ids only.

const (
	b145UID  = int64(7770007770) // above 2^31
	b145CID  = int64(-1009876543210)
	b145UIDs = "7770007770"
	b145MT   = "Bad Request: business connection not found"
	b145Reg  = `{"type":"regular","gift":{"id":"g-1","sticker":{"file_id":"f","file_unique_id":"u","type":"regular","width":1,"height":1,"is_animated":false,"is_video":false},"star_count":5},"send_date":1}`
)

func b145Result(result string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
	}
}

func b145Fail(code int, desc string, retryAfter int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		var p *testutil.Parameters
		if retryAfter > 0 {
			p = &testutil.Parameters{RetryAfter: retryAfter}
		}
		testutil.ReplyError(w, code, desc, p)
	}
}

func b145Path(m string) string { return "/bot" + testutil.TestToken + "/" + m }

// b145Rig is a real adapter on a mock server with the given replies; every other method answers 404.
func b145Rig(t *testing.T, replies map[string]http.HandlerFunc, opts ...sender.Option) (*testutil.MockTelegramServer, *Runtime) {
	t.Helper()
	server := testutil.NewMockServer(t)
	for _, m := range []string{"getUserGifts", "getChatGifts", "getBusinessAccountGifts", "getChat", "getMe", "getChatMember"} {
		h, ok := replies[m]
		if !ok {
			h = b145Fail(404, "Not Found: "+m+" not scripted", 0)
		}
		server.On(b145Path(m), h)
	}
	a := NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL(), opts...))
	return server, NewRuntime(a, b145CID, b145UID)
}

// b145Methods lists the Bot API methods the mock got, in order.
func b145Methods(server *testutil.MockTelegramServer) []string {
	var got []string
	for _, c := range server.Captures() {
		got = append(got, strings.TrimPrefix(c.Path, "/bot"+testutil.TestToken+"/"))
	}
	return got
}

func b145Count(server *testutil.MockTelegramServer, method string) int {
	n := 0
	for _, m := range b145Methods(server) {
		if m == method {
			n++
		}
	}
	return n
}

func b145Super() *ChatContext { return &ChatContext{ChatID: b145CID, ChatType: "supergroup"} }

// b145FailTransport fails every round trip with err and counts the calls (no network).
type b145FailTransport struct {
	n   atomic.Int32
	err error
}

func (rt *b145FailTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.n.Add(1)
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, rt.err
}

// E1: the business row passes only on the measured refusal; no StepName; the S79 evidence keys.
func TestB145_BusinessRefusal(t *testing.T) {
	server, rt := b145Rig(t, map[string]http.HandlerFunc{"getBusinessAccountGifts": b145Fail(400, b145MT, 0)})
	res, err := (&GetBusinessAccountGiftsStep{}).Execute(context.Background(), rt)
	require.NoError(t, err)
	assert.Equal(t, &StepResult{Method: "getBusinessAccountGifts", Evidence: map[string]any{"error_code": 400, "description": b145MT}}, res)
	require.Equal(t, 1, server.CaptureCount())
	assert.Equal(t, `{"business_connection_id":"zzNoSuchConnection"}`, strings.TrimSpace(string(server.LastCapture().Body)))
}

// E2: any other answer fails the business row, with the sender error kept.
func TestB145_BusinessOther(t *testing.T) {
	for _, c := range []struct {
		name  string
		reply http.HandlerFunc
		code  int    // the API error code reachable with errors.As; 0: none
		text  string // a text the error must hold; "": none
	}{
		{"other 400 text", b145Fail(400, "Bad Request: chat not found", 0), 400, ""},
		{"suffix", b145Fail(400, b145MT+"!", 0), 400, ""},
		{"no prefix", b145Fail(400, "business connection not found", 0), 400, ""},
		{"403", b145Fail(403, b145MT, 0), 403, ""},
		{"empty object", b145Result(`{}`), 0, "the server took the call"},
		{"empty page", b145Result(`{"total_count":0,"gifts":[]}`), 0, "the server took the call"},
	} {
		t.Run(c.name, func(t *testing.T) {
			server, rt := b145Rig(t, map[string]http.HandlerFunc{"getBusinessAccountGifts": c.reply})
			res, err := (&GetBusinessAccountGiftsStep{}).Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Nil(t, res)
			assert.Equal(t, 1, server.CaptureCount())
			if c.code != 0 {
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "error %v", err)
				assert.Equal(t, c.code, apiErr.Code)
			}
			if c.text != "" {
				assert.ErrorContains(t, err, c.text)
			}
		})
	}
	t.Run("network error", func(t *testing.T) {
		tr := &b145FailTransport{err: errors.New("b145 network down")}
		_, rt := b145Rig(t, nil, sender.WithHTTPClient(&http.Client{Transport: tr}))
		res, err := (&GetBusinessAccountGiftsStep{}).Execute(context.Background(), rt)
		require.Error(t, err)
		assert.Nil(t, res)
		assert.Equal(t, int32(1), tr.n.Load())
		assert.ErrorIs(t, err, tr.err)
	})
}

// b145Read is one read step: its Bot API method, its runtime setup and the exact request body.
type b145Read struct {
	method string
	step   Step
	setup  func(*Runtime)
	body   string
}

func b145Reads() []b145Read {
	return []b145Read{
		{"getUserGifts", &GetUserGiftsStep{}, func(*Runtime) {}, `{"user_id":7770007770,"limit":1}`},
		{"getChatGifts", &GetChatGiftsStep{}, func(rt *Runtime) { rt.ChatCtx = b145Super() }, `{"chat_id":-1009876543210,"limit":1}`},
	}
}

// b145Run runs one read step with reply as its method's answer.
func b145Run(t *testing.T, r b145Read, reply http.HandlerFunc) (*testutil.MockTelegramServer, *StepResult, error) {
	t.Helper()
	server, rt := b145Rig(t, map[string]http.HandlerFunc{r.method: reply})
	r.setup(rt)
	res, err := r.step.Execute(context.Background(), rt)
	return server, res, err
}

// E3: the two read steps: the request, the evidence, no StepName, the Method, an API error kept.
func TestB145_ReadSteps(t *testing.T) {
	t.Run("user admin 0 skips", func(t *testing.T) {
		server, rt := b145Rig(t, nil)
		rt.AdminUserID = 0
		_, err := (&GetUserGiftsStep{}).Execute(context.Background(), rt)
		assert.True(t, IsSkip(err), "error %v", err)
		assert.Equal(t, 0, server.CaptureCount())
	})
	for _, r := range b145Reads() {
		for _, c := range []struct {
			name, result string
			want         map[string]any
		}{
			{"empty", `{"total_count":0,"gifts":[]}`, map[string]any{"total_count": 0, "gifts_returned": 0, "next_offset_nonempty": false}},
			{"one", `{"total_count":4,"gifts":[` + b145Reg + `],"next_offset":"x"}`, map[string]any{"total_count": 4, "gifts_returned": 1, "next_offset_nonempty": true}},
			{"future type", `{"total_count":1,"gifts":[{"type":"future"}]}`, map[string]any{"total_count": 1, "gifts_returned": 1, "next_offset_nonempty": false}},
		} {
			t.Run(r.method+" "+c.name, func(t *testing.T) {
				server, res, err := b145Run(t, r, b145Result(c.result))
				require.NoError(t, err)
				assert.Equal(t, &StepResult{Method: r.method, Evidence: c.want}, res)
				require.Equal(t, 1, server.CaptureCount())
				assert.Equal(t, r.body, strings.TrimSpace(string(server.LastCapture().Body)))
			})
		}
		t.Run(r.method+" api error", func(t *testing.T) {
			_, res, err := b145Run(t, r, b145Fail(400, "Bad Request: chat not found", 0))
			assert.Nil(t, res)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 400, apiErr.Code)
		})
	}
}

// b145BadPages each fail exactly one page check; check is a word of its error text.
var b145BadPages = []struct{ name, result, check string }{
	{"null result", `null`, "no gift list"},
	{"empty object", `{}`, "no gift list"},
	{"null gifts", `{"total_count":0,"gifts":null}`, "no gift list"},
	{"two gifts", `{"total_count":2,"gifts":[` + b145Reg + `,` + b145Reg + `]}`, "want at most 1"},
	{"count below", `{"total_count":0,"gifts":[` + b145Reg + `]}`, "below the 1 gifts returned"},
	{"null item", `{"total_count":1,"gifts":[null]}`, "has no type"},
	{"empty type", `{"total_count":1,"gifts":[{"type":""}]}`, "has no type"},
}

// E4: each page check fails its own row.
func TestB145_PageChecks(t *testing.T) {
	for _, r := range b145Reads() {
		for _, p := range b145BadPages {
			t.Run(r.method+" "+p.name, func(t *testing.T) {
				server, res, err := b145Run(t, r, b145Result(p.result))
				assert.Equal(t, 1, server.CaptureCount(), "the call was made")
				require.Error(t, err, "the step must fail")
				assert.Nil(t, res)
				assert.Contains(t, err.Error(), p.check)
			})
		}
	}
}

// E5: the user step never writes the requested user id in an error text or in its evidence.
func TestB145_UserRedaction(t *testing.T) {
	for i := 0; i+4 <= len(b145UIDs); i++ {
		require.NotContains(t, testutil.TestToken, b145UIDs[i:i+4], "the token scrub would hide the id")
	}
	user := b145Reads()[0]
	t.Run("a api error", func(t *testing.T) {
		_, _, err := b145Run(t, user, b145Fail(400, "Bad Request: user "+b145UIDs+" is hidden, user "+b145UIDs, 0))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), b145UIDs)
		assert.Contains(t, err.Error(), "[user]")
		var apiErr *tg.APIError
		require.True(t, errors.As(err, &apiErr), "error %v", err)
		assert.Equal(t, 400, apiErr.Code)
	})
	t.Run("b 429", func(t *testing.T) {
		_, _, err := b145Run(t, user, b145Fail(429, "Slow down: user "+b145UIDs, 3))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), b145UIDs)
		assert.Contains(t, err.Error(), "[user]")
		var apiErr *tg.APIError
		require.True(t, errors.As(err, &apiErr), "error %v", err)
		assert.Equal(t, 429, apiErr.Code)
		assert.Equal(t, 3*time.Second, apiErr.RetryAfter)
	})
	t.Run("c page check texts", func(t *testing.T) {
		for _, p := range b145BadPages {
			_, _, err := b145Run(t, user, b145Result(p.result))
			require.Error(t, err, p.name)
			assert.NotContains(t, err.Error(), b145UIDs, p.name)
		}
	})
	t.Run("d evidence", func(t *testing.T) {
		for _, result := range []string{`{"total_count":0,"gifts":[]}`, `{"total_count":4,"gifts":[` + b145Reg + `],"next_offset":"x"}`} {
			_, res, err := b145Run(t, user, b145Result(result))
			require.NoError(t, err)
			b, err := json.Marshal(res.Evidence)
			require.NoError(t, err)
			assert.NotContains(t, string(b), b145UIDs)
			assert.NotContains(t, string(b), "user_id")
		}
	})
	t.Run("e non-API error", func(t *testing.T) {
		sentinel := errors.New("b145 sentinel")
		tr := &b145FailTransport{err: fmt.Errorf("lookup for user %s failed: %w", b145UIDs, sentinel)}
		_, rt := b145Rig(t, nil, sender.WithHTTPClient(&http.Client{Transport: tr}))
		_, err := (&GetUserGiftsStep{}).Execute(context.Background(), rt)
		require.Error(t, err)
		assert.Equal(t, int32(1), tr.n.Load())
		assert.NotContains(t, err.Error(), b145UIDs)
		assert.Contains(t, err.Error(), "[user]")
		assert.ErrorIs(t, err, sentinel)
	})
}

// E6: the chat step reads the chat type from ChatCtx, else from one getChat that writes nothing.
func TestB145_ChatType(t *testing.T) {
	page := b145Result(`{"total_count":0,"gifts":[]}`)
	t.Run("ChatCtx set", func(t *testing.T) {
		server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page})
		p := &ChatContext{ChatID: b145CID, ChatType: "supergroup", IsForum: true, BotIsAdmin: true, CanChangeInfo: true,
			CanDeleteMessages: true, CanRestrictMembers: true, CanPinMessages: true, CanManageTopics: true,
			CanInviteUsers: true, CanManageTags: true}
		saved := *p
		rt.ChatCtx = p
		_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"getChatGifts"}, b145Methods(server))
		assert.Same(t, p, rt.ChatCtx)
		assert.Equal(t, saved, *p)
	})
	t.Run("group runs", func(t *testing.T) {
		server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page})
		rt.ChatCtx = &ChatContext{ChatID: b145CID, ChatType: "group"}
		_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"getChatGifts"}, b145Methods(server))
	})
	for _, typ := range []string{"private", "channel", "zzother"} {
		t.Run("skip "+typ, func(t *testing.T) {
			server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page})
			rt.ChatCtx = &ChatContext{ChatID: b145CID, ChatType: typ}
			_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
			assert.True(t, IsSkip(err), "error %v", err)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
	getChat := func(typ string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyOK(w, map[string]any{"id": b145CID, "type": typ, "accent_color_id": 0, "max_reaction_count": 0})
		}
	}
	t.Run("ChatCtx nil probes once", func(t *testing.T) {
		server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page, "getChat": getChat("supergroup")})
		_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, []string{"getChat", "getChatGifts"}, b145Methods(server))
		assert.Equal(t, `{"chat_id":-1009876543210}`, strings.TrimSpace(string(server.CaptureAt(0).Body)))
		assert.Nil(t, rt.ChatCtx)
	})
	t.Run("ChatCtx nil private skips", func(t *testing.T) {
		server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page, "getChat": getChat("private")})
		_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
		assert.True(t, IsSkip(err), "error %v", err)
		assert.Equal(t, 1, b145Count(server, "getChat"))
		assert.Equal(t, 0, b145Count(server, "getChatGifts"))
		assert.Nil(t, rt.ChatCtx)
	})
	t.Run("ChatCtx nil getChat fails", func(t *testing.T) {
		server, rt := b145Rig(t, map[string]http.HandlerFunc{"getChatGifts": page, "getChat": b145Fail(400, "Bad Request: chat not found", 0)})
		_, err := (&GetChatGiftsStep{}).Execute(context.Background(), rt)
		require.Error(t, err)
		assert.False(t, IsSkip(err))
		var apiErr *tg.APIError
		require.True(t, errors.As(err, &apiErr), "error %v", err)
		assert.Equal(t, 400, apiErr.Code)
		assert.Equal(t, 1, b145Count(server, "getChat"))
		assert.Equal(t, 0, b145Count(server, "getChatGifts"))
		assert.Nil(t, rt.ChatCtx)
	})
}

// b145AdapterCase sends one request through the adapter; body is the literal the mock must get.
type b145AdapterCase struct {
	name, method, body string
	call               func(context.Context, *SenderAdapter) (*tg.OwnedGifts, error)
}

func b145AdapterCases() []b145AdapterCase {
	const off, offJS = " O 1/é<&> ", `" O 1/é\u003c\u0026\u003e "`
	user := func(name, body string, req sender.GetUserGiftsRequest) b145AdapterCase {
		req.UserID = b145UID
		return b145AdapterCase{"user " + name, "getUserGifts", `{"user_id":7770007770` + body + `}`,
			func(ctx context.Context, a *SenderAdapter) (*tg.OwnedGifts, error) { return a.GetUserGifts(ctx, req) }}
	}
	chat := func(name, body string, req sender.GetChatGiftsRequest) b145AdapterCase {
		if req.ChatID == nil {
			req.ChatID = b145CID
			body = `"chat_id":-1009876543210` + body
		}
		return b145AdapterCase{"chat " + name, "getChatGifts", `{` + body + `}`,
			func(ctx context.Context, a *SenderAdapter) (*tg.OwnedGifts, error) { return a.GetChatGifts(ctx, req) }}
	}
	biz := func(name, body string, req sender.GetBusinessAccountGiftsRequest) b145AdapterCase {
		if req.BusinessConnectionID == "" {
			req.BusinessConnectionID = "bc-1"
			body = `"business_connection_id":"bc-1"` + body
		}
		return b145AdapterCase{"business " + name, "getBusinessAccountGifts", `{` + body + `}`,
			func(ctx context.Context, a *SenderAdapter) (*tg.OwnedGifts, error) {
				return a.GetBusinessAccountGifts(ctx, req)
			}}
	}
	return []b145AdapterCase{
		user("full", `,"exclude_unlimited":true,"exclude_limited_upgradable":true,"exclude_limited_non_upgradable":true,`+
			`"exclude_from_blockchain":true,"exclude_unique":true,"sort_by_price":true,"offset":`+offJS+`,"limit":100`,
			sender.GetUserGiftsRequest{ExcludeUnlimited: true, ExcludeLimitedUpgradable: true, ExcludeLimitedNonUpgradable: true,
				ExcludeFromBlockchain: true, ExcludeUnique: true, SortByPrice: true, Offset: off, Limit: 100}),
		user("exclude_unlimited", `,"exclude_unlimited":true`, sender.GetUserGiftsRequest{ExcludeUnlimited: true}),
		user("exclude_limited_upgradable", `,"exclude_limited_upgradable":true`, sender.GetUserGiftsRequest{ExcludeLimitedUpgradable: true}),
		user("exclude_limited_non_upgradable", `,"exclude_limited_non_upgradable":true`, sender.GetUserGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		user("exclude_from_blockchain", `,"exclude_from_blockchain":true`, sender.GetUserGiftsRequest{ExcludeFromBlockchain: true}),
		user("exclude_unique", `,"exclude_unique":true`, sender.GetUserGiftsRequest{ExcludeUnique: true}),
		user("sort_by_price", `,"sort_by_price":true`, sender.GetUserGiftsRequest{SortByPrice: true}),
		chat("full", `,"exclude_unsaved":true,"exclude_saved":true,"exclude_unlimited":true,"exclude_limited_upgradable":true,`+
			`"exclude_limited_non_upgradable":true,"exclude_from_blockchain":true,"exclude_unique":true,"sort_by_price":true,`+
			`"offset":`+offJS+`,"limit":100`,
			sender.GetChatGiftsRequest{ExcludeUnsaved: true, ExcludeSaved: true, ExcludeUnlimited: true, ExcludeLimitedUpgradable: true,
				ExcludeLimitedNonUpgradable: true, ExcludeFromBlockchain: true, ExcludeUnique: true, SortByPrice: true, Offset: off, Limit: 100}),
		chat("exclude_unsaved", `,"exclude_unsaved":true`, sender.GetChatGiftsRequest{ExcludeUnsaved: true}),
		chat("exclude_saved", `,"exclude_saved":true`, sender.GetChatGiftsRequest{ExcludeSaved: true}),
		chat("exclude_unlimited", `,"exclude_unlimited":true`, sender.GetChatGiftsRequest{ExcludeUnlimited: true}),
		chat("exclude_limited_upgradable", `,"exclude_limited_upgradable":true`, sender.GetChatGiftsRequest{ExcludeLimitedUpgradable: true}),
		chat("exclude_limited_non_upgradable", `,"exclude_limited_non_upgradable":true`, sender.GetChatGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		chat("exclude_from_blockchain", `,"exclude_from_blockchain":true`, sender.GetChatGiftsRequest{ExcludeFromBlockchain: true}),
		chat("exclude_unique", `,"exclude_unique":true`, sender.GetChatGiftsRequest{ExcludeUnique: true}),
		chat("sort_by_price", `,"sort_by_price":true`, sender.GetChatGiftsRequest{SortByPrice: true}),
		chat("string id", `"chat_id":"@Chan_X"`, sender.GetChatGiftsRequest{ChatID: "@Chan_X"}),
		biz("full", `,"exclude_unsaved":true,"exclude_saved":true,"exclude_unlimited":true,"exclude_limited_upgradable":true,`+
			`"exclude_limited_non_upgradable":true,"exclude_unique":true,"exclude_from_blockchain":true,"sort_by_price":true,`+
			`"offset":`+offJS+`,"limit":100`,
			sender.GetBusinessAccountGiftsRequest{ExcludeUnsaved: true, ExcludeSaved: true, ExcludeUnlimited: true,
				ExcludeLimitedUpgradable: true, ExcludeLimitedNonUpgradable: true, ExcludeUnique: true, ExcludeFromBlockchain: true,
				SortByPrice: true, Offset: off, Limit: 100}),
		biz("exclude_unsaved", `,"exclude_unsaved":true`, sender.GetBusinessAccountGiftsRequest{ExcludeUnsaved: true}),
		biz("exclude_saved", `,"exclude_saved":true`, sender.GetBusinessAccountGiftsRequest{ExcludeSaved: true}),
		biz("exclude_unlimited", `,"exclude_unlimited":true`, sender.GetBusinessAccountGiftsRequest{ExcludeUnlimited: true}),
		biz("exclude_limited_upgradable", `,"exclude_limited_upgradable":true`, sender.GetBusinessAccountGiftsRequest{ExcludeLimitedUpgradable: true}),
		biz("exclude_limited_non_upgradable", `,"exclude_limited_non_upgradable":true`, sender.GetBusinessAccountGiftsRequest{ExcludeLimitedNonUpgradable: true}),
		biz("exclude_unique", `,"exclude_unique":true`, sender.GetBusinessAccountGiftsRequest{ExcludeUnique: true}),
		biz("exclude_from_blockchain", `,"exclude_from_blockchain":true`, sender.GetBusinessAccountGiftsRequest{ExcludeFromBlockchain: true}),
		biz("sort_by_price", `,"sort_by_price":true`, sender.GetBusinessAccountGiftsRequest{SortByPrice: true}),
		biz("spaces", `"business_connection_id":" Bc-1 "`, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: " Bc-1 "}),
		biz("blank", `"business_connection_id":"   "`, sender.GetBusinessAccountGiftsRequest{BusinessConnectionID: "   "}),
	}
}

func b145Adapter(t *testing.T, reply http.HandlerFunc) (*testutil.MockTelegramServer, *SenderAdapter) {
	t.Helper()
	server := testutil.NewMockServer(t)
	for _, m := range []string{"getUserGifts", "getChatGifts", "getBusinessAccountGifts"} {
		server.On(b145Path(m), reply)
	}
	return server, NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL()))
}

// E7: the adapter sends every field as given and returns the result; an API error is forwarded.
func TestB145_Adapter(t *testing.T) {
	ok := b145Result(`{"total_count":7,"gifts":[` + b145Reg + `],"next_offset":"n"}`)
	for _, c := range b145AdapterCases() {
		t.Run(c.name, func(t *testing.T) {
			server, a := b145Adapter(t, ok)
			got, err := c.call(context.Background(), a)
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			assert.Equal(t, b145Path(c.method), server.LastCapture().Path)
			assert.Equal(t, c.body, strings.TrimSpace(string(server.LastCapture().Body)))
			require.NotNil(t, got)
			assert.Equal(t, 7, got.TotalCount)
			assert.Equal(t, "n", got.NextOffset)
			require.Len(t, got.Gifts, 1)
			require.NotNil(t, got.Gifts[0].Gift)
			assert.Equal(t, "g-1", got.Gifts[0].Gift.ID)
		})
	}
	for _, c := range b145AdapterCases()[:1] {
		t.Run(c.name+" api error", func(t *testing.T) {
			_, a := b145Adapter(t, b145Fail(400, "Bad Request: chat not found", 0))
			_, err := c.call(context.Background(), a)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 400, apiErr.Code)
		})
	}
}

// S12 (adapter): a canceled context stops each adapter method before any request.
func TestB145_AdapterCanceled(t *testing.T) {
	server, a := b145Adapter(t, b145Result(`{"total_count":0,"gifts":[]}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, method := range []string{"getUserGifts", "getChatGifts", "getBusinessAccountGifts"} {
		for _, c := range b145AdapterCases() {
			if c.method != method {
				continue
			}
			_, err := c.call(ctx, a)
			assert.ErrorIs(t, err, context.Canceled, c.name)
			break
		}
	}
	assert.Equal(t, 0, server.CaptureCount())
}
