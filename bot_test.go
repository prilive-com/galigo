package galigo

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

func TestBotClose_Idempotent_PollingMode(t *testing.T) {
	// Create bot in polling mode (default)
	bot, err := New("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ",
		WithPolling(30, 100),
	)
	require.NoError(t, err)

	// First close should succeed
	err = bot.Close()
	assert.NoError(t, err)

	// Second close should be a no-op, not panic
	err = bot.Close()
	assert.NoError(t, err)

	// Third close should also be a no-op
	err = bot.Close()
	assert.NoError(t, err)
}

func TestBotClose_Idempotent_WebhookMode(t *testing.T) {
	// Create bot in webhook mode
	bot, err := New("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ",
		WithWebhook(8443, "secret"),
	)
	require.NoError(t, err)

	// In webhook mode, receiver is nil
	assert.Nil(t, bot.receiver)
	assert.NotNil(t, bot.webhook)

	// First close should succeed
	err = bot.Close()
	assert.NoError(t, err)

	// Second close should be a no-op, not panic
	err = bot.Close()
	assert.NoError(t, err)
}

func TestBotClose_Concurrent(t *testing.T) {
	// Create bot in polling mode
	bot, err := New("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ",
		WithPolling(30, 100),
	)
	require.NoError(t, err)

	// 100 goroutines closing simultaneously — must not panic
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = bot.Close()
		}()
	}

	// Wait for all goroutines — no panic means success
	wg.Wait()
}

func TestBotClose_WebhookMode_ChannelStaysOpen(t *testing.T) {
	// Create bot in webhook mode
	bot, err := New("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ",
		WithWebhook(8443, "secret"),
	)
	require.NoError(t, err)

	// Close the bot
	err = bot.Close()
	assert.NoError(t, err)

	// In webhook mode, channel should NOT be closed.
	// We verify this by accessing the internal updates channel directly
	// and doing a non-blocking send (would panic if closed).
	select {
	case bot.updates <- tg.Update{}:
		// Send succeeded — channel is open as expected
	default:
		// Channel is full, but open — that's fine
	}

	// The real test is that Close() didn't panic when called
	// and a subsequent webhook HTTP handler wouldn't panic on send
}

func TestBotClose_PollingMode_ChannelClosed(t *testing.T) {
	// Create bot in polling mode
	bot, err := New("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ",
		WithPolling(30, 100),
	)
	require.NoError(t, err)

	updates := bot.Updates()

	// Close the bot
	err = bot.Close()
	assert.NoError(t, err)

	// In polling mode, channel should be closed
	// Range loop would exit immediately
	var count int
	for range updates {
		count++
	}
	assert.Equal(t, 0, count, "channel should be closed and empty")
}
