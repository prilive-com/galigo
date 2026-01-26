package tg_test

import (
	"testing"

	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
)

// ==================== ParseMode ====================

func TestParseMode_String(t *testing.T) {
	tests := []struct {
		mode     tg.ParseMode
		expected string
	}{
		{tg.ParseModeHTML, "HTML"},
		{tg.ParseModeMarkdown, "Markdown"},
		{tg.ParseModeMarkdownV2, "MarkdownV2"},
		{tg.ParseMode(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.mode.String())
		})
	}
}

func TestParseMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  tg.ParseMode
		valid bool
	}{
		{tg.ParseModeHTML, true},
		{tg.ParseModeMarkdown, true},
		{tg.ParseModeMarkdownV2, true},
		{tg.ParseMode(""), true}, // empty is valid (no formatting)
		{tg.ParseMode("invalid"), false},
		{tg.ParseMode("html"), false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.mode.IsValid())
		})
	}
}

// ==================== ChatType ====================

func TestChatType_String(t *testing.T) {
	tests := []struct {
		chatType tg.ChatType
		expected string
	}{
		{tg.ChatTypePrivate, "private"},
		{tg.ChatTypeGroup, "group"},
		{tg.ChatTypeSupergroup, "supergroup"},
		{tg.ChatTypeChannel, "channel"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.chatType.String())
		})
	}
}

func TestChatType_IsGroup(t *testing.T) {
	tests := []struct {
		chatType tg.ChatType
		isGroup  bool
	}{
		{tg.ChatTypePrivate, false},
		{tg.ChatTypeGroup, true},
		{tg.ChatTypeSupergroup, true},
		{tg.ChatTypeChannel, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.chatType), func(t *testing.T) {
			assert.Equal(t, tt.isGroup, tt.chatType.IsGroup())
		})
	}
}
