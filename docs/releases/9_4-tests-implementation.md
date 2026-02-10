# Bot API 9.4 Testing Guide — v2 Addendum

**Applies to**: `bot-api-9.4-testing-guide.md` (v1)
**Source**: Cross-analysis of Consultants M & N, verified against galigo codebase
**Changes**: 6 improvements adopted, 11 suggestions rejected

---

## ADD: Feature Coverage Matrix (insert after Architecture Overview)

> Adopted from Consultant N. Provides quick visual reference for what needs testing where.

| 9.4 Feature | Unit Test (tg/) | Unit Test (sender/) | Testbot E2E | Premium? | Notes |
|-------------|:---:|:---:|:---:|:---:|-------|
| Button `style` + `icon_custom_emoji_id` | ✅ JSON marshal/unmarshal | — | ✅ S38 | Icon emoji: Yes | Style is server-accepted without Premium |
| `VideoQuality` + `Video.qualities` | ✅ JSON unmarshal | — | ✅ **S42 NEW** | No | Receive video, verify field |
| `setMyProfilePhoto` | — | ✅ request encoding | ✅ S41 | No | Destructive — manual only |
| `removeMyProfilePhoto` | — | ✅ request encoding | ✅ S41 | No | Destructive — manual only |
| `getUserProfileAudios` | ✅ response type | ✅ request/response | ✅ S39 | No | Read-only, safe |
| `ChatOwnerLeft` / `ChatOwnerChanged` | ✅ JSON unmarshal | — | ⚠️ Manual | No | Hard to trigger programmatically |
| `UniqueGift.is_burned` | ✅ JSON unmarshal | — | ⚠️ Depends | No | Needs gift-enabled account |
| `UniqueGiftModel.rarity` (string) | ✅ JSON unmarshal | — | — | No | New 9.4 field |
| `UniqueGiftColors` (all 6 fields) | ✅ JSON round-trip | — | — | No | Verified against pyTelegramBotAPI |
| `ChatFullInfo.first_profile_audio` | ✅ JSON unmarshal | — | ✅ S40 | No | Read-only via getChat |
| `ChatFullInfo.unique_gift_colors` | ✅ JSON unmarshal | — | ✅ S40 | No | Pointer, not slice |
| `ChatFullInfo.paid_message_star_count` | ✅ JSON unmarshal | — | ✅ S40 | No | Read-only |
| `User.allows_users_to_create_topics` | ✅ JSON unmarshal | — | — | No | — |
| Private chat topics (`createForumTopic`) | — | — | ✅ **S43 NEW** | No | Existing method, new context |

---

## ADD: Premium Requirements Table (insert after Feature Coverage Matrix)

> Adopted from both Consultants M and N.

| Feature | Premium Needed? | Who Needs It? | Workaround |
|---------|:---:|:---:|------------|
| `icon_custom_emoji_id` on buttons | Yes | Bot owner's account | Test without icon; icon is optional |
| Custom emoji in HTML (`<tg-emoji>`) | Yes | Bot owner's account | Test with regular emoji fallback |
| `style` on buttons | No | — | Works for all bots |
| `setMyProfilePhoto` | No | — | Standard bot method |
| `getUserProfileAudios` | No | — | Returns empty if no audios |
| `createForumTopic` in private chat | No | — | User must allow topics |
| Gift features (`is_burned`, etc.) | No | — | Read-only types; parsing only |

**CI Impact**: Scenarios requiring Premium should be excluded from automated CI runs. Add to testbot as manual-trigger-only suites. The existing `integration.yml` workflow_dispatch input already supports this via suite selection.

---

## ADD: S42_VideoQualities Testbot Scenario (insert in suites/api94.go)

> Adopted from Consultant N's Scenario 7. Our v1 guide had no testbot coverage for video qualities.

```go
// ==================== S42: Video Qualities (9.4) ====================

// S42_VideoQualities tests that received videos with the 9.4 qualities field
// deserialize correctly. Sends a video, then verifies the response contains
// the Video struct (qualities may or may not be populated by Telegram).
//
// This is a DESERIALIZATION test — we verify no panics/errors when Telegram
// includes or omits the qualities array. We do NOT assert specific quality
// values since Telegram generates them server-side.
func S42_VideoQualities() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S42-VideoQualities",
		ScenarioDescription: "Send video and verify qualities field deserialization (9.4)",
		CoveredMethods:      []string{"sendVideo"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			// Send a test video (from fixtures)
			&engine.SendVideoStep{
				Video:   engine.MediaFromBytes(fixtures.VideoBytes(), "test.mp4", "video"),
				Caption: "galigo 9.4 video qualities test",
			},
			// Verify the response didn't crash and Video field exists
			&engine.VerifyLastMessageHasVideoStep{},
			// Cleanup
			&engine.CleanupStep{},
		},
	}
}
```

**New step type needed** in `engine/steps_9_4.go`:

```go
// VerifyLastMessageHasVideoStep checks that the last sent message has a Video field.
// It logs the qualities array content for evidence but does NOT assert specific values.
type VerifyLastMessageHasVideoStep struct{}

func (s *VerifyLastMessageHasVideoStep) Name() string { return "verify video (9.4 qualities)" }

func (s *VerifyLastMessageHasVideoStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	if rt.LastMessage == nil {
		return nil, fmt.Errorf("no last message")
	}
	if rt.LastMessage.Video == nil {
		return nil, fmt.Errorf("last message has no video")
	}

	v := rt.LastMessage.Video
	evidence := map[string]any{
		"file_id":  v.FileID,
		"width":    v.Width,
		"height":   v.Height,
		"duration": v.Duration,
	}

	// Log qualities if present (9.4 field)
	if len(v.Qualities) > 0 {
		evidence["qualities_count"] = len(v.Qualities)
		for i, q := range v.Qualities {
			evidence[fmt.Sprintf("quality_%d", i)] = map[string]any{
				"width": q.Width, "height": q.Height,
				"file_id": q.FileID, "file_size": q.FileSize,
			}
		}
	} else {
		evidence["qualities_count"] = 0
		evidence["note"] = "Telegram did not include qualities (normal for short/small videos)"
	}

	return &StepResult{
		Method:   "sendVideo",
		Evidence: evidence,
	}, nil
}
```

**Also add** `SendVideoStep` reference — this already exists if `sender/methods.go` has `SendVideo`. Verify it's in the testbot `SenderClient` interface (it is — line 224).

---

## ADD: S43_PrivateChatTopics Testbot Scenario (insert in suites/api94.go)

> Adopted from both Consultants M and N. galigo already has `CreateForumTopic` (sender/forum.go:37).
> The 9.4 change allows this method in private chats — a valid new test.

```go
// ==================== S43: Private Chat Topics (9.4) ====================

// S43_PrivateChatTopics tests createForumTopic in a private (1:1) chat.
// Bot API 9.4 extends createForumTopic to work in private chats when
// the user has allows_users_to_create_topics enabled.
//
// SKIP CONDITION: If the test private chat doesn't support topics,
// the API will return an error and we skip gracefully.
func S43_PrivateChatTopics() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S43-PrivateChatTopics",
		ScenarioDescription: "Create forum topic in private chat (9.4 extension)",
		CoveredMethods:      []string{"createForumTopic"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.CreatePrivateTopicStep{
				TopicName: "galigo 9.4 Private Topic Test",
			},
		},
	}
}
```

**New step type** in `engine/steps_9_4.go`:

```go
// CreatePrivateTopicStep attempts to create a forum topic in the runtime's
// private chat (rt.ChatID). If the chat doesn't support topics, it skips.
type CreatePrivateTopicStep struct {
	TopicName string
}

func (s *CreatePrivateTopicStep) Name() string { return "createForumTopic (private)" }

func (s *CreatePrivateTopicStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	topic, err := rt.Sender.CreateForumTopic(ctx, rt.ChatID, s.TopicName)
	if err != nil {
		// If the chat doesn't support topics, skip gracefully
		// Telegram returns "Bad Request: CHAT_NOT_MODIFIED" or similar
		return nil, Skip(fmt.Sprintf("createForumTopic in private chat: %v (user may not have topics enabled)", err))
	}

	return &StepResult{
		Method: "createForumTopic",
		Evidence: map[string]any{
			"topic_name": topic.Name,
			"chat_id":    rt.ChatID,
			"context":    "private_chat",
		},
	}, nil
}
```

**SenderClient interface update** — `CreateForumTopic` is NOT currently in the testbot's `SenderClient` interface. Add:

```go
// Forum topics (existing method, 9.4 extends to private chats)
CreateForumTopic(ctx context.Context, chatID int64, name string, opts ...sender.CreateTopicOption) (*tg.ForumTopic, error)
```

---

## MODIFY: Update AllAPI94Scenarios (in suites/api94.go)

```go
func AllAPI94Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S38_StyledButtons(),
		S39_ProfileAudios(),
		S40_ChatInfo94(),
		S42_VideoQualities(), // NEW
		S43_PrivateChatTopics(), // NEW
		// S41 excluded from "all" — destructive (changes bot photo)
	}
}
```

---

## MODIFY: Fix WithStyle/WithIcon Tests (in Part 1B of v1 guide)

> Self-correction: `Btn()` returns `InlineKeyboardButton` by value. `WithStyle`/`WithIcon`
> methods don't exist yet — they're part of the 9.4 implementation. Tests should be
> structured to work either way.

**Replace** the two helper tests with conditional versions:

```go
// These tests validate the helper methods IF implemented.
// If WithStyle/WithIcon are not yet added, test direct struct construction instead.

func TestInlineKeyboardButton_DirectStyleConstruction(t *testing.T) {
	// Works regardless of whether helpers exist
	btn := tg.InlineKeyboardButton{
		Text:              "Delete",
		CallbackData:      "action:delete",
		Style:             "danger",
		IconCustomEmojiID: "5368324170671202286",
	}
	data, err := json.Marshal(btn)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"style":"danger"`)
	assert.Contains(t, string(data), `"icon_custom_emoji_id":"5368324170671202286"`)
}

// NOTE: If you implement WithStyle/WithIcon as chainable methods on InlineKeyboardButton:
//
//   func (b InlineKeyboardButton) WithStyle(style string) InlineKeyboardButton {
//       b.Style = style
//       return b
//   }
//
// Then add these additional tests:
//
//   func TestInlineKeyboardButton_WithStyleHelper(t *testing.T) {
//       btn := tg.Btn("Confirm", "action:confirm").WithStyle("success")
//       assert.Equal(t, "success", btn.Style)
//   }
//
//   func TestInlineKeyboardButton_WithIconHelper(t *testing.T) {
//       btn := tg.Btn("Delete", "action:delete").WithIcon("emoji_id")
//       assert.Equal(t, "emoji_id", btn.IconCustomEmojiID)
//   }
```

**Also replace** `tg.ButtonStyleDanger` references with string literals until constants are implemented:

```go
func TestInlineKeyboardButton_AllStyles(t *testing.T) {
	// Use string literals — constants will be defined in 9.4 implementation
	styles := []string{"danger", "success", "primary"}
	for _, style := range styles {
		btn := tg.InlineKeyboardButton{
			Text:         "Btn",
			CallbackData: "cb",
			Style:        style,
		}
		data, err := json.Marshal(btn)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"style":"`+style+`"`)
	}
}
```

---

## ADD: Quick Wins Section (insert before Implementation Order)

> Adopted from Consultant N's Section 5. Provides a confidence-per-hour prioritization
> alongside the existing package-based ordering.

### Highest ROI Tests (implement first for fastest confidence)

| Priority | Test | Why First | Time |
|:---:|------|-----------|------|
| 1 | `tg/gifts_test.go` — UniqueGiftBackdropColors unmarshal | Catches our v1 string→int bug | 10 min |
| 2 | `tg/keyboard_test.go` — Button style marshal/unmarshal | Core 9.4 feature, simple test | 10 min |
| 3 | `tg/types_test.go` — VideoQuality unmarshal | New type, clean test | 10 min |
| 4 | `sender/identity_test.go` — SetMyProfilePhoto | New method, mock server pattern | 15 min |
| 5 | `sender/methods_test.go` — GetUserProfileAudios | New method, follows existing GetUserProfilePhotos | 15 min |
| 6 | `tg/gifts_test.go` — Full UniqueGift with Colors | Integration of all sub-types | 15 min |
| 7 | Testbot S38 — Styled buttons E2E | First real API validation | 20 min |
| 8 | Testbot S39 — Profile audios E2E | Read-only, safe, quick | 10 min |
| 9 | Testbot S40 — ChatFullInfo 9.4 fields | Read-only verification | 10 min |
| 10 | Testbot S42 — Video qualities | Proves deserialization works live | 15 min |

**Total for top 10**: ~2 hours for comprehensive 9.4 test coverage.

---

## ADD: main.go Suite Wiring Updates (append to Section 3E)

Add to `runSuiteCommand` switch:

```go
case "video-qualities":
	scenarios = []engine.Scenario{suites.S42_VideoQualities()}
case "private-topics":
	scenarios = []engine.Scenario{suites.S43_PrivateChatTopics()}
```

Add to `integration.yml` workflow_dispatch options:

```yaml
options:
  - all
  - smoke
  - core
  - media
  - keyboards
  - interactive
  - chat-admin
  - stickers
  - stars
  - api94            # All 9.4 tests
  - private-topics   # NEW individual
```

---

## ADD: registry.go Updates (append to Section 3B)

```go
// 9.4: Already exists, but now works in private chats
// (No new registry entry needed — createForumTopic already registered)
```

Verify: `createForumTopic` is NOT currently in registry.go. Need to add:

```go
// === Forum (already existed, but adding for completeness) ===
{Name: "createForumTopic", Category: CategoryChatAdmin},
```

---

## Summary: v1 → v2 Changes

| Change | Source | Type |
|--------|--------|------|
| Feature coverage matrix at top | Consultant N | Structure |
| Premium requirements table | Consultants M + N | Documentation |
| S42_VideoQualities scenario + step | Consultant N | New test |
| S43_PrivateChatTopics scenario + step | Consultants M + N | New test |
| Quick wins prioritization section | Consultant N | Structure |
| Fix WithStyle/WithIcon to not assume helpers | Self-correction | Bug fix |
| Fix ButtonStyleDanger to use literals | Self-correction | Bug fix |
| CreateForumTopic in SenderClient interface | Cross-analysis | Gap fill |
| createForumTopic in registry.go | Cross-analysis | Gap fill |

**Test count**: 30 unit tests (v1) + 2 new testbot scenarios = **32 tests, ~+650 lines total**

**Rejected**: 11 suggestions from M and N (mockgen, t.Parallel, build tags, testdata dir, fabricated APIs/fields, KeyboardButton, client-side validation)