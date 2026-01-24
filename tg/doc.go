// Package tg provides core Telegram types shared by receiver and sender.
//
// This package contains:
//   - All Telegram API types (Message, User, Chat, Update, etc.)
//   - Error types and sentinel errors
//   - SecretToken for safe token handling
//   - Base configuration
//   - Keyboard builders
//
// # Usage
//
//	import "github.com/prilive-com/galigo/tg"
//
//	var msg tg.Message
//	var err tg.APIError
//	token := tg.SecretToken("123:ABC...")
package tg
