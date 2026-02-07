// Package galigo provides a unified Go library for Telegram Bot API.
//
// galigo combines receiving updates (webhook/long polling) and sending messages
// into a single, modern library with built-in resilience patterns.
//
// # Quick Start
//
//	bot, err := galigo.New(token,
//	    galigo.WithPolling(30, 100),
//	    galigo.WithRetries(5),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer bot.Close()
//
//	for update := range bot.Updates() {
//	    bot.SendMessage(ctx, update.Message.Chat.ID, "Echo: "+update.Message.Text)
//	}
//
// # Separate Receiver/Sender
//
// For microservices that only need one capability:
//
//	// Only receive updates
//	import "github.com/prilive-com/galigo/receiver"
//	recv, _ := receiver.New(token)
//
//	// Only send messages
//	import "github.com/prilive-com/galigo/sender"
//	send, _ := sender.New(token)
//
// # Shared Types
//
// All Telegram types are in the tg subpackage:
//
//	import "github.com/prilive-com/galigo/tg"
//	var msg tg.Message
//	var user tg.User
//
// # Features
//
//   - Dual mode: webhook or long polling
//   - Circuit breaker with sony/gobreaker
//   - Per-chat and global rate limiting
//   - Retry with exponential backoff and crypto jitter
//   - TLS 1.2+ enforcement
//   - Token auto-redaction in logs and errors
//   - Structured logging with slog
//   - OpenTelemetry-ready
//   - Go 1.22+ features: integer range loops, improved generics
package galigo
