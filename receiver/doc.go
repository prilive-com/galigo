// Package receiver provides Telegram update receiving via webhook or long polling.
//
// # Modes
//
// Webhook mode receives updates via HTTPS webhook (Telegram pushes to your server):
//
//	receiver, err := receiver.New(token,
//	    receiver.WithWebhook(8443, "secret"),
//	)
//
// Long polling mode receives updates by polling Telegram API (your server pulls):
//
//	receiver, err := receiver.New(token,
//	    receiver.WithLongPolling(30, 100),
//	)
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
