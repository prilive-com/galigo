package sender_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOption_WithLogger(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create custom logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithLogger(logger),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Should work with custom logger
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_WithHTTPClient(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create custom HTTP client with short timeout
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithHTTPClient(httpClient),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Should work with custom HTTP client
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_WithRateLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(10, 5), // 10 RPS, burst 5
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Should work with rate limit
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_WithRetries(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRetries(5),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_WithBaseURL(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)

	// Verify request went to mock server
	assert.Equal(t, 1, server.CaptureCount())
}

func TestOption_WithSleeper(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	sleeper := &testutil.FakeSleeper{}

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithSleeper(sleeper),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)

	// No retries, so no sleep calls
	assert.Equal(t, 0, sleeper.CallCount())
}

func TestOption_WithPerChatRateLimit(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithPerChatRateLimit(5, 2), // 5 RPS per chat, burst 2
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_WithCircuitBreakerSettings(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	settings := testutil.CircuitBreakerNeverTrip()

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithCircuitBreakerSettings(settings),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestOption_MultipleOptions(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	sleeper := &testutil.FakeSleeper{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithLogger(logger),
		sender.WithRateLimit(50, 10),
		sender.WithPerChatRateLimit(5, 2),
		sender.WithRetries(3),
		sender.WithSleeper(sleeper),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestNew_InvalidToken(t *testing.T) {
	_, err := sender.New("")
	assert.ErrorIs(t, err, sender.ErrInvalidToken)
}

func TestNewFromConfig(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	cfg := sender.DefaultConfig()
	cfg.Token = testutil.TestToken
	cfg.BaseURL = server.BaseURL()

	client, err := sender.NewFromConfig(cfg,
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
}

func TestNewFromConfig_InvalidToken(t *testing.T) {
	cfg := sender.DefaultConfig()
	// Token not set

	_, err := sender.NewFromConfig(cfg)
	assert.ErrorIs(t, err, sender.ErrInvalidToken)
}

func TestDefaultConfig(t *testing.T) {
	cfg := sender.DefaultConfig()

	assert.Equal(t, "https://api.telegram.org", cfg.BaseURL)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, float64(30), cfg.GlobalRPS)
	assert.Equal(t, 10, cfg.GlobalBurst)
	assert.Equal(t, float64(1), cfg.PerChatRPS)
	assert.Equal(t, 3, cfg.PerChatBurst)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, time.Second, cfg.RetryBaseWait)
	assert.Equal(t, 30*time.Second, cfg.RetryMaxWait)
	assert.Equal(t, 2.0, cfg.RetryFactor)
}

func TestDefaultCircuitBreakerSettings(t *testing.T) {
	settings := sender.DefaultCircuitBreakerSettings()

	assert.Equal(t, uint32(5), settings.MaxRequests)
	assert.Equal(t, 60*time.Second, settings.Interval)
	assert.Equal(t, 30*time.Second, settings.Timeout)
	assert.NotNil(t, settings.ReadyToTrip)
}

func TestClient_Close(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)

	// Use the client
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)

	// Close should not error
	err = client.Close()
	assert.NoError(t, err)
}
