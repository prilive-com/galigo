package receiver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 updates through the real receiver paths (B-65 65d, TEST-PLAN §4): the webhook handler and a running
// PollingClient, read from its updates channel. Ids are fake.

const (
	b65dStopBody = `{"update_id":1,"stopped_message_generation":{"chat":{"id":987654321,"type":"private"},` + // C3-03: a draft's chat
		`"message_thread_id":7,"draft_id":9}}`
	b65dJoinedBody = `{"update_id":2,"message":{"message_id":42,"date":1,"chat":{"id":-1009876543210,"type":"supergroup"},` +
		`"community_chat_joined":{"community":{"id":4503599627370495,"name":"c"}}}}`
)

func b65dWantStop() tg.Update {
	return tg.Update{UpdateID: 1, StoppedMessageGeneration: &tg.MessageGenerationStopped{
		Chat: &tg.Chat{ID: 987654321, Type: "private"}, MessageThreadID: 7, DraftID: 9}}
}

func b65dWantJoined() tg.Update {
	return tg.Update{UpdateID: 2, Message: &tg.Message{MessageID: 42, Date: 1, Chat: &tg.Chat{ID: -1009876543210, Type: "supergroup"},
		CommunityChatJoined: &tg.CommunityChatJoined{Community: tg.Community{ID: 4503599627370495, Name: "c"}}}}
}

// b65dPoll runs a PollingClient against a getUpdates mock that returns body once, then nothing, and returns the
// first update from the updates channel.
func b65dPoll(t *testing.T, body string) tg.Update {
	t.Helper()
	return b65dPollN(t, body, 1)[0]
}

// b65dPollN is b65dPoll for a body of n updates: it returns the first n updates from the channel, in order (B-65 65e).
func b65dPollN(t *testing.T, body string, n int) []tg.Update {
	t.Helper()
	var served atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:abc/getUpdates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if served.CompareAndSwap(false, true) {
			_, _ = w.Write([]byte(`{"ok":true,"result":[` + body + `]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL + "/bot"
	cfg.PollingTimeout = 0
	updates := make(chan tg.Update, 1)
	c := NewPollingClient(tg.SecretToken("123:abc"), updates, slog.New(slog.DiscardHandler), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, c.Start(ctx))
	defer c.Stop()
	got := make([]tg.Update, 0, n)
	for len(got) < n {
		select {
		case u := <-updates:
			got = append(got, u)
		case <-time.After(10 * time.Second):
			t.Fatalf("%d of %d updates from the polling client", len(got), n)
			return nil
		}
	}
	return got
}

// T-recv-stop
func TestB65D_ReceiveStopped(t *testing.T) {
	assert.Equal(t, b65dWantStop(), api102aWebhook(t, b65dStopBody), "webhook")
	assert.Equal(t, b65dWantStop(), b65dPoll(t, b65dStopBody), "polling")
}

// T-recv-joined
func TestB65D_ReceiveJoined(t *testing.T) {
	assert.Equal(t, b65dWantJoined(), api102aWebhook(t, b65dJoinedBody), "webhook")
	assert.Equal(t, b65dWantJoined(), b65dPoll(t, b65dJoinedBody), "polling")
}
