package sender_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
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

// Bot API 10.2 rich messages, input blocks without media (B-25 10.2d, TEST-PLAN §3, rows T-sb-1 … T-sb-7).

func api102dInt(n int) *int { return &n }

// api102dBlocks are the blocks the rows send; api102dBlocksJSON is their encoding.
var api102dBlocks = []tg.InputRichBlock{
	tg.InputRichBlockSectionHeading{Text: tg.RichText{Plain: "h"}, Size: 2},
	tg.InputRichBlockList{Items: []tg.InputRichBlockListItem{{Blocks: []tg.InputRichBlock{tg.InputRichBlockDivider{}}, Value: api102dInt(0)}}},
}

const api102dBlocksJSON = `[{"type":"heading","text":"h","size":2},{"type":"list","items":[{"blocks":[{"type":"divider"}],"value":0}]}]`

// api102dCaller sends rm through one of the three callers that validate an InputRichMessage.
type api102dCaller struct {
	name, method string
	call         func(context.Context, *sender.Client, tg.InputRichMessage) error
	// body is the expected request body around the rich message.
	body func(rich string) string
}

var api102dCallers = []api102dCaller{
	{"send", "sendRichMessage", func(ctx context.Context, c *sender.Client, rm tg.InputRichMessage) error {
		_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: api101dChat, RichMessage: rm})
		return err
	}, func(rich string) string { return `{"chat_id":123456789,"rich_message":` + rich + `}` }},
	{"draft", "sendRichMessageDraft", func(ctx context.Context, c *sender.Client, rm tg.InputRichMessage) error {
		return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: api101dChat, DraftID: 4, RichMessage: rm})
	}, func(rich string) string { return `{"chat_id":123456789,"draft_id":4,"rich_message":` + rich + `}` }},
	{"edit", "editMessageText", func(ctx context.Context, c *sender.Client, rm tg.InputRichMessage) error {
		_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: &rm})
		return err
	}, func(rich string) string { return `{"chat_id":123456789,"message_id":9,"rich_message":` + rich + `}` }},
}

func api102dServer(t *testing.T) (*testutil.MockTelegramServer, *sender.Client) {
	t.Helper()
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "sendRichMessage", api101dReply)
	api101dOnResult(server, "sendRichMessageDraft", "true")
	server.On(api101dPath("editMessageText"), func(w http.ResponseWriter, _ *http.Request) { api101dReplyRich(w) })
	return server, testutil.NewTestClient(t, server.BaseURL())
}

// T-sb-1: each caller sends a message made of blocks: the whole body, with no html or markdown key; IsRTL and
// SkipEntityDetection go with blocks.
func TestAPI102D_SendBlocks(t *testing.T) {
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			server, client := api102dServer(t)
			require.NoError(t, c.call(context.Background(), client, tg.InputRichMessage{Blocks: api102dBlocks}))
			require.Equal(t, 1, server.CaptureCount())
			got := server.LastCapture()
			got.AssertPath(t, api101dPath(c.method))
			assert.JSONEq(t, c.body(`{"blocks":`+api102dBlocksJSON+`}`), got.BodyString())

			server.ResetCaptures()
			require.NoError(t, c.call(context.Background(), client,
				tg.InputRichMessage{Blocks: api102dBlocks, IsRTL: true, SkipEntityDetection: true}))
			require.Equal(t, 1, server.CaptureCount())
			assert.JSONEq(t, c.body(`{"is_rtl":true,"skip_entity_detection":true,"blocks":`+api102dBlocksJSON+`}`),
				server.LastCapture().BodyString())
		})
	}
}

// T-sb-2: the validator counts the three forms. Every combination for every caller: 0 used → "html, markdown or
// blocks required"; 2 or 3 used → "use only one of html, markdown, blocks"; 1 used → sent. An empty blocks slice
// is not used; a slice with one nil element is used (and sent as [null]).
func TestAPI102D_Validation(t *testing.T) {
	const required, onlyOne = "html, markdown or blocks required", "use only one of html, markdown, blocks"
	empty := []tg.InputRichBlock{}
	rows := []struct {
		name string
		rm   tg.InputRichMessage
		msg  string // a refusal, or "" for a sent request with the rich message want
		want string
	}{
		{"none", tg.InputRichMessage{IsRTL: true}, required, ""},
		{"empty blocks", tg.InputRichMessage{Blocks: empty}, required, ""},
		{"empty blocks and flags", tg.InputRichMessage{Blocks: empty, IsRTL: true, SkipEntityDetection: true}, required, ""},
		{"html", tg.InputRichMessage{HTML: "<p>h</p>"}, "", `{"html":"<p>h</p>"}`},
		{"markdown", tg.InputRichMessage{Markdown: "m"}, "", `{"markdown":"m"}`},
		{"blocks", tg.InputRichMessage{Blocks: api102dBlocks}, "", `{"blocks":` + api102dBlocksJSON + `}`},
		{"nil element", tg.InputRichMessage{Blocks: []tg.InputRichBlock{nil}}, "", `{"blocks":[null]}`},
		{"html and empty blocks", tg.InputRichMessage{HTML: "<p>h</p>", Blocks: empty}, "", `{"html":"<p>h</p>"}`},
		{"markdown and empty blocks", tg.InputRichMessage{Markdown: "m", Blocks: empty}, "", `{"markdown":"m"}`},
		{"html and markdown", tg.InputRichMessage{HTML: "h", Markdown: "m"}, onlyOne, ""},
		{"html and blocks", tg.InputRichMessage{HTML: "h", Blocks: api102dBlocks}, onlyOne, ""},
		{"markdown and blocks", tg.InputRichMessage{Markdown: "m", Blocks: api102dBlocks}, onlyOne, ""},
		{"all three", tg.InputRichMessage{HTML: "h", Markdown: "m", Blocks: api102dBlocks}, onlyOne, ""},
		{"markdown and nil element", tg.InputRichMessage{Markdown: "m", Blocks: []tg.InputRichBlock{nil}}, onlyOne, ""},
	}
	for _, c := range api102dCallers {
		for _, r := range rows {
			t.Run(c.name+"/"+r.name, func(t *testing.T) {
				server, client := api102dServer(t)
				err := c.call(context.Background(), client, r.rm)
				if r.msg != "" {
					api101dRefused(t, server, err, "rich_message", r.msg)
					return
				}
				require.NoError(t, err)
				require.Equal(t, 1, server.CaptureCount())
				assert.JSONEq(t, c.body(r.want), server.LastCapture().BodyString())
			})
		}
	}
}

// T-sb-3: an inline result carries blocks unchecked (D4): blocks with html is sent as given.
func TestAPI102D_AnswerInlineQueryBlocks(t *testing.T) {
	server := testutil.NewMockServer(t)
	api101dOnResult(server, "answerInlineQuery", "true")
	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{InlineQueryID: "q",
		Results: []tg.InlineQueryResult{&tg.InlineQueryResultArticle{Type: "article", ID: "1", Title: "t",
			InputMessageContent: tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{HTML: "h", Blocks: api102dBlocks}}}}})
	require.NoError(t, err)
	var body struct {
		Results []struct {
			InputMessageContent json.RawMessage `json:"input_message_content"`
		} `json:"results"`
	}
	server.LastCapture().BodyJSON(t, &body)
	require.Len(t, body.Results, 1)
	assert.JSONEq(t, `{"rich_message":{"html":"h","blocks":`+api102dBlocksJSON+`}}`, string(body.Results[0].InputMessageContent))
}

// api102dChildEnv makes the test binary run T-sb-4 itself under a small stack limit (see tg's TestAPI102D_Depth).
const api102dChildEnv = "GALIGO_SENDER102D_DEPTH_CHILD"

// T-sb-4: a cyclic or too deep block is refused by each caller with the depth error, before any request; depth 100
// is sent.
func TestAPI102D_SendDepth(t *testing.T) {
	if os.Getenv(api102dChildEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAPI102D_SendDepth$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), api102dChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "the child failed or crashed:\n%s", out)
		var names []string
		for _, c := range api102dCallers {
			names = append(names, c.name)
		}
		require.Equal(t, []string{"send", "draft", "edit"}, names, "the callers whose rows the child must run")
		for _, c := range api102dCallers {
			for _, r := range []string{"cycle", "depth-101", "depth-100"} {
				assert.Contains(t, string(out), "--- PASS: TestAPI102D_SendDepth/"+c.name+"/"+r+" ", c.name+"/"+r)
			}
		}
		return
	}
	debug.SetMaxStack(64 << 20)
	nest := func(n int) []tg.InputRichBlock {
		var b tg.InputRichBlock = tg.InputRichBlockDivider{}
		for range n {
			b = tg.InputRichBlockDetails{Blocks: []tg.InputRichBlock{b}}
		}
		return []tg.InputRichBlock{b}
	}
	for _, c := range api102dCallers {
		t.Run(c.name+"/cycle", func(t *testing.T) {
			server, client := api102dServer(t)
			s := make([]tg.InputRichBlock, 1)
			s[0] = tg.InputRichBlockCollage{Blocks: s}
			err := c.call(context.Background(), client, tg.InputRichMessage{Blocks: s})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tg: input rich blocks nested more than 100 levels deep (or in a cycle)")
			assert.Equal(t, 0, server.CaptureCount())
		})
		t.Run(c.name+"/depth-101", func(t *testing.T) {
			server, client := api102dServer(t)
			err := c.call(context.Background(), client, tg.InputRichMessage{Blocks: nest(101)})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tg: input rich blocks nested more than 100 levels deep (or in a cycle)")
			assert.Equal(t, 0, server.CaptureCount())
		})
		t.Run(c.name+"/depth-100", func(t *testing.T) {
			server, client := api102dServer(t)
			require.NoError(t, c.call(context.Background(), client, tg.InputRichMessage{Blocks: nest(100)}))
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// T-sb-5 (T1-05, T2-01, T2-02, T3-02 of Codex): a blocks message takes the same send path as html and markdown, for
// each caller: the per-chat limiter of the request's chat is used and waited on, it is the SAME limiter a SendMessage
// to that chat uses, the client's circuit breaker is used (an open breaker refuses the call, and failed blocks calls
// open it), and a retryable reply (502) is not retried.
func TestAPI102D_PathBlocks(t *testing.T) {
	rm := tg.InputRichMessage{Blocks: api102dBlocks}
	reply := map[string]func(http.ResponseWriter){
		"sendRichMessage":      func(w http.ResponseWriter) { testutil.ReplyMessage(w, 4242) },
		"sendRichMessageDraft": func(w http.ResponseWriter) { testutil.ReplyBool(w, true) },
		"editMessageText":      func(w http.ResponseWriter) { testutil.ReplyMessage(w, 4242) },
	}
	for _, c := range api102dCallers {
		t.Run(c.name+"/limiter", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			require.Equal(t, 0, client.ChatLimiterCount())
			require.NoError(t, c.call(context.Background(), client, rm))
			assert.Equal(t, 1, client.ChatLimiterCount(), "one per-chat limiter after the call")
			assert.Equal(t, 1, server.CaptureCount())
		})
		t.Run(c.name+"/waits", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0),
				sender.WithPerChatRateLimit(0.01, 1))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			require.NoError(t, c.call(context.Background(), client, rm))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = c.call(ctx, client, rm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount())
		})
		t.Run(c.name+"/shares", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath("sendMessage"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 4241) })
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0),
				sender.WithPerChatRateLimit(0.01, 1))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api101dChat, Text: "x"})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = c.call(ctx, client, rm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount(), "only the SendMessage request")
		})
		t.Run(c.name+"/breaker", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath("deleteMessage"), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client := testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for range 2 {
				require.Error(t, client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{ChatID: api101dChat, MessageID: 1}))
			}
			require.Equal(t, 2, server.CaptureCount())
			err := c.call(context.Background(), client, rm)
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "error %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
		})
		t.Run(c.name+"/opens", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
			client := testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for i := 1; i <= 2; i++ {
				err := c.call(context.Background(), client, rm)
				var apiErr *tg.APIError
				require.True(t, errors.As(err, &apiErr), "call %d: %v", i, err)
				assert.Equal(t, 502, apiErr.Code)
				assert.Equal(t, i, server.CaptureCount(), "call %d sends a request", i)
			}
			err := client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{ChatID: api101dChat, MessageID: 1})
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "the failed blocks calls opened the breaker: %v", err)
			assert.Equal(t, 2, server.CaptureCount(), "no request while the breaker is open")
		})
		t.Run(c.name+"/no-retry", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			var n atomic.Int32
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) {
				if n.Add(1) == 1 {
					testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
					return
				}
				reply[c.method](w)
			})
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			err := c.call(context.Background(), client, rm)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 502, apiErr.Code)
			assert.Equal(t, 1, server.CaptureCount(), "exactly one request")
			assert.Equal(t, 0, sleeper.CallCount(), "no backoff sleep")
		})
	}
}

// T-sb-6 (T2-03 of Codex): the sender checks only the form count (D3), not the blocks' content: an empty paragraph, a
// heading of size 0 and a thinking block outside a draft are sent as given, by each caller.
func TestAPI102D_BlocksUnchecked(t *testing.T) {
	rm := tg.InputRichMessage{Blocks: []tg.InputRichBlock{
		tg.InputRichBlockParagraph{},
		tg.InputRichBlockSectionHeading{},
		tg.InputRichBlockThinking{Text: tg.RichText{Plain: "t"}},
	}}
	const rich = `{"blocks":[{"type":"paragraph","text":""},{"type":"heading","text":"","size":0},{"type":"thinking","text":"t"}]}`
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			server, client := api102dServer(t)
			require.NoError(t, c.call(context.Background(), client, rm))
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, api101dPath(c.method))
			assert.JSONEq(t, c.body(rich), server.LastCapture().BodyString())
		})
	}
}

// T-sb-7 (T3-01 of Codex): blocks go with every other field of the request, for each caller: the whole body is
// sent, as for html and markdown (the T-srm-1, T-srd-1 values).
func TestAPI102D_BlocksOptions(t *testing.T) {
	rm := tg.InputRichMessage{Blocks: api102dBlocks, IsRTL: true}
	kb := testutil.TestInlineKeyboard([]tg.InlineKeyboardButton{testutil.TestInlineButton("b", "d")})
	const rich = `{"is_rtl":true,"blocks":` + api102dBlocksJSON + `}`
	const kbJSON = `{"inline_keyboard":[[{"text":"b","callback_data":"d"}]]}`
	rows := []struct {
		name, method string
		call         func(context.Context, *sender.Client) error
		want         string
	}{
		{"send", "sendRichMessage", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{BusinessConnectionID: api101dID, ChatID: api101dChat,
				MessageThreadID: 7, DirectMessagesTopicID: 8, RichMessage: rm, DisableNotification: true, ProtectContent: true,
				AllowPaidBroadcast: true, MessageEffectID: api101dID, ReplyParameters: &tg.ReplyParameters{MessageID: 5}, ReplyMarkup: kb})
			return err
		}, `{"business_connection_id":" bC-Ab+%2fx/Y== ","chat_id":123456789,"message_thread_id":7,"direct_messages_topic_id":8,` +
			`"rich_message":` + rich + `,"disable_notification":true,"protect_content":true,"allow_paid_broadcast":true,` +
			`"message_effect_id":" bC-Ab+%2fx/Y== ","reply_parameters":{"message_id":5},"reply_markup":` + kbJSON + `}`},
		{"draft", "sendRichMessageDraft", func(ctx context.Context, c *sender.Client) error {
			return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: api101dChat, MessageThreadID: 7, DraftID: 3, RichMessage: rm})
		}, `{"chat_id":123456789,"message_thread_id":7,"draft_id":3,"rich_message":` + rich + `}`},
		{"edit", "editMessageText", func(ctx context.Context, c *sender.Client) error {
			_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: api101dChat, MessageID: 9, RichMessage: &rm, ReplyMarkup: kb})
			return err
		}, `{"chat_id":123456789,"message_id":9,"rich_message":` + rich + `,"reply_markup":` + kbJSON + `}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server, client := api102dServer(t)
			require.NoError(t, r.call(context.Background(), client))
			require.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertPath(t, api101dPath(r.method))
			assert.JSONEq(t, r.want, server.LastCapture().BodyString())
		})
	}
}
