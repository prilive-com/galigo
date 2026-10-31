package receiver_test

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestPolling_RequestCreationError_NoTokenInLog(t *testing.T) {
	const secretPre, secretPost = "ABCdefGHI", "jklMNOpqr"
	token := tg.SecretToken("123456789:" + secretPre + "STUVWXYZabcdefgh" + secretPost)

	logs := &lockedBuffer{}
	cfg := pollingTestConfig()
	cfg.BaseURL = "https://api.telegram.org/bot\x7f"
	cfg.PollingMaxErrors = 1
	c := receiver.NewPollingClient(token, make(chan tg.Update, 1), slog.New(slog.NewTextHandler(logs, nil)), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, c.Start(ctx))
	require.Eventually(t, func() bool { return !c.Running() }, 5*time.Second, 10*time.Millisecond)

	out := logs.String()
	assert.Contains(t, out, "fetch updates failed")
	assert.Contains(t, out, "[REDACTED]")
	assert.NotContains(t, out, secretPre)
	assert.NotContains(t, out, secretPost)
}
