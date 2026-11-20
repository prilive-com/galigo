package sender_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 reply markup and general changes, send side (B-65 65d, TEST-PLAN §3). Every body is compared whole,
// as arguments and as raw JSON, so a bool sent as a string fails.

const b65dGroup = int64(-1009876543210)

func b65dReplyTrue(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) }

// b65dBody calls the method on the mock and requires the whole JSON body to be want.
func b65dBody(t *testing.T, method string, want map[string]string, call func(ctx context.Context, c *sender.Client) error) {
	t.Helper()
	mediaType, args, _, raw, err := b48c1WireCall(t, method, b65dReplyTrue, call)
	require.NoError(t, err)
	assert.Equal(t, "application/json", mediaType)
	wantArgs := map[string]string{}
	for k, v := range want {
		if s, err := strconv.Unquote(v); err == nil {
			wantArgs[k] = s // a JSON string: its text is the argument
		} else {
			wantArgs[k] = v
		}
	}
	assertArgs(t, wantArgs, args)
	assertRawJSON(t, want, raw)
}

// T-wire-option: the new option alone, and given twice in each order (X-T1-06).
func TestB65D_WireOption(t *testing.T) {
	w := sender.WithCanSendWelcomeMessages
	for _, tc := range []struct {
		name string
		opts []sender.PromoteOption
		want string // "" = absent
	}{
		{"true", []sender.PromoteOption{w(true)}, "true"},
		{"false", []sender.PromoteOption{w(false)}, "false"},
		{"unset", []sender.PromoteOption{sender.WithCanManageTopics(true)}, ""},
		{"twice true false", []sender.PromoteOption{w(true), w(false)}, "false"},
		{"twice false true", []sender.PromoteOption{w(false), w(true)}, "true"},
		{"twice true true", []sender.PromoteOption{w(true), w(true)}, "true"},
		{"twice false false", []sender.PromoteOption{w(false), w(false)}, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]string{"chat_id": wireChatID, "user_id": "456"}
			if tc.want == "" {
				want["can_manage_topics"] = "true"
			} else {
				want["can_send_welcome_messages"] = tc.want
			}
			b65dBody(t, "promoteChatMember", want, func(ctx context.Context, c *sender.Client) error {
				return c.PromoteChatMember(ctx, testutil.TestChatID, 456, tc.opts...)
			})
		})
	}
}

// b65dRightsBody is the whole body PromoteChatMemberWithRights sends for rights with only the 8 plain rights false
// and the new right set to welcome.
func b65dRightsBody(chatID string, userID string, welcome bool) map[string]string {
	body := map[string]string{"chat_id": chatID, "user_id": userID, "can_send_welcome_messages": strconv.FormatBool(welcome)}
	for _, k := range []string{"is_anonymous", "can_manage_chat", "can_delete_messages", "can_manage_video_chats",
		"can_restrict_members", "can_promote_members", "can_change_info", "can_invite_users"} {
		body[k] = "false"
	}
	return body
}

// T-wire-rights: the literal key, from the rights value and from FullAdminRights.
func TestB65D_WireRights(t *testing.T) {
	for _, welcome := range []bool{true, false} {
		t.Run(strconv.FormatBool(welcome), func(t *testing.T) {
			b65dBody(t, "promoteChatMember", b65dRightsBody(wireChatID, "456", welcome), func(ctx context.Context, c *sender.Client) error {
				return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.ChatAdministratorRights{CanSendWelcomeMessages: welcome})
			})
		})
	}
	t.Run("full admin rights", func(t *testing.T) { // the whole body (C1-06): every right of FullAdminRights
		want := map[string]string{"chat_id": wireChatID, "user_id": "456", "is_anonymous": "false"}
		for _, k := range []string{"can_manage_chat", "can_delete_messages", "can_manage_video_chats", "can_restrict_members",
			"can_promote_members", "can_change_info", "can_invite_users", "can_post_messages", "can_edit_messages",
			"can_pin_messages", "can_post_stories", "can_edit_stories", "can_delete_stories", "can_manage_topics",
			"can_manage_direct_messages", "can_manage_tags", "can_send_welcome_messages"} {
			want[k] = "true"
		}
		b65dBody(t, "promoteChatMember", want, func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.FullAdminRights())
		})
	})
}

// T-wire-b95 (B-95): a channel username and another user reach the body of Demote and WithRights, with the new right.
func TestB65D_WireB95(t *testing.T) {
	t.Run("demote", func(t *testing.T) {
		want := map[string]string{"chat_id": `"@b65d_chan"`, "user_id": "789"}
		// Demote sends every right false except is_anonymous (as demoteBody in TestB82_PromoteWire)
		for _, k := range []string{"can_manage_chat", "can_delete_messages", "can_manage_video_chats",
			"can_restrict_members", "can_promote_members", "can_change_info", "can_invite_users", "can_post_messages",
			"can_edit_messages", "can_pin_messages", "can_manage_tags", "can_manage_direct_messages", "can_send_welcome_messages"} {
			want[k] = "false"
		}
		b65dBody(t, "promoteChatMember", want, func(ctx context.Context, c *sender.Client) error {
			return c.DemoteChatMember(ctx, "@b65d_chan", 789)
		})
	})
	t.Run("with rights", func(t *testing.T) {
		b65dBody(t, "promoteChatMember", b65dRightsBody(`"@b65d_chan"`, "789", true), func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, "@b65d_chan", 789, tg.ChatAdministratorRights{CanSendWelcomeMessages: true})
		})
	})
}

// b65dDraftFlags are the four flag inputs of T-wire-draft.
var b65dDraftFlags = []struct {
	name             string
	canStop, keepOne bool
}{
	{"both", true, true}, {"keep only", false, true}, {"can stop only", true, false}, {"neither", false, false},
}

// T-wire-draft (1)–(4): both draft methods send exactly the flags that are set.
func TestB65D_WireDraftJSON(t *testing.T) {
	for _, fl := range b65dDraftFlags {
		flags := func(want map[string]string) map[string]string {
			if fl.canStop {
				want["can_stop"] = "true"
			}
			if fl.keepOne {
				want["keep_on_stop"] = "true"
			}
			return want
		}
		t.Run("sendMessageDraft/"+fl.name, func(t *testing.T) {
			b65dBody(t, "sendMessageDraft", flags(map[string]string{"chat_id": "456", "draft_id": "5", "text": `"d"`}),
				func(ctx context.Context, c *sender.Client) error {
					return c.SendMessageDraft(ctx, sender.SendMessageDraftRequest{ChatID: int64(456), DraftID: 5, Text: "d",
						CanStop: fl.canStop, KeepOnStop: fl.keepOne})
				})
		})
		t.Run("sendRichMessageDraft/"+fl.name, func(t *testing.T) {
			b65dBody(t, "sendRichMessageDraft", flags(map[string]string{"chat_id": "456", "draft_id": "5", "rich_message": `{"markdown":"d"}`}),
				func(ctx context.Context, c *sender.Client) error {
					return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: int64(456), DraftID: 5,
						RichMessage: tg.InputRichMessage{Markdown: "d"}, CanStop: fl.canStop, KeepOnStop: fl.keepOne})
				})
		})
	}
}

// T-wire-draft (5)–(8): the flags on the form path of a rich draft with an upload (X-T2-05).
func TestB65D_WireDraftForm(t *testing.T) {
	for _, fl := range b65dDraftFlags {
		t.Run(fl.name, func(t *testing.T) {
			mediaType, args, files, _, err := b48c1WireCall(t, "sendRichMessageDraft", b65dReplyTrue, func(ctx context.Context, c *sender.Client) error {
				return c.SendRichMessageDraft(ctx, sender.SendRichMessageDraftRequest{ChatID: int64(456), DraftID: 5,
					RichMessage: tg.InputRichMessage{Markdown: "![](tg://photo?id=p)", Media: []tg.InputRichMessageMedia{
						{ID: "p", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("ph"), "p.jpg"))}}},
					CanStop: fl.canStop, KeepOnStop: fl.keepOne})
			})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			want := map[string]string{"chat_id": "456", "draft_id": "5",
				"rich_message": `{"markdown":"![](tg://photo?id=p)","media":[{"id":"p","media":{"media":"attach://file0","type":"photo"}}]}`}
			if fl.canStop {
				want["can_stop"] = "true"
			}
			if fl.keepOne {
				want["keep_on_stop"] = "true"
			}
			assertArgs(t, want, args)
			assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("ph")}}, files)
		})
	}
}

// T-wire-draft (9) (X-T3-04): a group chat with can_stop is sent; the server's refusal (probe call 05) is returned.
func TestB65D_WireDraftServerRefuses(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessageDraft", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyBadRequest(w, "TEXTDRAFT_PEER_INVALID") // the description "Bad Request: TEXTDRAFT_PEER_INVALID"
	})
	err := testutil.NewTestClient(t, server.BaseURL()).SendMessageDraft(context.Background(),
		sender.SendMessageDraftRequest{ChatID: b65dGroup, DraftID: 5, Text: "d", CanStop: true})
	require.Equal(t, 1, server.CaptureCount())
	_, args, _ := wireArgs(t, capturedRequest(server.LastCapture()))
	assertArgs(t, map[string]string{"chat_id": "-1009876543210", "draft_id": "5", "text": "d", "can_stop": "true"}, args)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "%v", err)
	assert.Equal(t, 400, apiErr.Code)
	assert.Equal(t, "Bad Request: TEXTDRAFT_PEER_INVALID", apiErr.Description)
}

// b65dInline is the markup of T-wire-markup: a callback button, a disabled button, a button with two kinds (the
// server decides, probe 05), and force_reply.
func b65dInline() *tg.InlineKeyboardMarkup {
	return &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{
		{{Text: "cb", CallbackData: "x"}, {Text: "off", Disabled: &tg.DisabledButton{}}},
		{{Text: "both", CallbackData: "y", Disabled: &tg.DisabledButton{}}},
	}, ForceReply: true}
}

const b65dInlineJSON = `{"inline_keyboard":[[{"text":"cb","callback_data":"x"},{"text":"off","disabled":{}}],` +
	`[{"text":"both","callback_data":"y","disabled":{}}]],"force_reply":true}`

func b65dMsgReply(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 42) }

// T-wire-markup: the markup reaches the server as it is, on the JSON path, the form path and the edit path.
func TestB65D_WireMarkup(t *testing.T) {
	t.Run("sendMessage", func(t *testing.T) {
		mediaType, args, _, raw, err := b48c1WireCall(t, "sendMessage", b65dMsgReply, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: testutil.TestChatID, Text: "m", ReplyMarkup: b65dInline()})
			return err
		})
		require.NoError(t, err)
		assert.Equal(t, "application/json", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "text": "m", "reply_markup": b65dInlineJSON}, args)
		assert.Equal(t, b65dInlineJSON, string(raw["reply_markup"]))
	})
	t.Run("sendPhoto upload", func(t *testing.T) {
		mediaType, args, files, _, err := b48c1WireCall(t, "sendPhoto", b65dMsgReply, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: testutil.TestChatID,
				Photo: sender.FromBytes([]byte("ph"), "p.jpg"), ReplyMarkup: b65dInline()})
			return err
		})
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "reply_markup": b65dInlineJSON}, args)
		assertFiles(t, map[string]wireFile{"photo": {FileName: "p.jpg", Data: []byte("ph")}}, files)
	})
	for _, tc := range []struct {
		name   string
		markup *tg.InlineKeyboardMarkup
		want   string
	}{
		{"editMessageReplyMarkup", b65dInline(), b65dInlineJSON},
		// the probe 08 shape: a disabled button and no force_reply (X-T3-05)
		{"editMessageReplyMarkup no force_reply", &tg.InlineKeyboardMarkup{InlineKeyboard: [][]tg.InlineKeyboardButton{
			{{Text: "off", Disabled: &tg.DisabledButton{}}}}}, `{"inline_keyboard":[[{"text":"off","disabled":{}}]]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mediaType, args, _, raw, err := b48c1WireCall(t, "editMessageReplyMarkup", b65dMsgReply, func(ctx context.Context, c *sender.Client) error {
				_, err := c.EditMessageReplyMarkup(ctx, sender.EditMessageReplyMarkupRequest{ChatID: testutil.TestChatID, MessageID: 42,
					ReplyMarkup: tc.markup})
				return err
			})
			require.NoError(t, err)
			assert.Equal(t, "application/json", mediaType)
			assertArgs(t, map[string]string{"chat_id": wireChatID, "message_id": "42", "reply_markup": tc.want}, args)
			assert.Equal(t, tc.want, string(raw["reply_markup"]))
		})
	}
}

// T-wire-replykb: force_reply on a reply keyboard reaches the server.
func TestB65D_WireReplyKeyboard(t *testing.T) {
	_, args, _, raw, err := b48c1WireCall(t, "sendMessage", b65dMsgReply, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMessage(ctx, sender.SendMessageRequest{ChatID: testutil.TestChatID, Text: "m",
			ReplyMarkup: &tg.ReplyKeyboardMarkup{Keyboard: [][]tg.KeyboardButton{{{Text: "k"}}}, ForceReply: true}})
		return err
	})
	require.NoError(t, err)
	const want = `{"keyboard":[[{"text":"k"}]],"force_reply":true}`
	assertArgs(t, map[string]string{"chat_id": wireChatID, "text": "m", "reply_markup": want}, args)
	assert.Equal(t, want, string(raw["reply_markup"]))
}
