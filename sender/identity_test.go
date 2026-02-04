package sender_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== SetMyCommands ====================

func TestSetMyCommands(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyCommands", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyCommands(context.Background(), []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
		{Command: "help", Description: "Get help"},
	})
	require.NoError(t, err)
}

func TestSetMyCommands_WithScope(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyCommands", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyCommands(context.Background(), []tg.BotCommand{
		{Command: "start", Description: "Start the bot"},
	}, sender.WithCommandScope(tg.BotCommandScopeAllPrivateChats()))
	require.NoError(t, err)
}

func TestSetMyCommands_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	tests := []struct {
		name     string
		commands []tg.BotCommand
		wantErr  string
	}{
		{
			name:     "too many commands",
			commands: make([]tg.BotCommand, 101),
			wantErr:  "at most 100",
		},
		{
			name:     "empty command",
			commands: []tg.BotCommand{{Command: "", Description: "test"}},
			wantErr:  "command",
		},
		{
			name:     "command too long",
			commands: []tg.BotCommand{{Command: "this_command_is_way_too_long_for_telegram", Description: "test"}},
			wantErr:  "command",
		},
		{
			name:     "invalid command chars",
			commands: []tg.BotCommand{{Command: "UPPERCASE", Description: "test"}},
			wantErr:  "lowercase",
		},
		{
			name:     "empty description",
			commands: []tg.BotCommand{{Command: "start", Description: ""}},
			wantErr:  "description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetMyCommands(context.Background(), tt.commands)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// ==================== GetMyCommands ====================

func TestGetMyCommands(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyCommands", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, []map[string]string{
			{"command": "start", "description": "Start the bot"},
			{"command": "help", "description": "Get help"},
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	commands, err := client.GetMyCommands(context.Background())
	require.NoError(t, err)
	assert.Len(t, commands, 2)
	assert.Equal(t, "start", commands[0].Command)
}

// ==================== DeleteMyCommands ====================

func TestDeleteMyCommands(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/deleteMyCommands", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.DeleteMyCommands(context.Background())
	require.NoError(t, err)
}

// ==================== SetMyName ====================

func TestSetMyName(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyName", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyName(context.Background(), "TestBot")
	require.NoError(t, err)
}

func TestSetMyName_WithLanguage(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyName", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyName(context.Background(), "TecTBot", sender.WithLanguage("ru"))
	require.NoError(t, err)
}

func TestSetMyName_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	// Name too long (65 chars)
	longName := "This name is definitely way too long for Telegram's 64 character limit!"
	err := client.SetMyName(context.Background(), longName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "64 characters")
}

// ==================== GetMyName ====================

func TestGetMyName(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyName", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]string{"name": "TestBot"})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.GetMyName(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "TestBot", result.Name)
}

// ==================== SetMyDescription ====================

func TestSetMyDescription(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyDescription(context.Background(), "This is a test bot")
	require.NoError(t, err)
}

func TestSetMyDescription_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	// Description too long (513 chars)
	longDesc := make([]byte, 513)
	for i := range longDesc {
		longDesc[i] = 'a'
	}
	err := client.SetMyDescription(context.Background(), string(longDesc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "512 characters")
}

// ==================== GetMyDescription ====================

func TestGetMyDescription(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]string{"description": "This is a test bot"})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.GetMyDescription(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "This is a test bot", result.Description)
}

// ==================== SetMyShortDescription ====================

func TestSetMyShortDescription(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyShortDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyShortDescription(context.Background(), "A test bot")
	require.NoError(t, err)
}

func TestSetMyShortDescription_Validation(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())

	// Short description too long (121 chars)
	longDesc := make([]byte, 121)
	for i := range longDesc {
		longDesc[i] = 'a'
	}
	err := client.SetMyShortDescription(context.Background(), string(longDesc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "120 characters")
}

// ==================== GetMyShortDescription ====================

func TestGetMyShortDescription(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyShortDescription", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]string{"short_description": "A test bot"})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.GetMyShortDescription(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "A test bot", result.ShortDescription)
}

// ==================== SetMyDefaultAdministratorRights ====================

func TestSetMyDefaultAdministratorRights(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyDefaultAdministratorRights", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	err := client.SetMyDefaultAdministratorRights(context.Background(),
		sender.WithAdminRights(tg.ChatAdministratorRights{
			CanDeleteMessages: true,
			CanInviteUsers:    true,
		}))
	require.NoError(t, err)
}

func TestSetMyDefaultAdministratorRights_ForChannels(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/setMyDefaultAdministratorRights", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, true)
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	canPost := true
	err := client.SetMyDefaultAdministratorRights(context.Background(),
		sender.WithAdminRightsForChannels(tg.ChatAdministratorRights{
			CanPostMessages: &canPost,
		}))
	require.NoError(t, err)
}

// ==================== GetMyDefaultAdministratorRights ====================

func TestGetMyDefaultAdministratorRights(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getMyDefaultAdministratorRights", func(w http.ResponseWriter, r *http.Request) {
		testutil.ReplyOK(w, map[string]any{
			"can_delete_messages": true,
			"can_invite_users":    true,
		})
	})

	client := testutil.NewTestClient(t, server.BaseURL())
	result, err := client.GetMyDefaultAdministratorRights(context.Background(), false)
	require.NoError(t, err)
	assert.True(t, result.CanDeleteMessages)
	assert.True(t, result.CanInviteUsers)
}

// ==================== BotCommandScope Constructors ====================

func TestBotCommandScopeConstructors(t *testing.T) {
	tests := []struct {
		name     string
		scope    tg.BotCommandScope
		wantType string
	}{
		{"default", tg.BotCommandScopeDefault(), "default"},
		{"all_private_chats", tg.BotCommandScopeAllPrivateChats(), "all_private_chats"},
		{"all_group_chats", tg.BotCommandScopeAllGroupChats(), "all_group_chats"},
		{"all_chat_administrators", tg.BotCommandScopeAllChatAdministrators(), "all_chat_administrators"},
		{"chat", tg.BotCommandScopeChat(int64(123)), "chat"},
		{"chat_administrators", tg.BotCommandScopeChatAdministrators(int64(123)), "chat_administrators"},
		{"chat_member", tg.BotCommandScopeChatMember(int64(123), 456), "chat_member"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantType, tt.scope.Type)
		})
	}
}
