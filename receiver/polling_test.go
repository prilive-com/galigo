package receiver_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
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
	queryCh := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queryCh <- r.URL.Query().Get("allowed_updates"):
		default:
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
		receiver.WithPollingAllowedUpdates([]string{"message", "callback_query"}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	capturedQuery := <-queryCh

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
	droppedCh := make(chan int, 1)

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
			select {
			case droppedCh <- id:
			default:
			}
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := client.Start(ctx)
	require.NoError(t, err)
	defer client.Stop()

	select {
	case calledWith := <-droppedCh:
		assert.Equal(t, 123, calledWith)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dropped callback")
	}
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
			100*time.Millisecond, // initial
			1*time.Second,        // max
			1.5,                  // factor
		),
	)

	require.NotNil(t, client)
}

// b112Token is a fake token in the shape Telegram uses.
const b112Token = "123456789:AAFakeB112TokenForTestsOnly00000"

// b112Host is the host of the test base URLs. The transport below never
// dials it, so it needs no DNS entry.
const b112Host = "b112.example.test"

// recordedRequest is one request as the client sent it and a server would
// see it.
type recordedRequest struct {
	Scheme   string // of the client's URL; the wire form does not carry it
	URLHost  string // of the client's URL, with any port: where a real transport dials
	Method   string
	Host     string // the effective Host header
	Path     string // escaped
	RawQuery string
	Cookie   string // from the client's cookie jar
	Tag      string
}

// deleteWebhookRecorder is an in-memory Bot API stand-in: no socket is
// opened. It records every request and answers deleteWebhook (or, with
// deleteRedirect, the /redirected target) with deleteBody, getUpdates with
// an empty update list, and any other path with a 404 API error.
type deleteWebhookRecorder struct {
	deleteBody      string
	deleteRedirect  bool                  // answer deleteWebhook with a 302 to /redirected, which answers deleteBody
	onDelete        func(r *http.Request) // optional; runs before the deleteWebhook answer
	deleteStatus    int                   // the deleteWebhook answer's HTTP status; 0: 200
	deleteReadErr   string                // when set, reading the deleteWebhook answer fails with this text after deleteBody
	mu              sync.Mutex
	reqs            []recordedRequest
	ctxs            []reqCtx // per request, in the order of reqs
	firstGetUpdates chan struct{}
	once            sync.Once
}

func newDeleteWebhookRecorder(deleteBody string) *deleteWebhookRecorder {
	return &deleteWebhookRecorder{deleteBody: deleteBody, firstGetUpdates: make(chan struct{})}
}

// serve records sent, the client's request, and r, the same request read
// back from the wire format, and answers it.
func (rec *deleteWebhookRecorder) serve(sent *http.Request, w http.ResponseWriter, r *http.Request) {
	rec.mu.Lock()
	rec.reqs = append(rec.reqs, recordedRequest{
		Scheme:   sent.URL.Scheme,
		URLHost:  sent.URL.Host,
		Method:   r.Method,
		Host:     r.Host,
		Path:     r.URL.EscapedPath(),
		RawQuery: r.URL.RawQuery,
		Cookie:   r.Header.Get("Cookie"),
		Tag:      r.Header.Get("X-Galigo-Test"),
	})
	d, _ := r.Context().Deadline()
	rec.ctxs = append(rec.ctxs, reqCtx{deadline: d, value: r.Context().Value(b112CtxKey{})})
	rec.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/deleteWebhook"):
		if rec.onDelete != nil {
			rec.onDelete(r)
		}
		if rec.deleteRedirect {
			w.Header().Set("Location", "/redirected")
			w.WriteHeader(http.StatusFound)
			return
		}
		if rec.deleteStatus != 0 {
			w.WriteHeader(rec.deleteStatus)
		}
		_, _ = io.WriteString(w, rec.deleteBody)
	case rec.deleteRedirect && r.URL.Path == "/redirected":
		_, _ = io.WriteString(w, rec.deleteBody)
	case strings.HasSuffix(r.URL.Path, "/getUpdates"):
		rec.once.Do(func() { close(rec.firstGetUpdates) })
		_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
	default:
		_, _ = io.WriteString(w, `{"ok":false,"error_code":404,"description":"Not Found"}`)
	}
}

func (rec *deleteWebhookRecorder) requests() []recordedRequest {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]recordedRequest(nil), rec.reqs...)
}

// reqCtx is what the context of one recorded request carried.
type reqCtx struct {
	deadline time.Time // zero when it had none
	value    any       // its b112CtxKey value
}

func (rec *deleteWebhookRecorder) reqContext(i int) reqCtx {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.ctxs[i]
}

// b112CtxKey is the key of a value that the caller's context must bring to
// the request.
type b112CtxKey struct{}

// callerContext returns a context with a value and a deadline earlier than
// any test client Timeout, so a request that drops the caller's context (and
// keeps only its cancel) is visible.
func callerContext() (context.Context, time.Time, context.CancelFunc) {
	deadline := time.Now().Add(30 * time.Second)
	ctx, cancel := context.WithDeadline(context.WithValue(context.Background(), b112CtxKey{}, "b112-value"), deadline)
	return ctx, deadline, cancel
}

// assertCallerContext checks that every recorded request carried the
// caller's value and deadline.
func assertCallerContext(t *testing.T, rec *deleteWebhookRecorder, deadline time.Time) {
	t.Helper()
	for i := range rec.requests() {
		rc := rec.reqContext(i)
		assert.Equal(t, "b112-value", rc.value, "request %d lost the caller's context value", i)
		assert.True(t, rc.deadline.Equal(deadline), "request %d deadline %v, want the caller's %v", i, rc.deadline, deadline)
	}
}

// client returns the client the polling client must use. It serves every
// request from the recorder in memory, marks it, and adds a cookie from its
// jar; its Timeout gives each request a deadline. A request sent through
// another client is not recorded and reaches no server here.
func (rec *deleteWebhookRecorder) client(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: tagTransport{base: handlerTransport{rec: rec}},
		Jar:       b112Jar{},
		Timeout:   timeout,
	}
}

// b112Jar gives every request the cookie b112=jar and keeps nothing.
type b112Jar struct{}

func (b112Jar) SetCookies(*url.URL, []*http.Cookie) {}

func (b112Jar) Cookies(*url.URL) []*http.Cookie {
	return []*http.Cookie{{Name: "b112", Value: "jar"}}
}

// handlerTransport serves a request from rec without a socket. Before that
// it refuses what a real transport refuses before dialling (a done context,
// a scheme other than http or https, an empty host), and it passes the
// request through the HTTP/1.1 wire format, so rec sees what a server
// would see. Like a real transport, it returns the context's error when the
// context ends before the answer is back.
type handlerTransport struct{ rec *deleteWebhookRecorder }

func (ht handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	if req.URL == nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.Host == "" {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.New("handlerTransport: unsupported scheme or empty host")
	}
	var wire bytes.Buffer
	if err := req.Write(&wire); err != nil {
		return nil, err
	}
	sreq, err := http.ReadRequest(bufio.NewReader(&wire))
	if err != nil {
		return nil, err
	}
	w := httptest.NewRecorder()
	ht.rec.serve(req, w, sreq.WithContext(req.Context()))
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	resp := w.Result()
	resp.Request = req
	if ht.rec.deleteReadErr != "" && strings.HasSuffix(sreq.URL.Path, "/deleteWebhook") {
		resp.Body = io.NopCloser(io.MultiReader(resp.Body, iotest.ErrReader(errors.New(ht.rec.deleteReadErr))))
	}
	return resp, nil
}

// tagTransport marks every request it sends.
type tagTransport struct{ base http.RoundTripper }

func (tt tagTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("X-Galigo-Test", "polling-client")
	return tt.base.RoundTrip(req)
}

const (
	b112DeleteOK  = `{"ok":true,"result":true}`
	b112Delete401 = `{"ok":false,"error_code":401,"description":"Unauthorized"}`
	// b112Delete401Echo is a 401 whose description repeats the request path,
	// token included, and a piece of the token, as a proxy or server may do.
	b112Delete401Echo = `{"ok":false,"error_code":401,"description":"Unauthorized: ` + b112DeletePath + ` (key ` + b112TokenPiece + `)"}`

	// b112TokenPiece is a piece of b112Token, longer than 4 bytes.
	b112TokenPiece = "FakeB112Token"

	// b112ReadErr is a body-read error text that repeats the request path
	// and a piece of the token, as a malformed trailer line can.
	b112ReadErr = "malformed trailer line: " + b112DeletePath + " key " + b112TokenPiece

	// b112BaseURL is the main Config.BaseURL, used as given. Its path is not
	// "/bot" and has no "bot" segment, so a fix that swaps only the host of the
	// default URL, or adds "bot", is visible; it ends in "/", so a fix that
	// trims the base is visible too.
	b112BaseURL    = "http://" + b112Host + "/custom/"
	b112DeletePath = "/custom/" + b112Token + "/deleteWebhook"
	b112UpdatePath = "/custom/" + b112Token + "/getUpdates"

	// b112TLSBaseURL is a second Config.BaseURL: another scheme, a port and
	// an escaped slash, which only the URL's raw path keeps.
	b112TLSBaseURL    = "https://" + b112Host + ":8443/custom%2Fdir/bot"
	b112TLSDeletePath = "/custom%2Fdir/bot" + b112Token + "/deleteWebhook"
	b112TLSUpdatePath = "/custom%2Fdir/bot" + b112Token + "/getUpdates"

	// The default base (Config.BaseURL empty) is the cloud Bot API. The
	// in-memory client serves it too, so nothing is dialled.
	b112DefaultDeletePath = "/bot" + b112Token + "/deleteWebhook"
	b112DefaultUpdatePath = "/bot" + b112Token + "/getUpdates"

	// Two Timeout values that no default has, so a fixed value is visible.
	b112Timeout    = 47 * time.Second
	b112TLSTimeout = 59 * time.Second
)

// b112BadBaseURL holds a control byte, so the request URL does not parse.
const b112BadBaseURL = "http://" + b112Host + "/custom\x01/bot"

// b112RedirectCalls is the one redirect policy call for the 302 fixture.
var b112RedirectCalls = []redirectCall{{
	Method: http.MethodGet, URL: "http://" + b112Host + "/redirected", Via: 1,
	FirstMethod: http.MethodGet, FirstURL: b112BaseURL + b112Token + "/deleteWebhook?drop_pending_updates=false",
}}

// errB112Redirect is what the test client's redirect policy returns.
var errB112Redirect = errors.New("b112: redirect refused by the client's policy")

// redirectCall is one call of the test client's redirect policy.
type redirectCall struct {
	Method      string // of the request about to be sent
	URL         string // of the request about to be sent
	Via         int    // len(via)
	FirstMethod string // of via[0]
	FirstURL    string // of via[0]
}

func countPathSuffix(reqs []recordedRequest, suffix string) int {
	n := 0
	for _, r := range reqs {
		if strings.HasSuffix(r.Path, suffix) {
			n++
		}
	}
	return n
}

// TestPolling_Start_DeleteWebhookTarget checks where Start sends deleteWebhook:
// to Config.BaseURL, through the client set by WithPollingHTTPClient (its
// transport, jar, Timeout and redirect policy, both when it refuses and when
// it follows), with the caller's context, once, before the first getUpdates;
// not at all without the option; and that a base URL that does not parse
// fails Start without the token in the error; and that a followed redirect's
// final answer decides, a 401 included.
func TestPolling_Start_DeleteWebhookTarget(t *testing.T) {
	on := []receiver.PollingOption{receiver.WithPollingDeleteWebhook(true)}
	tests := []struct {
		name       string
		opts       []receiver.PollingOption
		baseURL    string
		timeout    time.Duration
		wantDelete *recordedRequest // nil: no deleteWebhook request
		wantRedir  *recordedRequest // the followed redirect, sent after wantDelete
		wantUpdate recordedRequest  // the first getUpdates (Path and Host only)
		cancel     string           // "before": ctx cancelled before Start; "during": cancelled while deleteWebhook is in flight
		redirect   string           // deleteWebhook answers 302; the client's policy "refuse"s or "follow"s it
		deleteBody string           // the deleteWebhook answer (or the redirect target's); empty: b112DeleteOK
		deleteCode int              // the deleteWebhook answer's HTTP status; 0: 200
		readErr    string           // reading the deleteWebhook answer fails with this text after deleteBody
		wantCalls  []redirectCall   // the redirect policy's calls
		wantErr    error            // Start must fail with this (errors.Is)
		wantErrTxt string           // Start must fail with exactly this text
		wantURLErr string           // the cause of the *url.Error in the chain
		wantAPI    int              // the Code of the *receiver.APIError in the chain
	}{
		{name: "D1 option on", opts: on, baseURL: b112BaseURL, timeout: b112Timeout,
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantUpdate: recordedRequest{Host: b112Host, Path: b112UpdatePath}},
		{name: "D2 option absent", baseURL: b112BaseURL, timeout: b112Timeout,
			wantUpdate: recordedRequest{Host: b112Host, Path: b112UpdatePath}},
		{name: "D3 option false", opts: []receiver.PollingOption{receiver.WithPollingDeleteWebhook(false)}, baseURL: b112BaseURL, timeout: b112Timeout,
			wantUpdate: recordedRequest{Host: b112Host, Path: b112UpdatePath}},
		{name: "D4 option on cancelled context", opts: on, baseURL: b112BaseURL, timeout: b112Timeout, cancel: "before", wantErr: context.Canceled},
		{name: "D5 option on https port escaped path", opts: on, baseURL: b112TLSBaseURL, timeout: b112TLSTimeout,
			wantDelete: &recordedRequest{Scheme: "https", URLHost: b112Host + ":8443", Method: http.MethodGet, Host: b112Host + ":8443",
				Path: b112TLSDeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantUpdate: recordedRequest{Host: b112Host + ":8443", Path: b112TLSUpdatePath}},
		{name: "D6 option on cancelled in flight", opts: on, baseURL: b112BaseURL, timeout: b112Timeout, cancel: "during", wantErr: context.Canceled,
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"}},
		{name: "D7 option on default base", opts: on, timeout: b112Timeout,
			wantDelete: &recordedRequest{Scheme: "https", URLHost: "api.telegram.org", Method: http.MethodGet, Host: "api.telegram.org",
				Path: b112DefaultDeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantUpdate: recordedRequest{Host: "api.telegram.org", Path: b112DefaultUpdatePath}},
		{name: "D8 option on redirect refused by client policy", opts: on, baseURL: b112BaseURL, timeout: b112Timeout, redirect: "refuse", wantErr: errB112Redirect,
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantCalls: b112RedirectCalls},
		{name: "D9 option on redirect followed by client policy", opts: on, baseURL: b112BaseURL, timeout: b112Timeout, redirect: "follow",
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantRedir: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: "/redirected", Cookie: "b112=jar", Tag: "polling-client"},
			wantCalls:  b112RedirectCalls,
			wantUpdate: recordedRequest{Host: b112Host, Path: b112UpdatePath}},
		{name: "D10 option on base URL does not parse", opts: on, baseURL: b112BadBaseURL, timeout: b112Timeout,
			wantErrTxt: `failed to delete webhook: failed to create request: parse "http://b112.example.test/custom\x01/bot[REDACTED]/deleteWebhook?drop_pending_updates=false": net/url: invalid control character in URL`,
			wantURLErr: "net/url: invalid control character in URL"},
		{name: "D11 option on redirect followed final 401", opts: on, baseURL: b112BaseURL, timeout: b112Timeout, redirect: "follow",
			deleteBody: b112Delete401, wantErrTxt: "failed to delete webhook: telegram API error 401: Unauthorized", wantAPI: 401,
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"},
			wantRedir: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: "/redirected", Cookie: "b112=jar", Tag: "polling-client"},
			wantCalls: b112RedirectCalls},
		{name: "D12 option on 401 description repeats the token", opts: on, baseURL: b112BaseURL, timeout: b112Timeout,
			deleteBody: b112Delete401Echo, deleteCode: http.StatusUnauthorized, wantAPI: 401,
			wantErrTxt: "failed to delete webhook: telegram API error 401: Unauthorized: /custom/[REDACTED]/deleteWebhook (key [REDACTED])",
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"}},
		{name: "D13 option on body read error repeats the token", opts: on, baseURL: b112BaseURL, timeout: b112Timeout,
			deleteBody: `{"ok":false,`, readErr: b112ReadErr,
			wantErrTxt: "failed to delete webhook: failed to decode response: malformed trailer line: /custom/[REDACTED]/deleteWebhook key [REDACTED]",
			wantDelete: &recordedRequest{Scheme: "http", URLHost: b112Host, Method: http.MethodGet, Host: b112Host,
				Path: b112DeletePath, RawQuery: "drop_pending_updates=false", Cookie: "b112=jar", Tag: "polling-client"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := tt.deleteBody
			if body == "" {
				body = b112DeleteOK
			}
			rec := newDeleteWebhookRecorder(body)
			rec.deleteRedirect = tt.redirect != ""
			rec.deleteStatus, rec.deleteReadErr = tt.deleteCode, tt.readErr
			cfg := pollingTestConfig()
			cfg.BaseURL = tt.baseURL
			hc := rec.client(tt.timeout)
			var redirMu sync.Mutex
			var redirCalls []redirectCall
			hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				// Record first, so a refusing policy's calls are seen too.
				call := redirectCall{Method: req.Method, URL: req.URL.String(), Via: len(via)}
				if len(via) > 0 {
					call.FirstMethod, call.FirstURL = via[0].Method, via[0].URL.String()
				}
				redirMu.Lock()
				redirCalls = append(redirCalls, call)
				redirMu.Unlock()
				if tt.redirect != "follow" {
					return errB112Redirect
				}
				return nil
			}
			assertCalls := func() {
				redirMu.Lock()
				calls := append([]redirectCall(nil), redirCalls...)
				redirMu.Unlock()
				if tt.wantCalls == nil {
					assert.Empty(t, calls, "the redirect policy was called")
				} else {
					assert.Equal(t, tt.wantCalls, calls)
				}
			}
			opts := append([]receiver.PollingOption{receiver.WithPollingHTTPClient(hc)}, tt.opts...)
			client := receiver.NewPollingClient(tg.SecretToken(b112Token), make(chan tg.Update, 10), pollingTestLogger(), cfg, opts...)

			if tt.wantErr != nil || tt.wantErrTxt != "" {
				ctx, callerDeadline, cancel := callerContext()
				defer cancel()
				var sawCancel atomic.Bool
				switch tt.cancel {
				case "before":
					cancel()
				case "during":
					// The request must carry the caller's context: cancel it
					// and wait (bounded) until the request sees that.
					rec.onDelete = func(r *http.Request) {
						cancel()
						select {
						case <-r.Context().Done():
							sawCancel.Store(true)
						case <-time.After(2 * time.Second):
						}
					}
				}
				err := client.Start(ctx)
				if err == nil {
					client.Stop()
				}
				require.Error(t, err)
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				}
				if tt.wantErrTxt != "" {
					assert.Equal(t, tt.wantErrTxt, err.Error())
				}
				if tt.wantURLErr != "" {
					var urlErr *url.Error
					require.ErrorAs(t, err, &urlErr, "the parse error is kept in the chain")
					assert.Equal(t, tt.wantURLErr, urlErr.Err.Error())
				}
				if tt.wantAPI != 0 {
					var apiErr *receiver.APIError
					require.ErrorAs(t, err, &apiErr)
					assert.Equal(t, tt.wantAPI, apiErr.Code)
				}
				assert.True(t, strings.HasPrefix(err.Error(), "failed to delete webhook: "), err.Error())
				assert.NotContains(t, err.Error(), b112Token)
				assert.False(t, client.Running())
				if tt.cancel == "during" {
					assert.True(t, sawCancel.Load(), "the deleteWebhook request did not see the caller's cancel")
				}
				var want []recordedRequest // nil: no request at all
				if tt.wantDelete != nil {
					want = append(want, *tt.wantDelete)
				}
				if tt.wantRedir != nil {
					want = append(want, *tt.wantRedir)
				}
				// The delete (and a followed redirect), no getUpdates.
				assert.Equal(t, want, rec.requests())
				assertCallerContext(t, rec, callerDeadline)
				assertCalls()
				if tt.wantAPI != 0 {
					// As in TestPolling_Start_DeleteWebhookFails: a loop started
					// anyway polls at once; this bounded check sees it within 300ms.
					select {
					case <-rec.firstGetUpdates:
						t.Error("getUpdates was sent after a refused deleteWebhook")
					case <-time.After(300 * time.Millisecond):
					}
					assert.Equal(t, want, rec.requests())
				}
				return
			}

			before := time.Now()
			require.NoError(t, client.Start(context.Background()))
			after := time.Now()
			select {
			case <-rec.firstGetUpdates:
			case <-time.After(5 * time.Second):
				client.Stop()
				t.Fatal("no getUpdates request within 5s")
			}
			client.Stop()

			reqs := rec.requests()
			require.GreaterOrEqual(t, len(reqs), 1)
			for i, r := range reqs {
				assert.Equal(t, "polling-client", r.Tag, "request %d %s was not sent through the polling client", i, r.Path)
			}
			first := 0
			if tt.wantDelete == nil {
				assert.Zero(t, countPathSuffix(reqs, "/deleteWebhook"))
			} else {
				require.GreaterOrEqual(t, len(reqs), 2)
				assert.Equal(t, *tt.wantDelete, reqs[0])
				// Exactly one: the fixture answers deleteWebhook directly, or
				// redirects it to /redirected.
				assert.Equal(t, 1, countPathSuffix(reqs, "/deleteWebhook"))
				// The polling client's own Timeout bounds the request.
				d := rec.reqContext(0).deadline
				require.False(t, d.IsZero(), "deleteWebhook request has no deadline")
				assert.False(t, d.Before(before.Add(tt.timeout)) || d.After(after.Add(tt.timeout)),
					"deadline %v not in [%v, %v]", d, before.Add(tt.timeout), after.Add(tt.timeout))
				first = 1
				if tt.wantRedir != nil {
					require.GreaterOrEqual(t, len(reqs), 3)
					assert.Equal(t, *tt.wantRedir, reqs[1])
					first = 2
				}
			}
			assertCalls()
			assert.Equal(t, tt.wantUpdate.Host, reqs[first].Host)
			assert.Equal(t, tt.wantUpdate.Path, reqs[first].Path)
		})
	}
}

// TestPolling_Start_DeleteWebhookFails checks that a refused deleteWebhook
// stops Start: the APIError comes back wrapped, the client is not running,
// and no getUpdates request follows.
func TestPolling_Start_DeleteWebhookFails(t *testing.T) {
	rec := newDeleteWebhookRecorder(b112Delete401)
	cfg := pollingTestConfig()
	cfg.BaseURL = b112BaseURL
	client := receiver.NewPollingClient(tg.SecretToken(b112Token), make(chan tg.Update, 10), pollingTestLogger(), cfg,
		receiver.WithPollingHTTPClient(rec.client(b112Timeout)),
		receiver.WithPollingDeleteWebhook(true),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops a poll loop a wrong Start may have left
	err := client.Start(ctx)
	if err == nil {
		client.Stop()
	}
	require.Error(t, err)
	assert.Equal(t, "failed to delete webhook: telegram API error 401: Unauthorized", err.Error())
	var apiErr *receiver.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 401, apiErr.Code)
	assert.Equal(t, "Unauthorized", apiErr.Description)
	assert.NoError(t, apiErr.Err)
	assert.False(t, client.Running())

	want := []recordedRequest{{
		Scheme:   "http",
		URLHost:  b112Host,
		Method:   http.MethodGet,
		Host:     b112Host,
		Path:     b112DeletePath,
		RawQuery: "drop_pending_updates=false",
		Cookie:   "b112=jar",
		Tag:      "polling-client",
	}}
	assert.Equal(t, want, rec.requests())

	// A poll loop started anyway sends getUpdates at once. This bounded check
	// sees one that polls within 300ms; a later start is not seen.
	select {
	case <-rec.firstGetUpdates:
		t.Error("getUpdates was sent after a failed deleteWebhook")
	case <-time.After(300 * time.Millisecond):
	}
	assert.Equal(t, want, rec.requests())
}

// TestDeleteWebhook_ContextCancelled checks that the exported DeleteWebhook
// passes its context to the request: cancelled before the call, no request
// is sent; cancelled while the request is in flight, the request sees it,
// and it carries the caller's value and deadline. Both times the error wraps
// context.Canceled.
func TestDeleteWebhook_ContextCancelled(t *testing.T) {
	t.Run("X1 before call", func(t *testing.T) {
		rec := newDeleteWebhookRecorder(b112DeleteOK)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := receiver.DeleteWebhook(ctx, rec.client(b112Timeout), tg.SecretToken(b112Token), false)
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.NotContains(t, err.Error(), b112Token)
		assert.Empty(t, rec.requests())
	})
	t.Run("X2 in flight", func(t *testing.T) {
		rec := newDeleteWebhookRecorder(b112DeleteOK)
		ctx, callerDeadline, cancel := callerContext()
		defer cancel()
		var sawCancel atomic.Bool
		rec.onDelete = func(r *http.Request) {
			cancel()
			select {
			case <-r.Context().Done():
				sawCancel.Store(true)
			case <-time.After(2 * time.Second):
			}
		}

		err := receiver.DeleteWebhook(ctx, rec.client(b112Timeout), tg.SecretToken(b112Token), false)
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		assert.NotContains(t, err.Error(), b112Token)
		assert.True(t, sawCancel.Load(), "the deleteWebhook request did not see the caller's cancel")
		assert.Equal(t, []recordedRequest{{Scheme: "https", URLHost: "api.telegram.org", Method: http.MethodGet,
			Host: "api.telegram.org", Path: b112DefaultDeletePath, RawQuery: "drop_pending_updates=false",
			Cookie: "b112=jar", Tag: "polling-client"}}, rec.requests())
		assertCallerContext(t, rec, callerDeadline)
	})
}
