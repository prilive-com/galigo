package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-48b2 TEST-PLAN §2.2, T-sc, T-sc-stars.
func TestB48B2_Scenario(t *testing.T) {
	// first, so it runs even when a check below stops the parent (O-T2-04)
	t.Run("stars", func(t *testing.T) {
		var names []string
		for _, sc := range AllStarsScenarios() {
			names = append(names, sc.Name())
		}
		assert.Equal(t, []string{"S21-Stars", "S22-Invoice", "S70-PaidMedia"}, names)
	})
	s := S70_PaidMedia()
	assert.Equal(t, "S70-PaidMedia", s.Name())
	assert.Equal(t, []string{"sendPaidMedia"}, s.Covers())
	assert.Equal(t, 60*time.Second, s.Timeout())
	steps := s.Steps()
	require.Len(t, steps, 2)
	assert.IsType(t, &engine.SendPaidMediaStep{}, steps[0])
	assert.IsType(t, &engine.CleanupStep{}, steps[1])
}
