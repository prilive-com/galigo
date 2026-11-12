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

// S70_PaidMedia sends paid media (Bot API 7.6) priced at one Star: a photo, a video and a live photo (10.0), each
// uploaded, and checks the returned message's paid_media. Nobody buys it; the cleanup deletes it.
func S70_PaidMedia() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S70-PaidMedia",
		ScenarioDescription: "sendPaidMedia with a photo, a video and a live photo (7.6, 10.0)",
		CoveredMethods:      []string{"sendPaidMedia"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendPaidMediaStep{},
			&engine.CleanupStep{},
		},
	}
}

// AllStarsScenarios returns all Stars/payment scenarios.
func AllStarsScenarios() []engine.Scenario {
	return []engine.Scenario{
		S21_Stars(),
		S22_Invoice(),
		S70_PaidMedia(),
	}
}
