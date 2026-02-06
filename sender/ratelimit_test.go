package sender_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
)

func TestRateLimit_GlobalLimiter(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create client with low rate limit (2 RPS, burst 1)
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(2, 1),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Send 3 requests - should take at least 500ms due to rate limiting
	start := time.Now()
	for range 3 {
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// With 2 RPS and burst 1, 3 requests should take ~1s (first immediate, then 500ms each)
	assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond, "rate limiting should throttle requests")
}

func TestRateLimit_PerChatLimiter(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create client with per-chat rate limit
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(100, 100),    // High global limit
		sender.WithPerChatRateLimit(2, 1), // Low per-chat limit
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Send 3 requests to same chat - should be throttled
	start := time.Now()
	for range 3 {
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 400*time.Millisecond, "per-chat rate limiting should throttle")
}

func TestRateLimit_DifferentChatsNotThrottled(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create client with per-chat rate limit
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(100, 100),    // High global limit
		sender.WithPerChatRateLimit(1, 1), // 1 RPS per chat
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Send to different chats - should not be throttled by per-chat limiter
	start := time.Now()
	for i := range 5 {
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: int64(1000 + i), // Different chat each time
			Text:   "Hello",
		})
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// Different chats shouldn't throttle each other
	assert.Less(t, elapsed, 500*time.Millisecond, "different chats should not throttle each other")
}

func TestRateLimit_ChatLimiterCount(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Initially no limiters
	assert.Equal(t, 0, client.ChatLimiterCount())

	// Send to different chats
	for i := range 5 {
		client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: int64(1000 + i),
			Text:   "Hello",
		})
	}

	// Should have 5 chat limiters
	assert.Equal(t, 5, client.ChatLimiterCount())

	// Sending to same chat doesn't create new limiter
	client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: int64(1000),
		Text:   "Hello again",
	})
	assert.Equal(t, 5, client.ChatLimiterCount())
}

func TestRateLimit_ContextCancellation(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	// Create client with very low rate limit
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(0.1, 1), // Very slow: 1 request per 10 seconds
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// First request uses burst
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)

	// Second request with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = client.SendMessage(ctx, sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	// Rate limiter returns error when context deadline would be exceeded
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline"), "expected context-related error, got: %v", err)
}

func TestRateLimit_ConcurrentRequests(t *testing.T) {
	var requestCount atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		testutil.ReplyMessage(w, 1)
	})

	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRateLimit(100, 10), // High limit for concurrent test
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Send concurrent requests
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(chatID int64) {
			defer wg.Done()
			client.SendMessage(context.Background(), sender.SendMessageRequest{
				ChatID: chatID,
				Text:   "Hello",
			})
		}(int64(i + 1))
	}
	wg.Wait()

	assert.Equal(t, int32(10), requestCount.Load())
}
