package receiver_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pollingTestConfig() receiver.Config {
	cfg := receiver.DefaultConfig()
	cfg.Mode = receiver.ModeLongPolling
	cfg.PollingTimeout = 1 // 1 second for fast tests
	cfg.PollingLimit = 100
	cfg.PollingMaxErrors = 3
	cfg.RetryInitialDelay = 10 * time.Millisecond
	cfg.RetryMaxDelay = 50 * time.Millisecond
	cfg.RetryBackoffFactor = 2.0
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
	cfg.UpdateDeliveryTimeout = 100 * time.Millisecond
	return cfg
}

func pollingTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, nil))
}

type discardWriter struct{}

func (w *discardWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// ==================== Basic Lifecycle ====================

func TestPolling_NewPollingClient_CreatesClient(t *testing.T) {
	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	require.NotNil(t, client)
	assert.False(t, client.Running())
	assert.Equal(t, int64(0), client.Offset())
	assert.Equal(t, int32(0), client.ConsecutiveErrors())
}

func TestPolling_Start_SetsRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty updates
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	assert.True(t, client.Running())
}

func TestPolling_Start_AlreadyRunning_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Try to start again
	err = client.Start(ctx)
	assert.Error(t, err)
	assert.Equal(t, receiver.ErrAlreadyRunning, err)
}

func TestPolling_Stop_SetsNotRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx := context.Background()
	err := client.Start(ctx)
	require.NoError(t, err)

	assert.True(t, client.Running())

	client.Stop()

	assert.False(t, client.Running())
}

func TestPolling_ContextCancel_StopsPolling(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())

	err := client.Start(ctx)
	require.NoError(t, err)

	// Wait for at least one request
	time.Sleep(50 * time.Millisecond)
	assert.Greater(t, requestCount.Load(), int32(0))

	// Cancel context
	cancel()

	// Wait for stop
	time.Sleep(50 * time.Millisecond)
	assert.False(t, client.Running())
}

// ==================== Offset Handling ====================

func TestPolling_OffsetProgression(t *testing.T) {
	var requestCount atomic.Int32
	var lastOffset atomic.Int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		// Parse offset from query
		offset := r.URL.Query().Get("offset")
		if offset != "" {
			if v, err := parseOffset(offset); err == nil {
				lastOffset.Store(v)
			}
		}

		if count == 1 {
			// First request - return updates 100, 101
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []any{
					map[string]any{"update_id": 100, "message": map[string]any{"message_id": 1, "text": "a"}},
					map[string]any{"update_id": 101, "message": map[string]any{"message_id": 2, "text": "b"}},
				},
			})
		} else {
			// Subsequent requests - empty
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for updates to be delivered
	time.Sleep(100 * time.Millisecond)

	// Check offset advanced to 102 (last update ID + 1)
	assert.Equal(t, int64(102), client.Offset())

	// Verify second request used offset 102
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(102), lastOffset.Load())
}

func parseOffset(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func TestPolling_UpdatesDeliveredToChannel(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []any{
					map[string]any{
						"update_id": 100,
						"message": map[string]any{
							"message_id": 1,
							"text":       "Hello",
							"chat":       map[string]any{"id": 123, "type": "private"},
						},
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Receive update
	select {
	case update := <-updates:
		assert.Equal(t, 100, update.UpdateID)
		require.NotNil(t, update.Message)
		assert.Equal(t, "Hello", update.Message.Text)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for update")
	}
}

// ==================== Error Handling ====================

func TestPolling_ServerError_Retries(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count < 3 {
			// Return server error
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"error_code":  500,
				"description": "Internal Server Error",
			})
		} else {
			// Return success
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.PollingMaxErrors = 5 // Allow retries

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for retries
	time.Sleep(200 * time.Millisecond)

	// Should have made multiple requests
	assert.GreaterOrEqual(t, requestCount.Load(), int32(3))
	assert.True(t, client.Running())
}

func TestPolling_MaxErrorsExceeded_Stops(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  500,
			"description": "Internal Server Error",
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.PollingMaxErrors = 2 // Low threshold

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx := context.Background()
	err := client.Start(ctx)
	require.NoError(t, err)

	// Wait for max errors to be exceeded
	time.Sleep(200 * time.Millisecond)

	assert.False(t, client.Running())
	assert.GreaterOrEqual(t, requestCount.Load(), int32(2))
}

func TestPolling_ConsecutiveErrors_Resets_OnSuccess(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count == 1 {
			// First request fails
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"error_code":  500,
				"description": "Internal Server Error",
			})
		} else {
			// Subsequent requests succeed
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.PollingMaxErrors = 5

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for retry and success
	time.Sleep(200 * time.Millisecond)

	// Error count should be reset to 0
	assert.Equal(t, int32(0), client.ConsecutiveErrors())
}

// ==================== Health Check ====================

func TestPolling_IsHealthy_WhenRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Not healthy before start
	assert.False(t, client.IsHealthy())

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Healthy after start
	assert.True(t, client.IsHealthy())
}

func TestPolling_IsHealthy_UnhealthyWithErrors(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  500,
			"description": "error",
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.PollingMaxErrors = 5

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for errors to accumulate
	time.Sleep(200 * time.Millisecond)

	// Should be unhealthy due to errors (but still running until max)
	if client.ConsecutiveErrors() >= int32(cfg.PollingMaxErrors) {
		assert.False(t, client.IsHealthy())
	}
}

// ==================== Delivery Policies ====================

func TestPolling_DeliveryPolicy_DropNewest(t *testing.T) {
	var requestCount atomic.Int32
	var droppedUpdates []int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count == 1 {
			// Return multiple updates
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []any{
					map[string]any{"update_id": 100},
					map[string]any{"update_id": 101},
					map[string]any{"update_id": 102},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 1) // Small buffer - will overflow
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropNewest
	cfg.OnUpdateDropped = func(id int, reason string) {
		mu.Lock()
		droppedUpdates = append(droppedUpdates, id)
		mu.Unlock()
	}

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for updates to be processed
	time.Sleep(100 * time.Millisecond)

	// Some updates should have been dropped
	mu.Lock()
	dropped := len(droppedUpdates)
	mu.Unlock()

	// With buffer size 1, at least 1 update should be dropped
	assert.GreaterOrEqual(t, dropped, 1, "expected some updates to be dropped")
}

func TestPolling_DeliveryPolicy_DropOldest(t *testing.T) {
	var requestCount atomic.Int32
	var droppedUpdates []int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count == 1 {
			// Return multiple updates
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []any{
					map[string]any{"update_id": 100},
					map[string]any{"update_id": 101},
					map[string]any{"update_id": 102},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update, 1) // Small buffer
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropOldest
	cfg.OnUpdateDropped = func(id int, reason string) {
		mu.Lock()
		droppedUpdates = append(droppedUpdates, id)
		mu.Unlock()
	}

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for updates to be processed
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	dropped := len(droppedUpdates)
	mu.Unlock()

	// With DropOldest policy, old updates should be dropped to make room for new
	assert.GreaterOrEqual(t, dropped, 1, "expected some updates to be dropped")
}

func TestPolling_DeliveryPolicy_Block_Timeout(t *testing.T) {
	var requestCount atomic.Int32
	var droppedUpdates []int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)

		if count == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []any{
					map[string]any{"update_id": 100},
					map[string]any{"update_id": 101},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": []any{},
			})
		}
	}))
	defer server.Close()

	updates := make(chan tg.Update) // Unbuffered - will block
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyBlock
	cfg.UpdateDeliveryTimeout = 10 * time.Millisecond // Very short timeout
	cfg.OnUpdateDropped = func(id int, reason string) {
		mu.Lock()
		droppedUpdates = append(droppedUpdates, id)
		mu.Unlock()
	}

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	// Wait for timeout to trigger
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	dropped := len(droppedUpdates)
	mu.Unlock()

	// Updates should be dropped after timeout
	assert.GreaterOrEqual(t, dropped, 1, "expected updates to be dropped after timeout")
}

// ==================== Options ====================

func TestPollingOption_WithPollingMaxErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":          false,
			"error_code":  500,
			"description": "error",
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingMaxErrors(1), // Override to 1
	)

	ctx := context.Background()
	err := client.Start(ctx)
	require.NoError(t, err)

	// Should stop quickly due to max errors = 1
	time.Sleep(100 * time.Millisecond)

	assert.False(t, client.Running())
}

func TestPollingOption_WithPollingAllowedUpdates(t *testing.T) {
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("allowed_updates")
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingAllowedUpdates([]string{"message", "callback_query"}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, capturedQuery, "message")
	assert.Contains(t, capturedQuery, "callback_query")
}

func TestPollingOption_WithDeliveryPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	// Verify option is accepted without error
	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithDeliveryPolicy(receiver.DeliveryPolicyDropNewest),
	)

	require.NotNil(t, client)
}

func TestPollingOption_WithDeliveryTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	// Verify option is accepted without error
	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithDeliveryTimeout(5*time.Second),
	)

	require.NotNil(t, client)
}

func TestPollingOption_WithUpdateDroppedCallback(t *testing.T) {
	var calledWith int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": []any{
				map[string]any{"update_id": 123},
			},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update) // Unbuffered
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"
	cfg.UpdateDeliveryTimeout = 10 * time.Millisecond

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithUpdateDroppedCallback(func(id int, reason string) {
			calledWith = id
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 123, calledWith)
}

func TestPollingOption_WithPollingHTTPClient(t *testing.T) {
	customClient := &http.Client{Timeout: 5 * time.Second}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingHTTPClient(customClient),
	)

	require.NotNil(t, client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	time.Sleep(50 * time.Millisecond)
	assert.True(t, client.Running())
}

func TestPollingOption_WithPollingCircuitBreaker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	// Create custom breaker
	customBreaker := gobreaker.NewCircuitBreaker[[]byte](gobreaker.Settings{
		Name: "custom-test-breaker",
	})

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingCircuitBreaker(customBreaker),
	)

	require.NotNil(t, client)
}

func TestPollingOption_WithPollingDeleteWebhook(t *testing.T) {
	var deleteCalled atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deleteWebhook") {
			deleteCalled.Store(true)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": true,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	// Use URL-rewriting HTTP client to redirect deleteWebhook API calls
	rewriteClient := &http.Client{
		Transport: &urlRewriteTransport{
			base:      http.DefaultTransport,
			targetURL: server.URL,
		},
	}

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingDeleteWebhook(true),
		receiver.WithPollingHTTPClient(rewriteClient),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	time.Sleep(50 * time.Millisecond)

	assert.True(t, deleteCalled.Load(), "deleteWebhook should have been called")
}

func TestPollingOption_WithPollingRetryConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingRetryConfig(
			100*time.Millisecond,  // initial
			1*time.Second,         // max
			1.5,                   // factor
		),
	)

	require.NotNil(t, client)
}

func TestPolling_Start_DeleteWebhookFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "deleteWebhook") {
			json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"error_code":  401,
				"description": "Unauthorized",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"result": []any{},
		})
	}))
	defer server.Close()

	updates := make(chan tg.Update, 10)
	cfg := pollingTestConfig()
	cfg.BaseURL = server.URL + "/bot"

	client := receiver.NewPollingClient(
		tg.SecretToken("test:token"),
		updates,
		pollingTestLogger(),
		cfg,
		receiver.WithPollingDeleteWebhook(true),
	)

	ctx := context.Background()
	err := client.Start(ctx)

	// Should fail because deleteWebhook failed
	assert.Error(t, err)
	assert.False(t, client.Running())
}
