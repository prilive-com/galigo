package sender_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_429WithRetryAfter(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// First attempt: rate limited
			testutil.ReplyRateLimit(w, 5)
			return
		}
		// Second attempt: success
		testutil.ReplyMessage(w, 123)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 123, msg.MessageID)
	assert.Equal(t, int32(2), attempts.Load(), "should have made 2 attempts")
	assert.Equal(t, 1, sleeper.CallCount(), "should have slept once")
	assert.Equal(t, 5*time.Second, sleeper.LastCall(), "should sleep for retry_after duration")
}

func TestRetry_429WithRetryAfterHTTPHeaderFallback(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// First attempt: rate limited (header only, no JSON body param)
			testutil.ReplyRateLimitHeaderOnly(w, 3)
			return
		}
		// Second attempt: success
		testutil.ReplyMessage(w, 456)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 456, msg.MessageID)
	assert.Equal(t, int32(2), attempts.Load(), "should have made 2 attempts")
	assert.Equal(t, 1, sleeper.CallCount(), "should have slept once")
	assert.Equal(t, 3*time.Second, sleeper.LastCall(), "should sleep for HTTP header retry_after duration")
}

func TestRetry_429MultipleRetries(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count <= 2 {
			// First two attempts: rate limited
			testutil.ReplyRateLimit(w, 2)
			return
		}
		// Third attempt: success
		testutil.ReplyMessage(w, 456)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 456, msg.MessageID)
	assert.Equal(t, int32(3), attempts.Load())
	assert.Equal(t, 2, sleeper.CallCount())
	// Both sleeps should be 2 seconds (retry_after value)
	assert.Equal(t, 2*time.Second, sleeper.CallAt(0))
	assert.Equal(t, 2*time.Second, sleeper.CallAt(1))
}

func TestRetry_5xxWithExponentialBackoff(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			// First attempt: server error
			testutil.ReplyServerError(w, 502, "Bad Gateway")
			return
		}
		// Second attempt: success
		testutil.ReplyMessage(w, 789)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 789, msg.MessageID)
	assert.Equal(t, int32(2), attempts.Load())
	assert.Equal(t, 1, sleeper.CallCount())
	// Should use exponential backoff (base ~1s)
	sleepDuration := sleeper.LastCall()
	assert.GreaterOrEqual(t, sleepDuration, 800*time.Millisecond, "backoff should be at least 800ms")
	assert.LessOrEqual(t, sleepDuration, 1500*time.Millisecond, "backoff should be at most 1.5s")
}

func TestRetry_5xxMultipleRetries(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count <= 2 {
			testutil.ReplyServerError(w, 503, "Service Unavailable")
			return
		}
		testutil.ReplyMessage(w, 111)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 111, msg.MessageID)
	assert.Equal(t, int32(3), attempts.Load())
	assert.Equal(t, 2, sleeper.CallCount())
	// Second backoff should be larger than first (exponential)
	// With jitter this can vary, but second should generally be >= first
	first := sleeper.CallAt(0)
	second := sleeper.CallAt(1)
	assert.LessOrEqual(t, first, 1500*time.Millisecond, "first backoff should be ~1s")
	assert.GreaterOrEqual(t, second, first/2, "second backoff should be roughly >= first/2 (accounting for jitter)")
}

func TestRetry_NoRetryOn4xx(t *testing.T) {
	tests := []struct {
		name     string
		response func(w http.ResponseWriter, r *http.Request)
		sentinel error
	}{
		{
			name: "400 bad request",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyBadRequest(w, "chat not found")
			},
			sentinel: sender.ErrChatNotFound,
		},
		{
			name: "401 unauthorized",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyError(w, 401, "Unauthorized", nil)
			},
			sentinel: sender.ErrUnauthorized,
		},
		{
			name: "403 forbidden",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyForbidden(w, "bot was blocked by the user")
			},
			sentinel: sender.ErrBotBlocked,
		},
		{
			name: "404 not found",
			response: func(w http.ResponseWriter, r *http.Request) {
				testutil.ReplyNotFound(w, "message to edit not found")
			},
			sentinel: sender.ErrMessageNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32

			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				tt.response(w, r)
			})

			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
				ChatID: testutil.TestChatID,
				Text:   "Hello",
			})

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.sentinel)
			assert.Equal(t, int32(1), attempts.Load(), "should not retry 4xx errors")
			assert.Equal(t, 0, sleeper.CallCount(), "should not sleep on non-retryable errors")
		})
	}
}

func TestRetry_ContextCancelStopsRetry(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		testutil.ReplyRateLimit(w, 60) // Long retry
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after first attempt
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := client.SendMessage(ctx, sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	// Context cancel during sleep returns context.Canceled
	assert.True(t, errors.Is(err, context.Canceled),
		"expected context.Canceled, got: %v", err)
	// Should exit quickly (not wait 60s retry_after)
	assert.Less(t, elapsed, 500*time.Millisecond, "should exit quickly on cancel")
}

func TestRetry_ContextDeadlineStopsRetry(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		testutil.ReplyServerError(w, 502, "Bad Gateway")
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.SendMessage(ctx, sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
}

func TestRetry_MaxRetriesExceeded(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sender.ErrMaxRetries)
	// With 3 max retries: 1 initial + 3 retries = 4 attempts
	assert.Equal(t, int32(4), attempts.Load())
	assert.Equal(t, 3, sleeper.CallCount())
}

func TestRetry_AllRetriesExhausted(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		testutil.ReplyRateLimit(w, 1)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, sender.ErrMaxRetries)
	// Error message should contain the last error info
	assert.Contains(t, err.Error(), "429")
}

func TestRetry_SuccessOnLastAttempt(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		count := attempts.Add(1)
		if count < 4 {
			testutil.ReplyServerError(w, 500, "Internal Server Error")
			return
		}
		// Success on 4th attempt (last one)
		testutil.ReplyMessage(w, 999)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 999, msg.MessageID)
	assert.Equal(t, int32(4), attempts.Load())
	assert.Equal(t, 3, sleeper.CallCount())
}

func TestRetry_NoRetriesConfigured(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	sleeper := &testutil.FakeSleeper{}
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithSleeper(sleeper),
		sender.WithRetries(0),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), attempts.Load(), "should only make one attempt with 0 retries")
	assert.Equal(t, 0, sleeper.CallCount(), "should not sleep")
}

func TestRetry_SleeperRespectsCancelledContext(t *testing.T) {
	sleeper := &testutil.FakeSleeper{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := sleeper.Sleep(ctx, time.Hour)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, sleeper.CallCount(), "should not record cancelled sleep")
}

func TestRetry_TimeBetweenAttempts(t *testing.T) {
	var attempts atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			testutil.ReplyRateLimit(w, 1)
			return
		}
		testutil.ReplyMessage(w, 123)
	})

	sleeper := &testutil.FakeSleeper{}
	client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, sleeper.CallCount())
	assert.Equal(t, 1*time.Second, sleeper.TotalDuration())
}

func TestRetry_RetryableStatusCodes(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{429, true},  // Rate limited
		{500, true},  // Internal server error
		{502, true},  // Bad gateway
		{503, true},  // Service unavailable
		{504, true},  // Gateway timeout
		{400, false}, // Bad request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not found
		{505, false}, // HTTP version not supported (> 504)
	}

	for _, tt := range tests {
		t.Run("status_"+string(rune('0'+tt.statusCode/100))+string(rune('0'+(tt.statusCode/10)%10))+string(rune('0'+tt.statusCode%10)), func(t *testing.T) {
			var attempts atomic.Int32

			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
				count := attempts.Add(1)
				if count == 1 {
					if tt.statusCode == 429 {
						testutil.ReplyRateLimit(w, 1)
					} else {
						testutil.ReplyServerError(w, tt.statusCode, "Test error")
					}
					return
				}
				testutil.ReplyMessage(w, 1)
			})

			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRetries(3))

			_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
				ChatID: testutil.TestChatID,
				Text:   "Hello",
			})

			if tt.retryable {
				require.NoError(t, err, "expected success after retry for status %d", tt.statusCode)
				assert.GreaterOrEqual(t, attempts.Load(), int32(2), "should have retried for status %d", tt.statusCode)
			} else {
				require.Error(t, err, "expected error without retry for status %d", tt.statusCode)
				assert.Equal(t, int32(1), attempts.Load(), "should not retry for status %d", tt.statusCode)
			}
		})
	}
}
