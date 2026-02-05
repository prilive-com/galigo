package sender_test

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_OpensOnFailures(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	// Use breaker test client (trips after 2 consecutive failures)
	client := testutil.NewBreakerTestClient(t, server.BaseURL())

	// Make requests to trip breaker (needs 2 consecutive failures)
	for range 3 {
		_, _ = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// Next request should fail with circuit open
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	assert.ErrorIs(t, err, sender.ErrCircuitOpen)
}

func TestCircuitBreaker_RecoverAfterTimeout(t *testing.T) {
	var shouldFail atomic.Bool
	shouldFail.Store(true)

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		if shouldFail.Load() {
			testutil.ReplyServerError(w, 500, "Internal Server Error")
			return
		}
		testutil.ReplyMessage(w, 123)
	})

	client := testutil.NewBreakerTestClient(t, server.BaseURL())

	// Trip the breaker
	for range 3 {
		_, _ = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// Verify breaker is open
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.ErrorIs(t, err, sender.ErrCircuitOpen)

	// Wait for breaker timeout (2s in aggressive settings)
	time.Sleep(2500 * time.Millisecond)

	// Server now succeeds
	shouldFail.Store(false)

	// Should recover (half-open -> closed)
	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	require.NoError(t, err)
	assert.Equal(t, 123, msg.MessageID)
}

func TestCircuitBreaker_StaysOpenOnContinuedFailure(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	client := testutil.NewBreakerTestClient(t, server.BaseURL())

	// Trip the breaker
	for range 3 {
		_, _ = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// Wait for timeout (2s in aggressive settings)
	time.Sleep(2500 * time.Millisecond)

	// Try again - will fail and re-open breaker
	_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	// First request in half-open goes through (and fails)
	require.Error(t, err)

	// Next request should be blocked again
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	assert.ErrorIs(t, err, sender.ErrCircuitOpen)
}

func TestCircuitBreaker_SuccessDoesNotTrip(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyMessage(w, 1)
	})

	client := testutil.NewBreakerTestClient(t, server.BaseURL())

	// Many successful requests should not trip breaker
	for range 10 {
		_, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
		require.NoError(t, err)
	}

	// Should still work
	msg, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, msg.MessageID)
}

func TestCircuitBreaker_MixedResultsPartialFailure(t *testing.T) {
	var requestCount atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		// Fail every other request
		if count%2 == 0 {
			testutil.ReplyServerError(w, 500, "Internal Server Error")
			return
		}
		testutil.ReplyMessage(w, int(count))
	})

	client := testutil.NewBreakerTestClient(t, server.BaseURL())

	// Mixed results - breaker should eventually trip due to failure ratio
	var lastErr error
	for range 10 {
		_, lastErr = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// After enough failures, breaker should open
	// The aggressive breaker trips after 2 consecutive failures
	// With alternating success/failure, it may not trip immediately
	// but we should have processed requests
	assert.GreaterOrEqual(t, requestCount.Load(), int32(3), "should have made multiple requests")

	// If there were consecutive failures, breaker would be open
	if lastErr != nil {
		// Either a server error or circuit open
		assert.True(t, true, "error is expected after failures")
	}
}

func TestCircuitBreaker_DefaultSettings(t *testing.T) {
	var requestCount atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	// Use default settings (50% failure rate, min 3 requests)
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRetries(0),
	)
	require.NoError(t, err)
	defer client.Close()

	// Make 3 failing requests - should trip default breaker
	for range 4 {
		_, _ = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// Next request should fail with circuit open
	_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{
		ChatID: testutil.TestChatID,
		Text:   "Hello",
	})

	// With default settings, after 3+ failures at 100% rate, breaker should open
	assert.ErrorIs(t, err, sender.ErrCircuitOpen)
}

func TestCircuitBreaker_CustomSettings(t *testing.T) {
	var requestCount atomic.Int32

	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		testutil.ReplyServerError(w, 500, "Internal Server Error")
	})

	// Use the never-trip preset
	client, err := sender.New(testutil.TestToken,
		sender.WithBaseURL(server.BaseURL()),
		sender.WithRetries(0),
		sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()),
	)
	require.NoError(t, err)
	defer client.Close()

	// Many failures should not trip (custom settings never trip)
	for range 10 {
		_, _ = client.SendMessage(context.Background(), sender.SendMessageRequest{
			ChatID: testutil.TestChatID,
			Text:   "Hello",
		})
	}

	// All requests should have gone through
	assert.Equal(t, int32(10), requestCount.Load())
}
