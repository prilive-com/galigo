package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S13_WebhookLifecycle tests webhook management methods.
// Safety: backs up current webhook URL, sets a test URL, verifies, deletes, restores.
// This scenario is excluded from --run all to avoid disrupting production webhooks.
func S13_WebhookLifecycle() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S13_WebhookLifecycle",
		ScenarioDescription: "Test setWebhook, getWebhookInfo, deleteWebhook (with backup/restore)",
		CoveredMethods:      []string{"setWebhook", "getWebhookInfo", "deleteWebhook"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			// 1. Backup current webhook URL
			&engine.GetWebhookInfoStep{StoreAs: "original_webhook_url"},
			// 2. Set a test webhook URL (resolvable but not a real endpoint)
			&engine.SetWebhookStep{URL: "https://example.com/galigo-testbot-webhook-test"},
			// 3. Verify webhook was set
			&engine.VerifyWebhookURLStep{ExpectedURL: "https://example.com/galigo-testbot-webhook-test"},
			// 4. Delete webhook
			&engine.DeleteWebhookStep{},
			// 5. Verify webhook was deleted
			&engine.VerifyWebhookURLStep{ExpectedURL: ""},
			// 6. Restore original webhook (or leave deleted if none was set)
			&engine.RestoreWebhookStep{StoredKey: "original_webhook_url"},
		},
	}
}

// S14_GetUpdates tests the getUpdates method with a non-blocking call.
// This must run after webhooks are cleared (getUpdates fails if a webhook is active).
// Excluded from --run all since it requires no active webhook.
func S14_GetUpdates() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S14_GetUpdates",
		ScenarioDescription: "Test getUpdates with non-blocking call (timeout=0)",
		CoveredMethods:      []string{"getUpdates"},
		ScenarioTimeout:     15 * time.Second,
		ScenarioSteps: []engine.Step{
			// Ensure no webhook is active (getUpdates requires this)
			&engine.DeleteWebhookStep{},
			// Call getUpdates with timeout=0
			&engine.GetUpdatesStep{},
		},
	}
}

// AllWebhookScenarios returns all webhook/polling scenarios.
// These are excluded from --run all to avoid disrupting production webhooks.
func AllWebhookScenarios() []engine.Scenario {
	return []engine.Scenario{
		S13_WebhookLifecycle(),
		S14_GetUpdates(),
	}
}
