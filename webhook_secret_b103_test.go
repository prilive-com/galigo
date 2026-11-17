package galigo_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

const (
	b103Token  = "123456789:B103-FAKE-token-root-AAAAAAAAAAAAA"
	b103NewErr = "galigo: validation: webhook_secret - must be 1-256 characters, only A-Z, a-z, 0-9, _ and -"
	b103Good   = "Az9_-ok"
	b103Good2  = "B103-other_9"
	b103Child  = "GALIGO_B103_CHILD" // names the test that runs as a child process
)

// b103Rows is the shared matrix plus the empty secret (the check is off).
func b103Rows() []testutil.WebhookSecretCase {
	return append(testutil.WebhookSecretMatrix(), testutil.WebhookSecretCase{Name: "empty", Secret: "", OK: true})
}

// b103Opts returns the options that end in webhook mode with secret, or, with polling, in long polling mode with
// secret still set (WithPolling does not clear it).
func b103Opts(secret string, polling bool) []galigo.Option {
	if polling {
		return []galigo.Option{galigo.WithWebhook(8443, secret), galigo.WithPolling(30, 100)}
	}
	return []galigo.Option{galigo.WithWebhook(8443, secret)}
}

// b103Refused checks that New refused: a nil Bot and exactly the AC2 error.
func b103Refused(t *testing.T, b *galigo.Bot, err error) {
	t.Helper()
	assert.Nil(t, b)
	var ve *tg.ValidationError
	require.True(t, errors.As(err, &ve), "error %v is not a *tg.ValidationError", err)
	assert.Equal(t, b103NewErr, err.Error())
}

// b103Built checks that New built a Bot whose webhook handler keeps want as its secret: only want is accepted, and a
// refused request delivers nothing.
func b103Built(t *testing.T, b *galigo.Bot, err error, want string) {
	t.Helper()
	require.NoError(t, err)
	require.NotNil(t, b)
	t.Cleanup(func() { _ = b.Close() })
	updates := b.Updates()
	testutil.AssertWebhookAuth(t, b.WebhookHandler(), func() int {
		n := len(updates)
		for range n {
			<-updates
		}
		return n
	}, want)
}

// TestB103_New: in both final modes, New refuses a non-empty WithWebhook secret outside Telegram's rule with a nil Bot
// and the AC2 error, and builds a Bot for every other secret (AC2, AC4). It logs nothing at any level while it builds
// or refuses. In webhook mode the Bot's handler keeps the secret, and after the requests the whole record list is
// exactly the records of the requests.
func TestB103_New(t *testing.T) {
	for _, mode := range []string{"webhook", "polling"} {
		t.Run(mode, func(t *testing.T) {
			for _, c := range b103Rows() {
				t.Run(c.Name, func(t *testing.T) {
					rec := testutil.NewLogRecorder()
					opts := append(b103Opts(c.Secret, mode == "polling"), galigo.WithLogger(slog.New(rec)))
					b, err := galigo.New(b103Token, opts...)
					assert.Nil(t, rec.Records(), "no record while New runs")
					switch {
					case !c.OK:
						b103Refused(t, b, err)
					case mode == "polling":
						require.NoError(t, err)
						require.NotNil(t, b)
						assert.Nil(t, b.WebhookHandler())
						assert.NoError(t, b.Close())
					default:
						b103Built(t, b, err, c.Secret)
						assert.Equal(t, testutil.WebhookAuthLog(c.Secret), rec.Records(), "records after the requests")
					}
				})
			}
		})
	}
}

// TestB103_NewOptionOrder: New checks the secret left after all options: the last WithWebhook wins (also an empty
// one, also after an empty one, also between two good ones), and WithPolling does not clear it (AC2).
func TestB103_NewOptionOrder(t *testing.T) {
	for _, bad := range []string{"[REDACTED]", strings.Repeat("Az9_", 64) + "Z"} {
		t.Run(bad[:3], func(t *testing.T) {
			t.Run("bad_then_polling", func(t *testing.T) {
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, bad), galigo.WithPolling(30, 100))
				b103Refused(t, b, err)
			})
			t.Run("good_then_bad", func(t *testing.T) {
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, b103Good), galigo.WithWebhook(8443, bad))
				b103Refused(t, b, err)
			})
			t.Run("bad_then_good", func(t *testing.T) {
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, bad), galigo.WithWebhook(8443, b103Good))
				b103Built(t, b, err, b103Good)
			})
			t.Run("bad_then_empty", func(t *testing.T) {
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, bad), galigo.WithWebhook(8443, ""))
				b103Built(t, b, err, "")
			})
			t.Run("empty_then_bad", func(t *testing.T) {
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, ""), galigo.WithWebhook(8443, bad))
				b103Refused(t, b, err)
			})
		})
	}
	t.Run("good_then_empty", func(t *testing.T) {
		b, err := galigo.New(b103Token, galigo.WithWebhook(8443, b103Good), galigo.WithWebhook(8443, ""))
		b103Built(t, b, err, "")
	})
	t.Run("empty_then_good", func(t *testing.T) {
		b, err := galigo.New(b103Token, galigo.WithWebhook(8443, ""), galigo.WithWebhook(8443, b103Good))
		b103Built(t, b, err, b103Good)
	})
	t.Run("good_then_other_good", func(t *testing.T) {
		b, err := galigo.New(b103Token, galigo.WithWebhook(8443, b103Good), galigo.WithWebhook(8443, b103Good2))
		b103Built(t, b, err, b103Good2)
	})
}

// TestB103_NewTokenFirst: the existing token check still runs first, so a bad token with a bad secret returns the
// token error (AC2: "after the existing token check").
func TestB103_NewTokenFirst(t *testing.T) {
	for _, token := range []string{"", "no-colon", "abc:def"} {
		b, err := galigo.New(token, galigo.WithWebhook(8443, "[REDACTED]"))
		assert.Nil(t, b)
		assert.ErrorIs(t, err, tg.ErrInvalidToken, "token %q", token)
	}
}

// b103RunChild runs this test binary again with only the test name, and b103Child set to name. It requires exit 0,
// an empty stderr, the marker once, and that every stdout line is one of the test framework's own lines for name
// (Go's exact coverage line included, internal/coverage/cformat/format.go) or the marker: the whole output.
func b103RunChild(t *testing.T, name, marker string) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exe, "-test.run=^"+name+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), b103Child+"="+name)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	require.NoError(t, err, "child stdout:\n%s\nchild stderr:\n%s", stdout.String(), stderr.String())
	assert.Empty(t, stderr.String(), "child stderr")
	allowed := regexp.MustCompile(`^(=== RUN   ` + name + `|--- PASS: ` + name + ` \([0-9]+\.[0-9]{2}s\)|PASS|` +
		`coverage: [0-9]+\.[0-9]% of statements( in \./\.\.\.)?|coverage: \[no statements\]|` +
		`    webhook_secret_b103_test\.go:[0-9]+: ` + regexp.QuoteMeta(marker) + `)$`)
	out := stdout.String()
	assert.Equal(t, 1, strings.Count(out, marker), "marker once in child stdout:\n%s", out)
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		assert.Regexp(t, allowed, line, "child stdout line")
	}
}

// b103CleanupGoroutines counts the goroutines that run the sender's limiter cleanup loop, from a complete dump of
// all stacks.
func b103CleanupGoroutines() int {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), "sender.(*Client).startLimiterCleanup.func1(")
		}
		buf = make([]byte, 2*len(buf))
	}
}

// TestB103_NewStartsNothing: a refused New leaves no sender goroutine running, in both final modes and when a later
// option makes the secret bad (AC2). The count runs in a child process, where no sender existed before: Close
// does not wait for its goroutine to end, so an in-process count could hide a new goroutine behind one that is
// ending. It checks what is left running, not the order of the code: that the check runs before the sender is built
// is the change-stage gate d3-order.
func TestB103_NewStartsNothing(t *testing.T) {
	const marker = "B103 child counts 0 0 1"
	if os.Getenv(b103Child) != t.Name() {
		b103RunChild(t, t.Name(), marker)
		return
	}
	before := b103CleanupGoroutines()
	for _, c := range b103Rows() {
		if c.OK {
			continue
		}
		for _, polling := range []bool{false, true} {
			b, err := galigo.New(b103Token, b103Opts(c.Secret, polling)...)
			b103Refused(t, b, err)
		}
	}
	b, err := galigo.New(b103Token, galigo.WithWebhook(8443, b103Good), galigo.WithWebhook(8443, "[REDACTED]"))
	b103Refused(t, b, err)
	after := b103CleanupGoroutines()
	b, err = galigo.New(b103Token, galigo.WithWebhook(8443, b103Good))
	require.NoError(t, err)
	control := b103CleanupGoroutines()
	_ = b.Close()
	require.Equal(t, [3]int{0, 0, 1}, [3]int{before, after, control}, "goroutines: before, after the refused calls, after one valid New")
	t.Log(marker) // -test.v prints it; the parent looks for it
}

// b103LoadEnv sets every variable LoadConfig validates to a valid value, so the caller's environment cannot change a
// call.
func b103LoadEnv(t *testing.T, mode string) {
	t.Helper()
	for k, v := range map[string]string{"RECEIVER_MODE": mode, "TELEGRAM_BOT_TOKEN": b103Token, "WEBHOOK_URL": "",
		"POLLING_TIMEOUT": "30", "POLLING_LIMIT": "100", "UPDATE_DELIVERY_POLICY": "block"} {
		t.Setenv(k, v)
	}
}

// b103Capture runs f with os.Stdout and os.Stderr replaced by pipes and returns what f wrote to each.
func b103Capture(t *testing.T, f func()) (string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	var out, errb bytes.Buffer
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(&out, outR); done <- struct{}{} }()
	go func() { _, _ = io.Copy(&errb, errR); done <- struct{}{} }()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	func() {
		defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
		f()
	}()
	_ = outW.Close()
	_ = errW.Close()
	<-done
	<-done
	_ = outR.Close()
	_ = errR.Close()
	return out.String(), errb.String()
}

// b103NewQuiet runs New with opts three times, with no logger, with WithLogger first and with WithLogger last. Each
// call must succeed exactly when ok, log nothing to the custom logger, and is closed when built.
func b103NewQuiet(t *testing.T, ok bool, opts ...galigo.Option) {
	t.Helper()
	custom := testutil.NewLogRecorder()
	l := galigo.WithLogger(slog.New(custom))
	for i, o := range [][]galigo.Option{opts, append([]galigo.Option{l}, opts...), append(slices.Clone(opts), l)} {
		b, err := galigo.New(b103Token, o...)
		if assert.Equal(t, ok, err == nil, "New shape %d: %v", i, err) && err == nil {
			assert.NoError(t, b.Close())
		}
	}
	assert.Nil(t, custom.Records(), "custom logger")
}

// b103Handlers returns the constructor testutil.AssertWebhookHandlerLog needs: a handler for secret made with logger
// (nil: slog.Default()), with DefaultConfig or, for other, with the AssertWebhookOther settings.
func b103Handlers(logger *slog.Logger, secret string) func(bool, chan tg.Update) http.Handler {
	return func(other bool, updates chan tg.Update) http.Handler {
		cfg := receiver.DefaultConfig()
		cfg.WebhookSecret = tg.SecretToken(secret)
		if other {
			cfg.AllowedDomain = testutil.WebhookOtherHost
			cfg.MaxBodySize = testutil.WebhookOtherMaxBody
			cfg.RateLimitRequests = 0
			cfg.RateLimitBurst = testutil.WebhookOtherBurst
		}
		return receiver.NewWebhookHandler(logger, updates, cfg)
	}
}

// TestB103_NoOtherOutput: the B-103 entry points write nothing outside the logger they are given (AC4: the
// whole output, not a list of forbidden strings, on every path). In a child process, with slog.Default() replaced
// by a recorder and os.Stdout and os.Stderr replaced by pipes, it runs over the matrix: receiver.LoadConfig in both
// modes; the testbot's config.Load (it keeps the value as the environment returns it); New in both final modes, with a
// later good or empty WithWebhook, after a good one, and with three bad tokens (with no logger and with WithLogger),
// each with no logger, WithLogger first and WithLogger last; NewWebhookHandler with a custom logger. The default
// recorder must stay empty. Then, with a fresh default recorder per secret: NewWebhookHandler with a nil logger, and,
// for an accepted secret, a Bot built without WithLogger, whose handler gets the requests of testutil.AssertWebhookAuth
// and whose whole record list must be testutil.WebhookAuthLog. Both handler kinds also get the requests and the
// whole-record checks of testutil.AssertWebhookHandlerLog. The pipes must stay empty, and the parent
// accepts only empty child stderr and the test framework's own stdout lines.
func TestB103_NoOtherOutput(t *testing.T) {
	const marker = "B103 child output checked"
	if os.Getenv(b103Child) != t.Name() {
		b103RunChild(t, t.Name(), marker)
		return
	}
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	def := testutil.NewLogRecorder()
	slog.SetDefault(slog.New(def))
	rows := b103Rows()
	stdout, stderr := b103Capture(t, func() {
		for _, c := range rows {
			if strings.IndexByte(c.Secret, 0) >= 0 {
				continue // a NUL byte cannot be set in the environment
			}
			for _, mode := range []string{"webhook", "longpolling"} {
				b103LoadEnv(t, mode)
				t.Setenv("WEBHOOK_SECRET", c.Secret)
				_, err := receiver.LoadConfig()
				assert.Equal(t, c.OK, err == nil, "LoadConfig %s %s: %v", mode, c.Name, err)
			}
			t.Setenv("TESTBOT_TOKEN", b103Token)
			t.Setenv("TESTBOT_CHAT_ID", "-1001")
			t.Setenv("TESTBOT_ADMINS", "1")
			t.Setenv("TESTBOT_WEBHOOK_SECRET_TOKEN", c.Secret)
			want := os.Getenv("TESTBOT_WEBHOOK_SECRET_TOKEN") // read before Load, so Load cannot change what is compared
			if utf8.ValidString(c.Secret) {
				assert.Equal(t, c.Secret, want, "environment round trip %s", c.Name)
			}
			cfg, err := config.Load()
			if assert.NoError(t, err, "testbot Load %s", c.Name) {
				assert.Equal(t, want, cfg.WebhookSecretToken.Value(), "testbot Load %s", c.Name)
			}
		}
		for _, c := range rows {
			b103NewQuiet(t, c.OK, b103Opts(c.Secret, false)...)
			b103NewQuiet(t, c.OK, b103Opts(c.Secret, true)...)
			b103NewQuiet(t, true, galigo.WithWebhook(8443, c.Secret), galigo.WithWebhook(8443, b103Good))
			b103NewQuiet(t, c.OK, galigo.WithWebhook(8443, b103Good), galigo.WithWebhook(8443, c.Secret))
			b103NewQuiet(t, true, galigo.WithWebhook(8443, c.Secret), galigo.WithWebhook(8443, ""))
			custom := testutil.NewLogRecorder()
			for _, token := range []string{"", "no-colon", "abc:def"} {
				for _, opts := range [][]galigo.Option{{galigo.WithWebhook(8443, c.Secret)},
					{galigo.WithWebhook(8443, c.Secret), galigo.WithLogger(slog.New(custom))}} {
					b, err := galigo.New(token, opts...)
					assert.Nil(t, b)
					assert.ErrorIs(t, err, tg.ErrInvalidToken, "token %q, secret %s", token, c.Name)
				}
			}
			assert.Nil(t, custom.Records(), "bad token, custom logger, secret %s", c.Name)
			testutil.AssertWebhookHandlerLog(t, custom, b103Handlers(slog.New(custom), c.Secret), c)
		}
		assert.Nil(t, def.Records(), "slog.Default() received a record")
		for _, c := range rows {
			d := testutil.NewLogRecorder()
			slog.SetDefault(slog.New(d))
			testutil.AssertWebhookHandlerLog(t, d, b103Handlers(nil, c.Secret), c)
			if c.OK {
				d = testutil.NewLogRecorder()
				slog.SetDefault(slog.New(d))
				b, err := galigo.New(b103Token, galigo.WithWebhook(8443, c.Secret))
				b103Built(t, b, err, c.Secret)
				assert.Equal(t, testutil.WebhookAuthLog(c.Secret), d.Records(), "Bot without WithLogger, secret %s", c.Name)
			}
		}
	})
	assert.Empty(t, stdout, "stdout of the calls")
	assert.Empty(t, stderr, "stderr of the calls")
	t.Log(marker) // -test.v prints it; the parent looks for it
}
