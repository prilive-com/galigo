package receiver_test

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateDeliveryPolicy_Constants(t *testing.T) {
	// Verify the policy constants are distinct
	assert.NotEqual(t, receiver.DeliveryPolicyBlock, receiver.DeliveryPolicyDropNewest)
	assert.NotEqual(t, receiver.DeliveryPolicyBlock, receiver.DeliveryPolicyDropOldest)
	assert.NotEqual(t, receiver.DeliveryPolicyDropNewest, receiver.DeliveryPolicyDropOldest)
}

func TestDefaultConfig_DeliveryPolicy(t *testing.T) {
	cfg := receiver.DefaultConfig()

	assert.Equal(t, receiver.DeliveryPolicyBlock, cfg.UpdateDeliveryPolicy)
	assert.Equal(t, 5*time.Second, cfg.UpdateDeliveryTimeout)
	assert.Nil(t, cfg.OnUpdateDropped)
}

func TestDeliveryPolicyOptions(t *testing.T) {
	logger := slog.Default()
	updates := make(chan tg.Update, 10)
	cfg := receiver.DefaultConfig()

	var droppedUpdates []int
	var droppedReasons []string
	var mu sync.Mutex

	callback := func(id int, reason string) {
		mu.Lock()
		droppedUpdates = append(droppedUpdates, id)
		droppedReasons = append(droppedReasons, reason)
		mu.Unlock()
	}

	client := receiver.NewPollingClient(
		"test:token",
		updates,
		logger,
		cfg,
		receiver.WithDeliveryPolicy(receiver.DeliveryPolicyDropNewest),
		receiver.WithDeliveryTimeout(10*time.Second),
		receiver.WithUpdateDroppedCallback(callback),
	)

	require.NotNil(t, client)
}

func TestConfig_LoadDeliveryPolicy(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedError bool
		expected      receiver.UpdateDeliveryPolicy
	}{
		{"default block", "", false, receiver.DeliveryPolicyBlock},
		{"explicit block", "block", false, receiver.DeliveryPolicyBlock},
		{"drop_newest", "drop_newest", false, receiver.DeliveryPolicyDropNewest},
		{"dropnewest", "dropnewest", false, receiver.DeliveryPolicyDropNewest},
		{"drop_oldest", "drop_oldest", false, receiver.DeliveryPolicyDropOldest},
		{"dropoldest", "dropoldest", false, receiver.DeliveryPolicyDropOldest},
		{"invalid", "invalid_policy", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("UPDATE_DELIVERY_POLICY", tt.envValue)
			}
			t.Setenv("TELEGRAM_BOT_TOKEN", "test:token")

			cfg, err := receiver.LoadConfig()
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, cfg.UpdateDeliveryPolicy)
			}
		})
	}
}

func TestConfig_LoadDeliveryTimeout(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "test:token")
	t.Setenv("UPDATE_DELIVERY_TIMEOUT", "10s")

	cfg, err := receiver.LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, cfg.UpdateDeliveryTimeout)
}
