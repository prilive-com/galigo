// Package receiver provides Telegram update receiving via webhook or long polling.
//
// You make the updates channel and read updates from it. The receiver writes to it and never closes it.
//
// # Modes
//
// Webhook mode receives updates via HTTPS webhook (Telegram pushes to your server).
// [NewWebhookHandler] returns a [net/http.Handler]; serve it on your own HTTPS server:
//
//	updates := make(chan tg.Update, 100)
//	cfg := receiver.DefaultConfig()
//	cfg.WebhookSecret = tg.SecretToken(secret)
//	handler := receiver.NewWebhookHandler(logger, updates, cfg)
//	http.Handle("/webhook", handler)
//
// Long polling mode receives updates by polling Telegram API (your server pulls).
// [PollingClient.Start] starts polling in the background:
//
//	updates := make(chan tg.Update, 100)
//	cfg := receiver.DefaultConfig()
//	cfg.PollingTimeout = 30
//	cfg.PollingLimit = 100
//	poller := receiver.NewPollingClient(tg.SecretToken(token), updates, logger, cfg)
//	if err := poller.Start(ctx); err != nil {
//	    // handle the error
//	}
//	defer poller.Stop()
//
// # Features
//
//   - Dual-mode operation (webhook or long polling)
//   - Circuit breaker for resilience
//   - Rate limiting for webhook mode
//   - Automatic webhook management
//   - Kubernetes-ready health probes
//   - Graceful shutdown support
package receiver
