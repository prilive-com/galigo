// Package sender provides Telegram message sending with resilience features.
//
// # Features
//
//   - Circuit breaker for fault tolerance
//   - Per-chat and global rate limiting
//   - Retry with exponential backoff
//   - Edit, delete, forward, copy messages
//   - Callback query responses
//   - Inline keyboard support
//
// # Usage
//
//	client, err := sender.New(token,
//	    sender.WithRateLimit(30, 50),
//	    sender.WithRetries(3),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
//	result, err := client.SendMessage(ctx, sender.SendMessageRequest{
//	    ChatID: chatID,
//	    Text:   "Hello, World!",
//	})
package sender
