package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-65 65i: S79 unique gifts (TEST-PLAN section 4). Every value is written out here; the guard call
// (getBusinessConnection) is not a covered method (A-P3-01).
func TestB65I_S79(t *testing.T) {
	assert.Equal(t, &engine.BaseScenario{
		ScenarioName:        "S79-UniqueGift",
		ScenarioDescription: "Get a unique gift by name; deleteBusinessAccountGift refused on a fake business connection (Bot API 10.4)",
		CoveredMethods:      []string{"getUniqueGift", "deleteBusinessAccountGift"},
		ScenarioSteps: []engine.Step{
			&engine.GetUniqueGiftStep{GiftName: "PlushPepe-1"},
			&engine.GetUniqueGiftStep{GiftName: "zzNoSuchGift-1", WantError: "gift not found"},
			&engine.DeleteBusinessAccountGiftStep{},
		},
		ScenarioTimeout: 30 * time.Second,
	}, S79_UniqueGift())
}
