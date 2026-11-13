package sender_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-82 and B-88 (Bot API 9.2, channel direct messages, send side): direct_messages_topic_id on forwardMessages,
// copyMessages and sendMediaGroup; can_manage_direct_messages on promoteChatMember.

// b82Topic is a topic id above 2^31 (topic ids may need up to 52 bits); b82TopicMax is the largest 52-bit id, all
// ones, so a value that passes through a float32 or loses a low bit fails (a power of two does not show that).
const (
	b82Topic    = int64(1) << 40
	b82TopicMax = int64(1)<<52 - 1
)

// assertRawJSON compares the raw JSON text of every top-level value of a JSON body with want, so a number or a bool
// sent as a JSON string fails (wireCall's argument map decodes a JSON string to its text).
func assertRawJSON(t *testing.T, want map[string]string, raw map[string]json.RawMessage) {
	t.Helper()
	got := map[string]string{}
	for k, v := range raw {
		got[k] = string(v)
	}
	assert.Equal(t, want, got, "raw JSON values")
}

func replyIDs(w http.ResponseWriter, _ *http.Request) {
	testutil.ReplyOK(w, []tg.MessageID{{MessageID: 7}})
}

// withTopic sets DirectMessagesTopicID by reflection, so a version with a wrong Go type still builds and the shape row
// (T1) names it; an int64 value goes to an integer field as it is and to a float field as float64.
func withTopic[T any](req T, topic int64) T {
	switch f := reflect.ValueOf(&req).Elem().FieldByName("DirectMessagesTopicID"); f.Kind() {
	case reflect.Int, reflect.Int32, reflect.Int64:
		f.SetInt(topic)
	case reflect.Float32, reflect.Float64:
		f.SetFloat(float64(topic))
	}
	return req
}

// T1: the Go shape of the four new fields (E-0126, E-0132).
func TestB82_Shapes(t *testing.T) {
	for _, r := range []struct {
		typ            any
		name, gt, tags string
	}{
		{sender.ForwardMessagesRequest{}, "DirectMessagesTopicID", "int64", "direct_messages_topic_id,omitempty"},
		{sender.CopyMessagesRequest{}, "DirectMessagesTopicID", "int64", "direct_messages_topic_id,omitempty"},
		{sender.SendMediaGroupRequest{}, "DirectMessagesTopicID", "int64", "direct_messages_topic_id,omitempty"},
		{sender.PromoteChatMemberRequest{}, "CanManageDirectMessages", "*bool", "can_manage_direct_messages,omitempty"},
	} {
		rt := reflect.TypeOf(r.typ)
		t.Run(rt.Name(), func(t *testing.T) {
			f, ok := rt.FieldByName(r.name)
			require.True(t, ok, "%s has no %s", rt.Name(), r.name)
			assert.False(t, f.Anonymous)
			assert.Len(t, f.Index, 1, "a direct field, not promoted")
			assert.Equal(t, r.gt, f.Type.String())
			assert.Equal(t, r.tags, f.Tag.Get("json"))
		})
	}
}

// T2: forwardMessages and copyMessages send the topic id in the JSON body as a JSON number with exact digits, and
// no key at 0.
func TestB82_ForwardCopyWire(t *testing.T) {
	for _, method := range []string{"forwardMessages", "copyMessages"} {
		for _, topic := range []int64{0, 1, b82Topic, b82TopicMax} {
			t.Run(fmt.Sprintf("%s/%d", method, topic), func(t *testing.T) {
				mediaType, args, _, raw, err := b48c1WireCall(t, method, replyIDs, func(ctx context.Context, c *sender.Client) error {
					var err error
					if method == "forwardMessages" {
						_, err = c.ForwardMessages(ctx, withTopic(sender.ForwardMessagesRequest{ChatID: testutil.TestChatID, FromChatID: testutil.TestChatID,
							MessageIDs: []int{5}}, topic))
					} else {
						_, err = c.CopyMessages(ctx, withTopic(sender.CopyMessagesRequest{ChatID: testutil.TestChatID, FromChatID: testutil.TestChatID,
							MessageIDs: []int{5}}, topic))
					}
					return err
				})
				require.NoError(t, err)
				assert.Equal(t, "application/json", mediaType)
				want := map[string]string{"chat_id": wireChatID, "from_chat_id": wireChatID, "message_ids": "[5]"}
				if topic != 0 {
					want["direct_messages_topic_id"] = fmt.Sprint(topic)
				}
				assertArgs(t, want, args)
				assertRawJSON(t, want, raw)
			})
		}
	}
}

// T3: sendMediaGroup sends the topic id as a form field with exact digits, on the upload path and the file-id path,
// and no field at 0.
func TestB82_MediaGroupWire(t *testing.T) {
	for _, path := range []string{"upload", "fileid"} {
		for _, topic := range []int64{0, 1, b82Topic, b82TopicMax} {
			t.Run(fmt.Sprintf("%s/%d", path, topic), func(t *testing.T) {
				media := []sender.InputFile{sender.FromFileID("A").WithMediaType("photo"), sender.FromFileID("B").WithMediaType("photo")}
				wantMedia := `[{"type":"photo","media":"A"},{"type":"photo","media":"B"}]`
				wantFiles := map[string]wireFile{}
				if path == "upload" {
					media = []sender.InputFile{sender.FromBytes([]byte("pa"), "a.png").WithMediaType("photo"),
						sender.FromBytes([]byte("pb"), "b.png").WithMediaType("photo")}
					wantMedia = `[{"type":"photo","media":"attach://file0"},{"type":"photo","media":"attach://file1"}]`
					wantFiles = map[string]wireFile{"file0": {FileName: "a.png", Data: []byte("pa")}, "file1": {FileName: "b.png", Data: []byte("pb")}}
				}
				mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
					_, err := c.SendMediaGroup(ctx, withTopic(sender.SendMediaGroupRequest{ChatID: testutil.TestChatID, Media: media}, topic))
					return err
				})
				require.NoError(t, err)
				assert.Equal(t, "multipart/form-data", mediaType)
				want := map[string]string{"chat_id": wireChatID, "media": wantMedia}
				if topic != 0 {
					want["direct_messages_topic_id"] = fmt.Sprint(topic)
				}
				assertArgs(t, want, args)
				assertFiles(t, wantFiles, files)
			})
		}
	}
}

// T4: promoteChatMember: the option (true, false, unset), PromoteChatMemberWithRights (true, false, nil, and
// FullAdminRights) and DemoteChatMember (false) at every layer that copies the right (E-0149). The right is checked as
// raw JSON too (a quoted "true" fails). The demote row compares the whole body: the chat, the user and every right
// DemoteChatMember clears, so a wrong user or a neighbour line replaced by the new one fails (E-0072).
func TestB82_PromoteWire(t *testing.T) {
	yes, no := true, false
	demoteBody := map[string]string{"chat_id": wireChatID, "user_id": "456"}
	for _, k := range []string{"can_manage_chat", "can_delete_messages", "can_manage_video_chats", "can_restrict_members",
		"can_promote_members", "can_change_info", "can_invite_users", "can_post_messages", "can_edit_messages",
		"can_pin_messages", "can_manage_tags", "can_manage_direct_messages"} {
		demoteBody[k] = "false"
	}
	// the 13 other options with their keys (the combined-option leaves below)
	others := []struct {
		opt func(bool) sender.PromoteOption
		key string
	}{
		{sender.WithAnonymous, "is_anonymous"}, {sender.WithCanManageChat, "can_manage_chat"},
		{sender.WithCanDeleteMessages, "can_delete_messages"}, {sender.WithCanManageVideoChats, "can_manage_video_chats"},
		{sender.WithCanRestrictMembers, "can_restrict_members"}, {sender.WithCanPromoteMembers, "can_promote_members"},
		{sender.WithCanChangeInfo, "can_change_info"}, {sender.WithCanInviteUsers, "can_invite_users"},
		{sender.WithCanPostMessages, "can_post_messages"}, {sender.WithCanEditMessages, "can_edit_messages"},
		{sender.WithCanPinMessages, "can_pin_messages"}, {sender.WithCanManageTopics, "can_manage_topics"},
		{sender.WithCanManageTags, "can_manage_tags"},
	}
	// rightsNilOthers sets every right of tg.ChatAdministratorRights except the new one by reflection (the i-th other
	// right gets val(i); a pointer right points to that value), leaves the new one nil, and returns the rights with the
	// whole body WithRights must send: a WithRights copy that falls back to any other right, or to a mix of them, when
	// the new one is nil sends the key (E-0132, E-0143)
	rightsNilOthers := func(val func(i int) bool) (tg.ChatAdministratorRights, map[string]string, []string) {
		var rights tg.ChatAdministratorRights
		body := map[string]string{"chat_id": wireChatID, "user_id": "456"}
		var keys []string
		rv := reflect.ValueOf(&rights).Elem()
		for i := range rv.NumField() {
			sf := rv.Type().Field(i)
			if sf.Name == "CanManageDirectMessages" {
				continue
			}
			v := val(len(keys))
			switch f := rv.Field(i); f.Kind() {
			case reflect.Bool:
				f.SetBool(v)
			case reflect.Pointer:
				f.Set(reflect.ValueOf(&v))
			default:
				t.Fatalf("field %s has kind %s", sf.Name, f.Kind())
			}
			k, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
			body[k] = strconv.FormatBool(v)
			keys = append(keys, k)
		}
		return rights, body, keys
	}
	othersTrue, othersTrueBody, otherRightKeys := rightsNilOthers(func(int) bool { return true })
	for _, r := range []struct {
		name string
		call func(ctx context.Context, c *sender.Client) error
		want string            // "true", "false" or "" (absent)
		only bool              // the body holds only chat_id, user_id and this right (options only: WithRights always sends the 8 plain bools of tg.ChatAdministratorRights)
		body map[string]string // the whole body, when set
	}{
		{"option true", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMember(ctx, testutil.TestChatID, 456, sender.WithCanManageDirectMessages(true))
		}, "true", true, nil},
		{"option false", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMember(ctx, testutil.TestChatID, 456, sender.WithCanManageDirectMessages(false))
		}, "false", true, nil},
		{"option twice true false", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMember(ctx, testutil.TestChatID, 456, sender.WithCanManageDirectMessages(true), sender.WithCanManageDirectMessages(false))
		}, "false", true, nil},
		{"option twice false true", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMember(ctx, testutil.TestChatID, 456, sender.WithCanManageDirectMessages(false), sender.WithCanManageDirectMessages(true))
		}, "true", true, nil},
		{"option unset", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMember(ctx, testutil.TestChatID, 456, sender.WithCanManageTopics(true))
		}, "", false, nil},
		{"rights true", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.ChatAdministratorRights{CanManageDirectMessages: &yes})
		}, "true", false, nil},
		{"rights false", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.ChatAdministratorRights{CanManageDirectMessages: &no})
		}, "false", false, nil},
		{"rights nil", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.ChatAdministratorRights{CanManageTopics: &yes})
		}, "", false, nil},
		{"full admin rights", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, tg.FullAdminRights())
		}, "true", false, nil},
		{"demote", func(ctx context.Context, c *sender.Client) error {
			return c.DemoteChatMember(ctx, testutil.TestChatID, 456)
		}, "false", false, demoteBody},
		{"rights nil others true", func(ctx context.Context, c *sender.Client) error {
			return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, othersTrue)
		}, "", false, othersTrueBody},
	} {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, _, raw, err := b48c1WireCall(t, "promoteChatMember", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) }, r.call)
			require.NoError(t, err)
			assert.Equal(t, "application/json", mediaType)
			if r.body != nil {
				assertArgs(t, r.body, args)
				assertRawJSON(t, r.body, raw)
				return
			}
			if r.only {
				want := map[string]string{"chat_id": wireChatID, "user_id": "456", "can_manage_direct_messages": r.want}
				assertArgs(t, want, args)
				assertRawJSON(t, want, raw)
				return
			}
			got, ok := args["can_manage_direct_messages"]
			if r.want == "" {
				assert.False(t, ok, "can_manage_direct_messages sent: %q", got)
				return
			}
			assert.True(t, ok, "can_manage_direct_messages missing")
			assert.Equal(t, r.want, got)
			assert.Equal(t, r.want, string(raw["can_manage_direct_messages"]), "raw JSON value")
		})
	}

	// The combined-option leaves: PromoteChatMember with the 13 other options and WithCanManageDirectMessages, the new
	// option last or first, its value true or false. Each leaf runs the passes "others all true", "others all false"
	// and one pass per other option with only that one true, so no other value is tied to the new value, to the order
	// or to another option (E-0100, E-0062); every pass compares the whole body, as arguments and as raw JSON. So an
	// option that clears, overwrites or copies another field, also only for some values, fails.
	type otherPass struct {
		name string
		val  func(i int) bool
	}
	otherPasses := []otherPass{{"others all true", func(int) bool { return true }}, {"others all false", func(int) bool { return false }}}
	for j := range others {
		otherPasses = append(otherPasses, otherPass{"only " + others[j].key, func(i int) bool { return i == j }})
	}
	for _, cc := range []struct {
		name         string
		newFirst, dm bool
	}{
		{"options combined last true", false, true}, {"options combined last false", false, false},
		{"options combined first true", true, true}, {"options combined first false", true, false},
	} {
		t.Run(cc.name, func(t *testing.T) {
			for _, p := range otherPasses {
				want := map[string]string{"chat_id": wireChatID, "user_id": "456", "can_manage_direct_messages": strconv.FormatBool(cc.dm)}
				var opts []sender.PromoteOption
				if cc.newFirst {
					opts = append(opts, sender.WithCanManageDirectMessages(cc.dm))
				}
				for i, o := range others {
					opts = append(opts, o.opt(p.val(i)))
					want[o.key] = strconv.FormatBool(p.val(i))
				}
				if !cc.newFirst {
					opts = append(opts, sender.WithCanManageDirectMessages(cc.dm))
				}
				mediaType, args, _, raw, err := b48c1WireCall(t, "promoteChatMember", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) },
					func(ctx context.Context, c *sender.Client) error {
						return c.PromoteChatMember(ctx, testutil.TestChatID, 456, opts...)
					})
				require.NoError(t, err, p.name)
				assert.Equal(t, "application/json", mediaType, p.name)
				for _, d := range argsDiff(want, args) {
					t.Errorf("%s: args: %s", p.name, d)
				}
				gotRaw := map[string]string{}
				for k, v := range raw {
					gotRaw[k] = string(v)
				}
				assert.Equal(t, want, gotRaw, "%s: raw JSON values", p.name)
			}
		})
	}

	// The nil-right leaf (change round c1, A-C1-01): WithRights with the new right nil and the 16 other rights in the
	// passes "others all true", "others all false" and one pass per other right with only that one true. A WithRights
	// copy that, for a nil new right, derives a value from any other right being true or false, or from a mix of them,
	// sends the key; every pass compares the whole body, as arguments and as raw JSON.
	t.Run("rights nil others each", func(t *testing.T) {
		// in this leaf, not in the parent: a failure here must not stop the other leaves (mutant MC-new-right-in-tg)
		require.Len(t, otherRightKeys, 16, "the other rights of tg.ChatAdministratorRights")
		passes := []otherPass{{"others all true", func(int) bool { return true }}, {"others all false", func(int) bool { return false }}}
		for j := range otherRightKeys {
			passes = append(passes, otherPass{"only " + otherRightKeys[j], func(i int) bool { return i == j }})
		}
		for _, p := range passes {
			rights, want, _ := rightsNilOthers(p.val)
			mediaType, args, _, raw, err := b48c1WireCall(t, "promoteChatMember", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) },
				func(ctx context.Context, c *sender.Client) error {
					return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, rights)
				})
			require.NoError(t, err, p.name)
			assert.Equal(t, "application/json", mediaType, p.name)
			for _, d := range argsDiff(want, args) {
				t.Errorf("%s: args: %s", p.name, d)
			}
			gotRaw := map[string]string{}
			for k, v := range raw {
				gotRaw[k] = string(v)
			}
			assert.Equal(t, want, gotRaw, "%s: raw JSON values", p.name)
		}
	})
}

// T5: the whole class of B-88 (a right that PromoteChatMemberWithRights drops; 9.5 dropped can_manage_tags the same
// way): every field of tg.ChatAdministratorRights, set by reflection, reaches the body with its value, in a pass with
// all true, a pass with all false, and one pass per right with only that right true (so a copy from any other right
// fails); and the rights of PromoteChatMemberRequest are exactly the keys of tg.ChatAdministratorRights.
func TestB82_PromoteRightsClass(t *testing.T) {
	jsonKey := func(f reflect.StructField) string { k, _, _ := strings.Cut(f.Tag.Get("json"), ","); return k }
	rt := reflect.TypeOf(tg.ChatAdministratorRights{})
	var rightKeys []string
	for i := range rt.NumField() {
		rightKeys = append(rightKeys, jsonKey(rt.Field(i)))
	}
	var reqKeys []string
	pt := reflect.TypeOf(sender.PromoteChatMemberRequest{})
	for i := range pt.NumField() {
		if k := jsonKey(pt.Field(i)); k != "chat_id" && k != "user_id" {
			reqKeys = append(reqKeys, k)
		}
	}
	t.Run("key set", func(t *testing.T) {
		assert.ElementsMatch(t, rightKeys, reqKeys, "rights of PromoteChatMemberRequest vs tg.ChatAdministratorRights")
	})

	// "all true", "all false", then one pass per right with only that right true: every pair of
	// rights differs in at least one pass, so a copy from any other right fails (E-0062, E-0072).
	type rightsPass struct {
		name string
		val  func(i int) bool
	}
	passes := []rightsPass{{"all true", func(int) bool { return true }}, {"all false", func(int) bool { return false }}}
	for j := range rt.NumField() {
		passes = append(passes, rightsPass{"only " + rightKeys[j], func(i int) bool { return i == j }})
	}
	for _, pass := range passes {
		t.Run(pass.name, func(t *testing.T) {
			var rights tg.ChatAdministratorRights
			rv := reflect.ValueOf(&rights).Elem()
			want := map[string]string{"chat_id": wireChatID, "user_id": "456"}
			for i := range rt.NumField() {
				v := pass.val(i)
				switch f := rv.Field(i); f.Kind() {
				case reflect.Bool:
					f.SetBool(v)
				case reflect.Pointer:
					f.Set(reflect.ValueOf(&v))
				default:
					t.Fatalf("field %s has kind %s", rt.Field(i).Name, f.Kind())
				}
				want[rightKeys[i]] = fmt.Sprint(v)
			}
			_, args, _, raw, err := b48c1WireCall(t, "promoteChatMember", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) },
				func(ctx context.Context, c *sender.Client) error {
					return c.PromoteChatMemberWithRights(ctx, testutil.TestChatID, 456, rights)
				})
			require.NoError(t, err)
			assertArgs(t, want, args)
			assertRawJSON(t, want, raw)
		})
	}
}
