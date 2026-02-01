# Code Review Request — galigo 438089f

**Date:** 2026-01-29
**Commit:** `438089f` (main)
**Previous reviewed:** `bc5ef57`
**Diff:** `git diff bc5ef57..438089f` — 51 files, +12,208 / -1,539 lines

---

## Summary

This commit adds **chat administration API methods** to the galigo Telegram Bot library and fixes a **retry-safety bug in file uploads**.

Three areas of change:

1. **New sender methods** — Chat info, settings, pins, moderation, polls, forum topics
2. **Retry-safe file uploads** — `FromBytes()` constructor with `Source` factory pattern
3. **Testbot acceptance scenarios** — 5 new scenarios (S15-S19) covering all chat admin methods

---

## 1. New Telegram API Methods

### tg/ package — New types

| File | Types |
|------|-------|
| `tg/chat_full_info.go` | `ChatFullInfo` — full chat info returned by getChat |
| `tg/chat_member.go` | `ChatMember` — polymorphic JSON unmarshal (owner/admin/member/restricted/left/banned) |
| `tg/chat_permissions.go` | `ChatPermissions` — granular chat permission flags |
| `tg/chat_admin_rights.go` | `ChatAdministratorRights` — admin permission flags |
| `tg/forum.go` | `Sticker`, `ForumTopic` |
| `tg/update.go` | `Poll`, `PollOption`, `PollAnswer` added |

**Review focus:** `ChatMember` uses polymorphic JSON unmarshaling based on `status` field — check correctness of type mapping and edge cases.

### sender/ package — New method files

| File | Methods | Pattern |
|------|---------|---------|
| `sender/chat_info.go` | GetChat, GetChatAdministrators, GetChatMemberCount, GetChatMember | `callJSON` (no retry) |
| `sender/chat_settings.go` | SetChatTitle, SetChatDescription | `callJSON` |
| `sender/chat_pin.go` | PinChatMessage, UnpinChatMessage, UnpinAllChatMessages | `callJSON` |
| `sender/chat_admin.go` | BanChatMember, UnbanChatMember, RestrictChatMember, PromoteChatMember, SetChatAdministratorCustomTitle | `callJSON` |
| `sender/chat_moderation.go` | Additional moderation methods | `callJSON` |
| `sender/polls.go` | SendPoll (regular + quiz), StopPoll | `callJSON` |
| `sender/forum.go` | GetForumTopicIconStickers, CreateForumTopic, EditForumTopic, CloseForumTopic, ReopenForumTopic, DeleteForumTopic | `callJSON` |
| `sender/call.go` | Generic `callJSON[T]` helper | Shared by all above |
| `sender/validate.go` | Input validation helpers (chatID, messageID, threadID) | Used in all methods |
| `sender/bulk_options.go` | Functional options for bulk/complex requests | Options pattern |

**Review focus:**
- All chat admin methods use `callJSON` (no retry, no rate limiting) — intentional because these are admin operations, not high-frequency sends. Is this the right choice?
- Input validation: methods validate parameters before making API calls. Check for missing validations.
- `callJSON[T]` generic helper eliminates boilerplate. Check type constraint correctness.

### Unit tests

Every new sender file has a corresponding `_test.go` with table-driven tests using `testutil.MockServer`:
- `chat_info_test.go`, `chat_settings_test.go`, `chat_pin_test.go`
- `chat_admin_test.go`, `chat_moderation_test.go`
- `polls_test.go`, `forum_test.go`
- `call_test.go`, `validate_test.go`, `security_test.go`
- `tg/chat_member_test.go`, `tg/chat_permissions_test.go`

---

## 2. Retry-Safe File Uploads (Bug Fix)

### Problem

`withRetry` in `sender/client.go` retries failed requests by calling the same closure again. For file uploads, the `io.Reader` inside `InputFile` is consumed on the first attempt. On retry, the reader is at EOF, causing Telegram to return `400 Bad Request: file must be non-empty`.

**Reproduction:** Run `--run all` (16 scenarios) — under load, transient 429/5xx errors trigger retry, and media upload scenarios fail with empty file.

### Fix

**`sender/inputfile.go`:**
- Added `Source func() io.Reader` field — factory that returns fresh reader per attempt
- Added `FromBytes(data []byte, filename string) InputFile` — creates `Source` from bytes
- Added `OpenReader() io.Reader` method — returns `Source()` if set, else `Reader`
- `IsUpload()` and `IsEmpty()` updated to check `Source`

**`sender/multipart.go`:**
- `handleInputFile`, `handleInputFileSlice`, `handleInputMedia` all call `file.OpenReader()` instead of `file.Reader` directly

**`sender/retry_test.go`** — Two new tests:
- `TestRetry_FileUpload_FromBytes_RetrySafe` — proves retry sends full file content
- `TestRetry_FileUpload_FromReader_EmptyOnRetry` — documents known limitation

**`cmd/galigo-testbot/engine/adapter.go`:**
- Changed `mediaInputToInputFile` from `FromReader(bytes.NewReader(...))` to `FromBytes(...)` — testbot now uses retry-safe uploads

**Review focus:**
- Is the `Source` factory approach the right pattern? Alternatives: seekable reader reset, copy-on-retry.
- `FromReader` remains single-use (backward compatible). Should it be deprecated?
- `OpenReader()` falls back to `Reader` — is silent fallback acceptable or should it warn?

---

## 3. Testbot Chat Admin Scenarios

### New files

| File | Content |
|------|---------|
| `cmd/galigo-testbot/engine/steps_chat_admin.go` | 13 step implementations for chat admin operations |
| `cmd/galigo-testbot/suites/chat_admin.go` | 5 scenarios (S15-S19) |
| `cmd/galigo-testbot/engine/adapter.go` | 15 new adapter methods wrapping sender client |
| `cmd/galigo-testbot/engine/scenario.go` | 15 new methods added to `SenderClient` interface |

### Scenarios

| Scenario | Steps | Methods Covered |
|----------|-------|-----------------|
| S15-ChatInfo | getChat, getChatAdministrators, getChatMemberCount, getChatMember | 4 methods |
| S16-ChatSettings | save title, setChatTitle, setChatDescription, restore | 2 methods |
| S17-PinMessages | sendMessage, pin, unpin, pin, unpinAll, cleanup | 3 methods |
| S18-Polls | sendPoll (simple), stopPoll, sendPoll (quiz), cleanup | 2 methods |
| S19-ForumStickers | getForumTopicIconStickers | 1 method |

**Test result:** All 16 scenarios (55 steps) pass against real Telegram API.

### Runner improvements

**`cmd/galigo-testbot/engine/runner.go`:**
- Added `RunnerConfig` struct with `BaseDelay`, `Jitter`, `MaxMessages`, `RetryOn429`, `Max429Retries`
- `pace()` — jittered delay between steps (base + random jitter)
- `runStepWithRetry()` — catches `tg.APIError` code 429, waits `RetryAfter + 500ms`, retries

**`cmd/galigo-testbot/config/config.go`:**
- New env vars: `TESTBOT_JITTER_INTERVAL`, `TESTBOT_RETRY_429`, `TESTBOT_MAX_429_RETRIES`

### Naming cleanup

Renamed internal "tier" terminology to user-friendly names:
- CLI: `--run tier2` → `--run chat-admin`
- Registry: `CategoryTier1`/`CategoryTier2` → `CategoryMessaging`/`CategoryChatAdmin`
- Files: `tier2.go` → `chat_admin.go`, `steps_tier2.go` → `steps_chat_admin.go`
- Functions: `AllTier2Scenarios()` → `AllChatAdminScenarios()`

---

## 4. Documentation

- **README.md** — Added chat admin API sections, `FromBytes` in File Uploads, updated package structure, testbot suite commands
- **CLAUDE.md** — Updated package structure, InputFile type docs, test phases table, CLI suite list

---

## Files Changed Summary

| Area | New Files | Modified Files |
|------|-----------|----------------|
| tg/ types | 5 new (+2 tests) | 1 modified (update.go) |
| sender/ methods | 12 new (+10 tests) | 4 modified |
| testbot/ engine | 1 new (steps_chat_admin.go) | 4 modified |
| testbot/ suites | 1 new (chat_admin.go) | 1 modified |
| testbot/ config | — | 2 modified |
| docs/ | 5 new | 1 modified |
| root docs | — | 2 modified (README.md, CLAUDE.md) |

---

## Questions for Reviewer

1. **callJSON vs withRetry:** Chat admin methods use `callJSON` (no retry). Should any of them (e.g., GetChat, GetChatMemberCount) use retry for resilience?

2. **FromBytes vs FromReader:** Should `FromReader` be deprecated in favor of `FromBytes`? Current approach: both exist, `FromReader` has a warning comment.

3. **ChatMember polymorphism:** Uses `status` field to determine concrete type. Is this robust enough, or should we use a more defensive approach?

4. **Runner 429 retry:** The testbot runner retries at the application level (not sender level). This creates two retry layers for send methods. Is this acceptable?

5. **Naming:** `chat-admin` as the CLI suite name for chat info + settings + pins + polls + forum. Good name, or should it be split?
