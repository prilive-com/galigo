package sender_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-51: every request of a sender.Client waits on the client's global limiter; a request with a chat id also waits on
// that chat's limiter first. Rows R1-R14 of the B-51 test plan (R10 is TestAPI101B_GlobalRateLimit). A refusal is
// compared as the whole error: the limiter's or the context's error reaches the caller unchanged (T3-01).

const (
	b51Watchdog = 10 * time.Second // a call that blocks longer fails the row, not the whole binary
	b51Deadline = 5 * time.Second  // the tight limiter needs 1000 s, so rate refuses at once
	b51Burst0   = "rate: Wait(n=1) exceeds limiter's burst 0"
	b51Refused  = "rate: Wait(n=1) would exceed context deadline"
)

// b51Class is one input class of executeRequest's chat id (none, "", the fake key "<nil>", a real chat).
type b51Class struct {
	name string
	call func(context.Context, *sender.Client) error
}

var b51Classes = []b51Class{
	{"C1 none (GetMe)", func(ctx context.Context, c *sender.Client) error {
		_, err := c.GetMe(ctx)
		return err
	}},
	{"C2 none via callJSON (SetMyCommands)", func(ctx context.Context, c *sender.Client) error {
		return c.SetMyCommands(ctx, []tg.BotCommand{{Command: "a", Description: "b"}})
	}},
	{`C3 ChatID ""`, func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: "", InlineMessageID: "i", Text: "t"})
		return err
	}},
	{"C4 ChatID nil (control)", func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageText(ctx, sender.EditMessageTextRequest{ChatID: nil, InlineMessageID: "i", Text: "t"})
		return err
	}},
	{"C5 chat (control)", func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: int64(-1001), Text: "t"})
		return err
	}},
}

// b51ChatLess are the classes that had no wait before B-51.
var b51ChatLess = b51Classes[:3]

func b51Path(method string) string { return "/bot" + testutil.TestToken + "/" + method }

func b51Server(t *testing.T) *testutil.MockTelegramServer {
	t.Helper()
	s := testutil.NewMockServer(t)
	s.On(b51Path("getMe"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyUser(w) })
	s.On(b51Path("setMyCommands"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
	s.On(b51Path("editMessageText"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyBool(w, true) })
	s.On(b51Path("sendMessage"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
	return s
}

// b51Client sets the global limit; the per-chat and group limits are high, so only the global limiter decides.
func b51Client(t *testing.T, s *testutil.MockTelegramServer, rps float64, burst int) *sender.Client {
	t.Helper()
	return testutil.NewTestClient(t, s.BaseURL(), sender.WithRateLimit(rps, burst),
		sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
}

// b51Call runs one call under the watchdog.
func b51Call(t *testing.T, ctx context.Context, c *sender.Client, cl b51Class) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cl.call(ctx, c) }()
	select {
	case err := <-done:
		return err
	case <-time.After(b51Watchdog):
		t.Fatalf("%s: the call did not return in %s", cl.name, b51Watchdog)
		return nil
	}
}

func b51Ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), b51Deadline)
	t.Cleanup(cancel)
	return ctx
}

// R1: a zero limiter refuses every class before any request.
func TestB51_ZeroLimiterRefusesEveryClass(t *testing.T) {
	for _, cl := range b51Classes {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 0, 0)
			err := b51Call(t, b51Ctx(t), c, cl)
			require.Error(t, err)
			assert.EqualError(t, err, b51Burst0)
			assert.Equal(t, 0, s.CaptureCount(), "no request")
		})
	}
}

// R2: one token, the next in 1000 s: the first call passes, the second is refused before a request.
func TestB51_TightLimiterBlocksSecondCall(t *testing.T) {
	for _, cl := range b51Classes {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 0.001, 1)
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			require.Equal(t, 1, s.CaptureCount())
			err := b51Call(t, b51Ctx(t), c, cl)
			require.Error(t, err)
			assert.EqualError(t, err, b51Refused)
			assert.Equal(t, 1, s.CaptureCount(), "the second call sends nothing")
		})
	}
}

// R3: a chat call and a chat-less call share the one global token, in both orders.
func TestB51_SharesGlobalTokenWithChatCalls(t *testing.T) {
	chat := b51Classes[4]
	for _, cl := range b51ChatLess {
		for _, pair := range [][2]b51Class{{chat, cl}, {cl, chat}} {
			t.Run(pair[0].name+" then "+pair[1].name, func(t *testing.T) {
				s := b51Server(t)
				c := b51Client(t, s, 0.001, 1)
				require.NoError(t, b51Call(t, b51Ctx(t), c, pair[0]))
				err := b51Call(t, b51Ctx(t), c, pair[1])
				require.Error(t, err)
				assert.EqualError(t, err, b51Refused)
				assert.Equal(t, 1, s.CaptureCount())
			})
		}
	}
}

// R4: a chat-less call creates no per-chat limiter; "<nil>" (B-76) and a real chat do.
func TestB51_NoPerChatLimiterWithoutChat(t *testing.T) {
	want := []int{0, 0, 0, 1, 1}
	for i, cl := range b51Classes {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 1000, 100)
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			assert.Equal(t, want[i], c.ChatLimiterCount())
		})
	}
}

// R5: a chat call takes one global token, not two.
func TestB51_ChatCallTakesOneGlobalToken(t *testing.T) {
	s := b51Server(t)
	c := b51Client(t, s, 0.001, 1)
	require.NoError(t, b51Call(t, b51Ctx(t), c, b51Classes[4]))
	assert.Equal(t, 1, s.CaptureCount())
}

// R6: a limiter refusal is not a circuit-breaker failure (the breaker trips at 2 consecutive failures).
func TestB51_RefusalIsNotBreakerFailure(t *testing.T) {
	for _, cl := range b51ChatLess {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := testutil.NewBreakerTestClient(t, s.BaseURL(), sender.WithRateLimit(0, 0),
				sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
			for i := range 4 {
				err := b51Call(t, b51Ctx(t), c, cl)
				require.Error(t, err, "call %d", i+1)
				assert.EqualError(t, err, b51Burst0, "call %d", i+1)
				assert.NotErrorIs(t, err, gobreaker.ErrOpenState, "call %d", i+1)
			}
			assert.Equal(t, 0, s.CaptureCount())
		})
	}
}

// R7: a cancel during the wait ends the wait with context.Canceled.
func TestB51_CancelDuringWait(t *testing.T) {
	for _, cl := range b51ChatLess {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 0.001, 1)
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			time.AfterFunc(100*time.Millisecond, cancel)
			err := b51Call(t, ctx, c, cl)
			assert.Equal(t, context.Canceled, err)
			assert.Equal(t, 1, s.CaptureCount())
		})
	}
}

// R8: concurrent calls of every class (run under -race).
func TestB51_Concurrent(t *testing.T) {
	s := b51Server(t)
	c := b51Client(t, s, 1000, 100)
	const workers, calls = 16, 20
	var wg sync.WaitGroup
	errs := make(chan error, workers*calls)
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range calls {
				ctx, cancel := context.WithTimeout(context.Background(), b51Deadline)
				errs <- b51Classes[(w+i)%len(b51Classes)].call(ctx, c)
				cancel()
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, workers*calls, s.CaptureCount())
}

// R9: a real wait: 10 req/s, burst 1, three calls need two waits of 100 ms (lower bound only).
func TestB51_RealWait(t *testing.T) {
	s := b51Server(t)
	c := b51Client(t, s, 10, 1)
	start := time.Now()
	for range 3 {
		require.NoError(t, b51Call(t, b51Ctx(t), c, b51Classes[0]))
	}
	assert.GreaterOrEqual(t, time.Since(start), 180*time.Millisecond)
	assert.Equal(t, 3, s.CaptureCount())
}

// R11: a chat-less call takes exactly one global token.
func TestB51_OneTokenPerChatLessCall(t *testing.T) {
	for _, cl := range b51ChatLess {
		t.Run(cl.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 0.001, 2)
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			require.NoError(t, b51Call(t, b51Ctx(t), c, cl))
			err := b51Call(t, b51Ctx(t), c, cl)
			require.Error(t, err)
			assert.EqualError(t, err, b51Refused)
			assert.Equal(t, 2, s.CaptureCount())
		})
	}
}

// R12: while the breaker is open, a chat-less call still takes a global token before ErrOpenState (PLAN §8).
func TestB51_BreakerOpenTakesToken(t *testing.T) {
	s := testutil.NewMockServer(t)
	s.On(b51Path("getMe"), func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})
	c := testutil.NewBreakerTestClient(t, s.BaseURL(), sender.WithRateLimit(0.001, 3),
		sender.WithPerChatRateLimit(1000, 100), sender.WithGroupRateLimit(1000, 100))
	getMe := b51Classes[0]
	for i := range 2 {
		err := b51Call(t, b51Ctx(t), c, getMe)
		var apiErr *tg.APIError
		require.ErrorAs(t, err, &apiErr, "call %d", i+1)
		assert.Equal(t, 500, apiErr.Code)
	}
	err := b51Call(t, b51Ctx(t), c, getMe)
	require.ErrorIs(t, err, gobreaker.ErrOpenState, "call 3: the breaker is open")
	err = b51Call(t, b51Ctx(t), c, getMe)
	require.Error(t, err)
	assert.EqualError(t, err, b51Refused, "call 4: call 3 took the last token")
	assert.NotErrorIs(t, err, gobreaker.ErrOpenState)
	assert.Equal(t, 2, s.CaptureCount())
}

// R13 (T1-01): a call blocked in the global wait does not block other callers: each class, with a 5 s deadline, is
// refused at once while another chat-less call waits for the next token.
func TestB51_BlockedWaitDoesNotBlockOthers(t *testing.T) {
	for _, other := range b51Classes {
		t.Run(other.name, func(t *testing.T) {
			s := b51Server(t)
			c := b51Client(t, s, 0.001, 1)
			require.NoError(t, b51Call(t, b51Ctx(t), c, b51Classes[0]))
			bctx, bcancel := context.WithCancel(context.Background())
			blocked := make(chan error, 1)
			go func() { blocked <- b51Classes[0].call(bctx, c) }()
			time.Sleep(200 * time.Millisecond) // let the blocked call reach its wait
			start := time.Now()
			err := b51Call(t, b51Ctx(t), c, other)
			took := time.Since(start)
			bcancel()
			select {
			case berr := <-blocked:
				assert.Equal(t, context.Canceled, berr, "the blocked call ends with its cancel")
			case <-time.After(b51Watchdog):
				t.Fatal("the blocked call did not return after its cancel")
			}
			require.Error(t, err)
			assert.EqualError(t, err, b51Refused)
			assert.Less(t, took, 2*time.Second, "refused at once, not after the blocked call")
			assert.Equal(t, 1, s.CaptureCount())
		})
	}
}

// R14 (T2-01, E-0143): a census of the chat-less methods named in PLAN §2 and §8, plus the two payment answers
// (C1-02): with a zero limiter each one is refused before any request, so none of these methods is exempt from the
// global wait.
func TestB51_ChatLessMethodsCensus(t *testing.T) {
	census := []b51Class{
		{"logOut", func(ctx context.Context, c *sender.Client) error { return c.LogOut(ctx) }},
		{"close", func(ctx context.Context, c *sender.Client) error { return c.CloseBot(ctx) }},
		{"getFile", func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetFile(ctx, "f")
			return err
		}},
		{"getUserProfilePhotos", func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetUserProfilePhotos(ctx, 5000000001)
			return err
		}},
		{"getUserProfileAudios", func(ctx context.Context, c *sender.Client) error {
			_, err := c.GetUserProfileAudios(ctx, 5000000001)
			return err
		}},
		{"answerCallbackQuery", func(ctx context.Context, c *sender.Client) error {
			return c.AnswerCallbackQuery(ctx, sender.AnswerCallbackQueryRequest{CallbackQueryID: "q"})
		}},
		{"answerInlineQuery", func(ctx context.Context, c *sender.Client) error {
			return c.AnswerInlineQuery(ctx, sender.AnswerInlineQueryRequest{InlineQueryID: "q",
				Results: []tg.InlineQueryResult{tg.InlineQueryResultArticle{ID: "1", Title: "t",
					InputMessageContent: tg.InputTextMessageContent{MessageText: "t"}}}})
		}},
		{"setGameScore inline", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SetGameScore(ctx, sender.SetGameScoreRequest{UserID: 5000000001, Score: 1, InlineMessageID: "i"})
			return err
		}},
		{"answerShippingQuery", func(ctx context.Context, c *sender.Client) error {
			return c.AnswerShippingQuery(ctx, sender.AnswerShippingQueryRequest{ShippingQueryID: "q", ErrorMessage: "e"})
		}},
		{"answerPreCheckoutQuery", func(ctx context.Context, c *sender.Client) error {
			return c.AnswerPreCheckoutQuery(ctx, sender.AnswerPreCheckoutQueryRequest{PreCheckoutQueryID: "q", OK: true})
		}},
	}
	for _, cl := range census {
		t.Run(cl.name, func(t *testing.T) {
			s := testutil.NewMockServer(t) // no handler: a request that gets through gets the mock's default success and is counted
			c := b51Client(t, s, 0, 0)
			err := b51Call(t, b51Ctx(t), c, cl)
			require.Error(t, err)
			assert.EqualError(t, err, b51Burst0)
			assert.Equal(t, 0, s.CaptureCount(), "no request")
			assert.Equal(t, 0, c.ChatLimiterCount(), "no per-chat limiter")
		})
	}
}
