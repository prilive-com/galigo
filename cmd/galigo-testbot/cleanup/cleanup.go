package cleanup

import (
	"context"
	"log/slog"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// Cleaner handles message cleanup.
type Cleaner struct {
	sender engine.SenderClient
	logger *slog.Logger
}

// NewCleaner creates a new cleaner.
func NewCleaner(sender engine.SenderClient, logger *slog.Logger) *Cleaner {
	return &Cleaner{
		sender: sender,
		logger: logger,
	}
}

// CleanupMessages deletes all tracked messages in runtime.
func (c *Cleaner) CleanupMessages(ctx context.Context, rt *engine.Runtime) (deleted int, errors int) {
	for _, cm := range rt.CreatedMessages {
		if err := c.sender.DeleteMessage(ctx, cm.ChatID, cm.MessageID); err != nil {
			c.logger.Warn("failed to delete message",
				"chat_id", cm.ChatID,
				"message_id", cm.MessageID,
				"error", err)
			errors++
		} else {
			deleted++
		}
	}

	// Clear the list
	rt.CreatedMessages = rt.CreatedMessages[:0]
	rt.LastMessage = nil

	// Clean up sticker sets (best-effort)
	for _, name := range rt.CreatedStickerSets {
		if err := c.sender.DeleteStickerSet(ctx, name); err != nil {
			c.logger.Warn("failed to delete sticker set", "name", name, "error", err)
		} else {
			c.logger.Debug("deleted sticker set", "name", name)
		}
	}
	rt.CreatedStickerSets = rt.CreatedStickerSets[:0]

	c.logger.Info("cleanup completed", "deleted", deleted, "errors", errors)

	return deleted, errors
}
