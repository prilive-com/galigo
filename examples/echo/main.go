// Example: Simple echo bot using galigo
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/prilive-com/galigo"
	"github.com/prilive-com/galigo/tg"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable required")
	}

	// Create bot with long polling
	bot, err := galigo.New(token,
		galigo.WithPolling(30, 100),
		galigo.WithRetries(3),
		galigo.WithPollingMaxErrors(5),
	)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	defer bot.Close()

	// Setup graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start receiving updates
	if err := bot.Start(ctx); err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	log.Println("Bot started. Press Ctrl+C to stop.")

	// Process updates
	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down...")
			return
		case update := <-bot.Updates():
			handleUpdate(ctx, bot, update)
		}
	}
}

func handleUpdate(ctx context.Context, bot *galigo.Bot, update tg.Update) {
	// Handle text messages
	if update.Message != nil && update.Message.Text != "" {
		msg := update.Message

		// Handle /start command
		if msg.Text == "/start" {
			_, err := bot.SendMessage(ctx, msg.Chat.ID,
				"Hello! I'm an echo bot. Send me any message and I'll echo it back.",
				galigo.WithParseMode(tg.ParseModeHTML),
			)
			if err != nil {
				log.Printf("Failed to send welcome: %v", err)
			}
			return
		}

		// Echo the message
		_, err := bot.SendMessage(ctx, msg.Chat.ID,
			"Echo: "+msg.Text,
			galigo.WithReplyTo(msg.MessageID),
		)
		if err != nil {
			log.Printf("Failed to echo message: %v", err)
		}
	}

	// Handle callback queries
	if update.CallbackQuery != nil {
		cb := update.CallbackQuery
		if err := bot.Answer(ctx, cb); err != nil {
			log.Printf("Failed to answer callback: %v", err)
		}
	}
}
