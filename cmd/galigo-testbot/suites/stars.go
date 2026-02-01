package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// S21_Stars tests Star balance and transaction retrieval (read-only, safe).
func S21_Stars() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S21-Stars",
		ScenarioDescription: "Get Star balance and recent transactions",
		CoveredMethods:      []string{"getMyStarBalance", "getStarTransactions"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.GetMyStarBalanceStep{},
			&engine.GetStarTransactionsStep{Limit: 5},
		},
	}
}

// S22_Invoice tests sending a Stars invoice (XTR currency, no payment provider needed).
func S22_Invoice() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S22-Invoice",
		ScenarioDescription: "Send a Stars invoice",
		CoveredMethods:      []string{"sendInvoice"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendInvoiceStep{
				Title:       "[galigo-test] Test Item",
				Description: "A test invoice from galigo-testbot",
				Payload:     "galigo_test_payload",
				Currency:    "XTR",
				Prices:      []tg.LabeledPrice{{Label: "Test Item", Amount: 1}},
			},
			&engine.CleanupStep{},
		},
	}
}

// AllStarsScenarios returns all Stars/payment scenarios.
func AllStarsScenarios() []engine.Scenario {
	return []engine.Scenario{
		S21_Stars(),
		S22_Invoice(),
	}
}
