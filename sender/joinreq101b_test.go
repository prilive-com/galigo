package sender_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 part b: join request queries (B-25 10.1b, TEST-PLAN §3, rows T-s-1 … T-s-13).
//
// The inputs are values a normaliser would change (E-0117): spaces at both ends, mixed case, "+", a lower-case
// "%2f", "%20", an unsorted query and a fragment. The request body is compared with literal JSON.
const (
	api101bID = " jQ-Ab+%2fx/Y== "
	// api101bU is padded, so a trim changes it; url.Parse rejects it, so it does not test parsing
	api101bU = " HTTPS://E.x/A%2fb?z=2&a=A%20B&plus=%2b#F "
	// api101bUC is not padded: url.Parse accepts it, and its String/Query().Encode() would change it
	api101bUC = "HTTPS://E.x/A%2fb?z=2&a=A%20B&plus=%2b#F"
	// api101bB is a blank value: only spaces and a tab
	api101bB = " \t "
)

func api101bPath(method string) string { return "/bot" + testutil.TestToken + "/" + method }

func api101bOK(server *testutil.MockTelegramServer, methods ...string) {
	for _, m := range methods {
		server.On(api101bPath(m), func(w http.ResponseWriter, _ *http.Request) {
			api96ReplyRaw(w, `{"ok":true,"result":true}`)
		})
	}
}

// api101bMethods calls each new method with a valid request.
var api101bMethods = []struct {
	method string
	call   func(context.Context, *sender.Client) error
}{
	{"answerChatJoinRequestQuery", func(ctx context.Context, c *sender.Client) error {
		return c.AnswerChatJoinRequestQuery(ctx, sender.AnswerChatJoinRequestQueryRequest{
			ChatJoinRequestQueryID: api101bID, Result: "approve"})
	}},
	{"sendChatJoinRequestWebApp", func(ctx context.Context, c *sender.Client) error {
		return c.SendChatJoinRequestWebApp(ctx, sender.SendChatJoinRequestWebAppRequest{
			ChatJoinRequestQueryID: api101bID, WebAppURL: api101bUC})
	}},
}

// T-s-1
func TestAPI101B_AnswerChatJoinRequestQuery(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101bOK(server, "answerChatJoinRequestQuery")
	client := testutil.NewTestClient(t, server.BaseURL())

	err := client.AnswerChatJoinRequestQuery(context.Background(), sender.AnswerChatJoinRequestQueryRequest{
		ChatJoinRequestQueryID: api101bID, Result: sender.JoinRequestApprove})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	c.AssertPath(t, api101bPath("answerChatJoinRequestQuery"))
	c.AssertContentType(t, "application/json")
	assert.JSONEq(t, `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"approve"}`, c.BodyString())
}

// T-s-2: the constants are the spec values, and every result is sent as given (no value check, no trim).
func TestAPI101B_AnswerChatJoinRequestQuery_Results(t *testing.T) {
	assert.Equal(t, "approve", sender.JoinRequestApprove)
	assert.Equal(t, "decline", sender.JoinRequestDecline)
	assert.Equal(t, "queue", sender.JoinRequestQueue)

	rows := []struct{ result, body string }{
		{sender.JoinRequestApprove, `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"approve"}`},
		{sender.JoinRequestDecline, `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"decline"}`},
		{sender.JoinRequestQueue, `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"queue"}`},
		{"APPROVE", `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"APPROVE"}`},
		{" APPROVE ", `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":" APPROVE "}`},
		{"maybe", `{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":"maybe"}`},
	}
	for _, r := range rows {
		t.Run(r.result, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, "answerChatJoinRequestQuery")
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.AnswerChatJoinRequestQuery(context.Background(), sender.AnswerChatJoinRequestQueryRequest{
				ChatJoinRequestQueryID: api101bID, Result: r.result})
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			assert.JSONEq(t, r.body, server.LastCapture().BodyString())
		})
	}
}

type api101bValRow struct {
	name  string
	call  func(*sender.Client) error
	field string // "" = no error expected
	text  string
	body  string // the expected body when no error is expected
}

func api101bRunValidation(t *testing.T, method string, rows []api101bValRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, method)
			client := testutil.NewTestClient(t, server.BaseURL())
			err := r.call(client)
			if r.field == "" {
				require.NoError(t, err)
				require.Equal(t, 1, server.CaptureCount())
				assert.JSONEq(t, r.body, server.LastCapture().BodyString())
				return
			}
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "error %v", err)
			assert.Equal(t, r.field, ve.Field)
			assert.Equal(t, r.text, err.Error()) // the whole text
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

const (
	api101bIDText     = "galigo: validation: chat_join_request_query_id - required"
	api101bResultText = "galigo: validation: result - required"
	api101bURLText    = "galigo: validation: web_app_url - required"
)

// T-s-3
func TestAPI101B_AnswerChatJoinRequestQuery_Validation(t *testing.T) {
	answer := func(id, result string) func(*sender.Client) error {
		return func(c *sender.Client) error {
			return c.AnswerChatJoinRequestQuery(context.Background(), sender.AnswerChatJoinRequestQueryRequest{
				ChatJoinRequestQueryID: id, Result: result})
		}
	}
	api101bRunValidation(t, "answerChatJoinRequestQuery", []api101bValRow{
		{"empty id", answer("", "approve"), "chat_join_request_query_id", api101bIDText, ""},
		{"empty result", answer(api101bID, ""), "result", api101bResultText, ""},
		{"both empty", answer("", ""), "chat_join_request_query_id", api101bIDText, ""},
		{"blank id", answer(api101bB, "approve"), "", "",
			`{"chat_join_request_query_id":" \t ","result":"approve"}`},
		{"blank result", answer(api101bID, api101bB), "", "",
			`{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","result":" \t "}`},
	})
}

// T-s-4
func TestAPI101B_SendChatJoinRequestWebApp(t *testing.T) {
	rows := []struct{ name, url, body string }{
		{"padded", api101bU,
			`{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","web_app_url":" HTTPS://E.x/A%2fb?z=2&a=A%20B&plus=%2b#F "}`},
		{"parseable", api101bUC,
			`{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","web_app_url":"HTTPS://E.x/A%2fb?z=2&a=A%20B&plus=%2b#F"}`},
		{"http scheme", "http://example.com/x",
			`{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","web_app_url":"http://example.com/x"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, "sendChatJoinRequestWebApp")
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.SendChatJoinRequestWebApp(context.Background(), sender.SendChatJoinRequestWebAppRequest{
				ChatJoinRequestQueryID: api101bID, WebAppURL: r.url})
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			c.AssertPath(t, api101bPath("sendChatJoinRequestWebApp"))
			c.AssertContentType(t, "application/json")
			assert.JSONEq(t, r.body, c.BodyString())
		})
	}
}

// T-s-5
func TestAPI101B_SendChatJoinRequestWebApp_Validation(t *testing.T) {
	webApp := func(id, u string) func(*sender.Client) error {
		return func(c *sender.Client) error {
			return c.SendChatJoinRequestWebApp(context.Background(), sender.SendChatJoinRequestWebAppRequest{
				ChatJoinRequestQueryID: id, WebAppURL: u})
		}
	}
	api101bRunValidation(t, "sendChatJoinRequestWebApp", []api101bValRow{
		{"empty id", webApp("", api101bU), "chat_join_request_query_id", api101bIDText, ""},
		{"empty url", webApp(api101bID, ""), "web_app_url", api101bURLText, ""},
		{"both empty", webApp("", ""), "chat_join_request_query_id", api101bIDText, ""},
		{"blank id", webApp(api101bB, api101bU), "", "",
			`{"chat_join_request_query_id":" \t ","web_app_url":" HTTPS://E.x/A%2fb?z=2&a=A%20B&plus=%2b#F "}`},
		{"blank url", webApp(api101bID, api101bB), "", "",
			`{"chat_join_request_query_id":" jQ-Ab+%2fx/Y== ","web_app_url":" \t "}`},
	})
}

// T-s-6
func TestAPI101B_APIError(t *testing.T) {
	for _, m := range api101bMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyError(w, 400, "Bad Request: BOT_GUARD_NOT_SUPPORTED", nil)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			err := m.call(context.Background(), client)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 400, apiErr.Code)
			assert.Equal(t, "Bad Request: BOT_GUARD_NOT_SUPPORTED", apiErr.Description)
			assert.Equal(t, m.method, apiErr.Method)
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// T-s-7: a valid envelope's result is not decoded (any value is nil error); a broken body is an error.
func TestAPI101B_ResultNotDecoded(t *testing.T) {
	bodies := []struct {
		name, body string
		wantErr    bool
	}{
		{"false", `{"ok":true,"result":false}`, false},
		{"string", `{"ok":true,"result":"x"}`, false},
		{"broken", `{"ok":true,"result":`, true},
	}
	for _, m := range api101bMethods {
		for _, b := range bodies {
			t.Run(m.method+"/"+b.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
					api96ReplyRaw(w, b.body)
				})
				client := testutil.NewTestClient(t, server.BaseURL())
				err := m.call(context.Background(), client)
				if b.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// T-s-8: neither method retries (502 and 429); SendMessage on the same client is the control.
func TestAPI101B_NoRetry(t *testing.T) {
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
			paths := []string{"sendMessage"}
			for _, m := range api101bMethods {
				paths = append(paths, m.method)
			}
			for _, p := range paths {
				var n atomic.Int32
				server.On(api101bPath(p), func(w http.ResponseWriter, _ *http.Request) {
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

			for _, m := range api101bMethods {
				server.ResetCaptures()
				set.check(t, m.call(context.Background(), client))
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

// T-s-9: a chat-less call waits on no limiter (plan AC6; pins today's behaviour, B-51 would change this row).
func TestAPI101B_NoRateLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101bOK(server, "answerChatJoinRequestQuery", "sendChatJoinRequestWebApp", "sendMessage")
	client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRateLimit(0, 0))
	for _, m := range api101bMethods {
		server.ResetCaptures()
		require.NoError(t, m.call(context.Background(), client), m.method)
		require.NoError(t, m.call(context.Background(), client), m.method)
		assert.Equal(t, 2, server.CaptureCount(), "%s: two requests", m.method)
	}

	// control, last: a call with a chat id waits on the global limiter, which has burst 0
	server.ResetCaptures()
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api100Chat, Text: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limiter's burst")
	assert.Equal(t, 0, server.CaptureCount())
}

// T-s-10: both methods go through the client's circuit breaker.
func TestAPI101B_Breaker(t *testing.T) {
	fail := func(server *testutil.MockTelegramServer) {
		for _, m := range api101bMethods {
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
		}
	}
	for _, m := range api101bMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			fail(server)
			client := testutil.NewBreakerTestClient(t, server.BaseURL())
			for i := 1; i <= 2; i++ {
				err := m.call(context.Background(), client)
				require.Error(t, err)
				assert.False(t, errors.Is(err, gobreaker.ErrOpenState), "call %d: %v", i, err)
				assert.Equal(t, i, server.CaptureCount(), "call %d sends a request", i)
			}
			err := m.call(context.Background(), client)
			require.Error(t, err)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "call 3: %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "call 3 sends no request")
		})
	}

	t.Run("shared", func(t *testing.T) {
		server := testutil.NewMockServer(t)
		fail(server)
		client := testutil.NewBreakerTestClient(t, server.BaseURL())
		for range 2 {
			require.Error(t, api101bMethods[0].call(context.Background(), client))
		}
		require.Equal(t, 2, server.CaptureCount())
		err := api101bMethods[1].call(context.Background(), client)
		assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "error %v", err)
		assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
	})
}

// T-s-11: the request types have exactly the planned fields and tags, and no methods.
func TestAPI101B_RequestShapes(t *testing.T) {
	type field struct{ name, tag string }
	rows := []struct {
		rt     reflect.Type
		fields []field
	}{
		{reflect.TypeFor[sender.AnswerChatJoinRequestQueryRequest](), []field{
			{"ChatJoinRequestQueryID", `json:"chat_join_request_query_id"`}, {"Result", `json:"result"`}}},
		{reflect.TypeFor[sender.SendChatJoinRequestWebAppRequest](), []field{
			{"ChatJoinRequestQueryID", `json:"chat_join_request_query_id"`}, {"WebAppURL", `json:"web_app_url"`}}},
	}
	for _, r := range rows {
		t.Run(r.rt.Name(), func(t *testing.T) {
			require.Equal(t, len(r.fields), r.rt.NumField())
			for i, f := range r.fields {
				sf := r.rt.Field(i)
				assert.Equal(t, f.name, sf.Name)
				assert.Equal(t, reflect.String, sf.Type.Kind())
				assert.Equal(t, reflect.TypeFor[string](), sf.Type)
				assert.Equal(t, f.tag, string(sf.Tag))
			}
			assert.Equal(t, 0, r.rt.NumMethod(), "no method on the value type")
			assert.Equal(t, 0, reflect.PointerTo(r.rt).NumMethod(), "no method on the pointer type")
		})
	}

	// C1-01 openai: the two methods have exactly the planned signatures (an extra variadic parameter would still
	// compile at every call site)
	assert.Equal(t,
		reflect.TypeFor[func(*sender.Client, context.Context, sender.AnswerChatJoinRequestQueryRequest) error]().String(),
		reflect.TypeOf((*sender.Client).AnswerChatJoinRequestQuery).String())
	assert.Equal(t,
		reflect.TypeFor[func(*sender.Client, context.Context, sender.SendChatJoinRequestWebAppRequest) error]().String(),
		reflect.TypeOf((*sender.Client).SendChatJoinRequestWebApp).String())
}

// T-s-12: a context cancelled before the call stops it before a request is sent.
func TestAPI101B_Context(t *testing.T) {
	for _, m := range api101bMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			api101bOK(server, m.method)
			client := testutil.NewTestClient(t, server.BaseURL())
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := m.call(ctx, client)
			assert.True(t, errors.Is(err, context.Canceled), "error %v", err)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// T-s-13: the caller's context reaches the request: cancelling it while the server holds the reply ends the call.
func TestAPI101B_ContextMidflight(t *testing.T) {
	for _, m := range api101bMethods {
		t.Run(m.method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			arrived := make(chan struct{})
			release := make(chan struct{})
			t.Cleanup(func() { close(release) }) // runs before the server closes (cleanups run last-in first-out)
			server.On(api101bPath(m.method), func(w http.ResponseWriter, _ *http.Request) {
				close(arrived)
				<-release
				api96ReplyRaw(w, `{"ok":true,"result":true}`)
			})
			client := testutil.NewTestClient(t, server.BaseURL())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- m.call(ctx, client) }()

			select {
			case <-arrived:
			case err := <-done:
				t.Fatalf("the call returned before the request arrived: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("the request did not arrive")
			}
			cancel()
			select {
			case err := <-done:
				assert.True(t, errors.Is(err, context.Canceled), "error %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("the call did not end after cancel")
			}
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}
