package sender

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

const testToken = "123456789:ABCdefGHIjklMNOpqrSTUvwxYZ"

func TestClientClose_Idempotent(t *testing.T) {
	// Create a minimal client
	client, err := New(testToken)
	require.NoError(t, err)

	// First close should succeed
	err = client.Close()
	assert.NoError(t, err)

	// Second close should be a no-op, not panic
	err = client.Close()
	assert.NoError(t, err)

	// Third close should also be a no-op
	err = client.Close()
	assert.NoError(t, err)
}

func TestClientClose_Concurrent(t *testing.T) {
	// Create a minimal client
	client, err := New(testToken)
	require.NoError(t, err)

	// 100 goroutines closing simultaneously — must not panic
	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()
	}

	// Wait for all goroutines — no panic means success
	wg.Wait()
}

func TestClientClose_WithoutCleanupTicker(t *testing.T) {
	// Create client with WithoutLimiterCleanup option (no cleanup goroutine)
	cfg := DefaultConfig()
	cfg.Token = tg.SecretToken("123456789:ABCdefGHIjklMNOpqrSTUvwxYZ")
	client, err := NewFromConfig(cfg)
	require.NoError(t, err)

	// cleanupTicker should be set by default
	assert.NotNil(t, client.cleanupTicker)

	// Close should work
	err = client.Close()
	assert.NoError(t, err)

	// Double close should be no-op
	err = client.Close()
	assert.NoError(t, err)
}
