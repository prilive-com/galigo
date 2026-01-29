# galigo Testing Delta Plan

## Focused Implementation Based on Actual Coverage Gaps

**Version:** 1.0 (Revised from developer feedback)  
**Approach:** Coverage-driven, delta-only  
**Estimated Effort:** 15-20 hours  
**Target:** 80%+ overall, 90% receiver/, 90% tg/

---

## Current State (Source of Truth)

| Package | Current | Target | Gap | Priority |
|---------|---------|--------|-----|----------|
| sender/ | 64.7% | 90% | 25.3% | Medium |
| receiver/ | 20.2% | 90% | **69.8%** | **CRITICAL** |
| tg/ | 22.3% | 90% | **67.7%** | **CRITICAL** |
| internal/testutil/ | 52.3% | 80% | OK | Skip |
| internal/syncutil/ | 100% | - | - | Skip |

---

## What Already Exists (DO NOT REIMPLEMENT)

| Component | Location | Status |
|-----------|----------|--------|
| FakeSleeper | `internal/testutil/sleeper.go` | ✅ Full functionality, uses `Calls()` |
| Reply helpers | `internal/testutil/replies.go` | ✅ ReplyOK, ReplyError, ReplyRateLimit, etc. |
| Mock server | `internal/testutil/` | ✅ Working |
| Retry tests | `sender/retry_test.go` | ✅ 15+ tests |
| Breaker tests | `sender/breaker_test.go` | ✅ Exists |
| Rate limit tests | `sender/ratelimit_test.go` | ✅ Exists |

---

## Phase 0: Baseline & Backlog (30 minutes)

### Run Coverage Analysis

```bash
go test ./... -race
go test ./... -coverpkg=./... -coverprofile=coverage.out
go tool cover -func=coverage.out | sort -k3 -n | head -60
```

### Output

Create `test_backlog.md` with:
- Lowest-coverage functions in receiver/
- Lowest-coverage functions in tg/
- Untested Tier-1 methods in sender/

This becomes the definitive work list.

---

## Phase 1: receiver/ Package (6-8 hours)

### 1.1 Polling Tests

**Files to create:**
- `receiver/polling_test.go`
- `receiver/polling_shutdown_test.go`
- `receiver/polling_offset_test.go`

**Must-cover behaviors:**

```go
// 1. Offset progression
func TestPolling_OffsetProgression(t *testing.T) {
    // Server returns updates IDs 100, 101
    // Next request must use offset 102
}

// 2. Offset only advances after delivery (update-loss fix)
func TestPolling_OffsetAdvancesOnlyAfterDelivery(t *testing.T) {
    // Verify offset not advanced until handler confirms
}

// 3. Graceful shutdown
func TestPolling_GracefulShutdown(t *testing.T) {
    // Cancel context
    // Assert goroutine exits, no leak, no more requests
}

// 4. Error resilience
func TestPolling_ServerError_Continues(t *testing.T) {
    // Server returns 500 then success
    // Assert continues without resetting offset
}

// 5. Backpressure policy
func TestPolling_ChannelFull_Policy(t *testing.T) {
    // Verify chosen policy (block+timeout+drop or block forever)
    // Assert offset behavior matches policy
}
```

### 1.2 Webhook Handler Tests

**Files to create:**
- `receiver/webhook_test.go`
- `receiver/webhook_security_test.go`

**Must-cover behaviors:**

```go
// 1. Method validation
func TestWebhook_NonPOST_Returns405(t *testing.T)

// 2. JSON validation
func TestWebhook_InvalidJSON_Returns400(t *testing.T)

// 3. Secret token - missing
func TestWebhook_MissingSecretToken_Rejects(t *testing.T) {
    // No X-Telegram-Bot-Api-Secret-Token header → reject
}

// 4. Secret token - wrong
func TestWebhook_WrongSecretToken_Rejects(t *testing.T) {
    // Wrong header value → reject
}

// 5. Secret token - correct
func TestWebhook_CorrectSecretToken_Accepts(t *testing.T) {
    // Correct header → accept and forward update
}

// 6. Update forwarding
func TestWebhook_ValidUpdate_ForwardsToHandler(t *testing.T)
```

### 1.3 Webhook API Tests (if receiver includes setWebhook, etc.)

**File:** `receiver/webhook_api_test.go`

```go
func TestSetWebhook_Success(t *testing.T)
func TestSetWebhook_TelegramError(t *testing.T)
func TestSetWebhook_IncludesSecretToken(t *testing.T)
func TestDeleteWebhook_Success(t *testing.T)
func TestGetWebhookInfo_Success(t *testing.T)
```

---

## Phase 2: tg/ Package (4-6 hours)

### 2.1 Envelope Parsing Tests (Highest Value)

**File:** `tg/envelope_test.go`

```go
// 1. Success parsing
func TestAPIResponse_OK_ParsesResult(t *testing.T) {
    data := `{"ok":true,"result":{"message_id":42}}`
    // Verify Result populated correctly
}

// 2. Error parsing
func TestAPIResponse_Error_MapsToAPIError(t *testing.T) {
    data := `{"ok":false,"error_code":400,"description":"Bad Request"}`
    // Verify error fields
}

// 3. ResponseParameters with retry_after
func TestAPIResponse_429_ParsesRetryAfter(t *testing.T) {
    data := `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":35}}`
    // Verify RetryAfter = 35
}

// 4. ResponseParameters with migrate_to_chat_id
func TestAPIResponse_Migration_ParsesMigrateToChatID(t *testing.T) {
    data := `{"ok":false,"error_code":400,"parameters":{"migrate_to_chat_id":-1001234567890}}`
}

// 5. Unknown fields ignored
func TestAPIResponse_UnknownFields_Ignored(t *testing.T)
```

### 2.2 Type Round-Trip Tests

**File:** `tg/types_roundtrip_test.go`

```go
// ChatID - int64
func TestChatID_Int64_RoundTrip(t *testing.T) {
    tests := []int64{123456, -1001234567890, math.MaxInt64}
    for _, id := range tests {
        chatID := tg.ChatIDFromInt64(id)
        data, _ := json.Marshal(chatID)
        var decoded tg.ChatID
        json.Unmarshal(data, &decoded)
        assert.Equal(t, id, decoded.Int64())
    }
}

// ChatID - username
func TestChatID_Username_RoundTrip(t *testing.T)

// InlineKeyboardMarkup
func TestInlineKeyboardMarkup_RoundTrip(t *testing.T)

// ReplyKeyboardMarkup
func TestReplyKeyboardMarkup_RoundTrip(t *testing.T)

// InputFile modes (file_id, url, upload metadata)
func TestInputFile_FileID_Marshal(t *testing.T)
func TestInputFile_URL_Marshal(t *testing.T)
func TestInputFile_Upload_NotMarshalable(t *testing.T) // Should error or be handled by multipart
```

### 2.3 Update Fixtures (Optional but Effective)

**Directory:** `tg/testdata/updates/`

```json
// message_text.json
{"update_id":123,"message":{"message_id":1,"text":"Hello"}}

// callback_query.json
{"update_id":124,"callback_query":{"id":"123","data":"btn_1"}}

// photo.json
{"update_id":125,"message":{"message_id":2,"photo":[{"file_id":"..."}]}}
```

**File:** `tg/update_fixtures_test.go`

```go
func TestDecodeUpdate_Fixtures(t *testing.T) {
    fixtures, _ := filepath.Glob("testdata/updates/*.json")
    for _, f := range fixtures {
        t.Run(filepath.Base(f), func(t *testing.T) {
            data, _ := os.ReadFile(f)
            var update tg.Update
            err := json.Unmarshal(data, &update)
            require.NoError(t, err)
            assert.NotZero(t, update.UpdateID)
        })
    }
}
```

---

## Phase 3: sender/ Method Tests for NEW Tier-1 Endpoints (4-6 hours)

### Important: Don't Duplicate Existing Tests

- Retry behavior → Already covered in `sender/retry_test.go`
- Breaker behavior → Already covered in `sender/breaker_test.go`
- Rate limiting → Already covered in `sender/ratelimit_test.go`

Only add method-specific contract tests.

### Test Client Configuration for Method Tests

```go
// Use breaker "never trip" and fast limiter for method tests
func newMethodTestClient(t *testing.T, serverURL string) *sender.Client {
    return testutil.NewTestClient(t, serverURL,
        sender.WithCircuitBreakerSettings(neverTripSettings()),
        sender.WithGlobalRPS(1000), // Fast for tests
    )
}
```

### 3.1 Basic Methods

**File:** `sender/basic_methods_test.go`

```go
// GetMe - 2 tests (already simple, no options)
func TestGetMe_Success(t *testing.T)
func TestGetMe_TelegramError(t *testing.T)

// GetFile - 3 tests
func TestGetFile_MinimalSuccess(t *testing.T)
func TestGetFile_TelegramError(t *testing.T)
```

### 3.2 Media Send Methods

**File:** `sender/send_media_test.go`

For each of: SendDocument, SendPhoto, SendVideo, SendAudio, SendAnimation, SendVoice, SendVideoNote, SendSticker

```go
// Pattern: 3 tests per method

// SendDocument
func TestSendDocument_FileID_MinimalSuccess(t *testing.T)
func TestSendDocument_Upload_MinimalSuccess(t *testing.T) // Multipart
func TestSendDocument_OptionsSuccess(t *testing.T)        // caption, parse_mode, thread_id, etc.
func TestSendDocument_TelegramError(t *testing.T)

// SendPhoto (same pattern)
func TestSendPhoto_FileID_MinimalSuccess(t *testing.T)
func TestSendPhoto_Upload_MinimalSuccess(t *testing.T)
func TestSendPhoto_OptionsSuccess(t *testing.T)
func TestSendPhoto_TelegramError(t *testing.T)

// ... etc for other media types
```

### 3.3 Media Group

**File:** `sender/media_group_test.go`

```go
func TestSendMediaGroup_MinimalSuccess(t *testing.T) {
    // Verify attach:// references generated
    // Verify file parts uploaded
}

func TestSendMediaGroup_MixedFileIDAndUpload(t *testing.T) {
    // First item file_id, second item upload
    // Verify correct handling
}

func TestSendMediaGroup_OptionsSuccess(t *testing.T) {
    // message_thread_id, business_connection_id
}

func TestSendMediaGroup_TelegramError(t *testing.T)
```

### 3.4 Edit Media

**File:** `sender/edit_media_test.go`

```go
func TestEditMessageMedia_MinimalSuccess(t *testing.T)
func TestEditMessageMedia_InlineMessageID(t *testing.T) // Different addressing
func TestEditMessageMedia_TelegramError(t *testing.T)
```

### 3.5 File Download

**File:** `sender/file_download_test.go`

```go
func TestDownloadFile_Success(t *testing.T)
func TestDownloadFile_Streaming(t *testing.T)      // Verify no full buffering
func TestDownloadFile_404Error(t *testing.T)
func TestDownloadFile_ContextCancel(t *testing.T)
```

---

## Testutil Patches (Only If Needed)

### Only add if currently missing:

**1. Body restoration after capture (if causing empty-body bug)**

```go
// In MockTelegramServer handler
body, _ := io.ReadAll(r.Body)
r.Body.Close()
r.Body = io.NopCloser(bytes.NewReader(body)) // RESTORE
```

**2. Multipart parsing helper (if Tier-1 includes multipart)**

```go
func ParseMultipart(t *testing.T, r *http.Request) *ParsedMultipart
func (pm *ParsedMultipart) AssertFormField(t, key, expected string)
func (pm *ParsedMultipart) AssertFilePart(t, key, filename string)
func (pm *ParsedMultipart) AssertAttachRef(t, field, attachName string)
```

---

## Summary: Actual Work Items

| Phase | Focus | Hours | Files |
|-------|-------|-------|-------|
| 0 | Baseline | 0.5 | coverage.out, test_backlog.md |
| 1 | receiver/ | 6-8 | polling_test.go, webhook_test.go |
| 2 | tg/ | 4-6 | envelope_test.go, types_roundtrip_test.go |
| 3 | sender/ methods | 4-6 | basic_methods_test.go, send_media_test.go, etc. |
| **Total** | | **15-20 hours** | |

---

## Process Change: Coverage-Driven Planning

To avoid future "plan mismatch" issues:

```bash
# 1. Generate coverage
go test ./... -coverpkg=./... -coverprofile=coverage.out

# 2. Find gaps
go tool cover -func=coverage.out | sort -k3 -n | head -60

# 3. Write tests for lowest-coverage functions first

# 4. Re-run coverage

# 5. Repeat until thresholds hit
```

This guarantees plans always derive from actual code state.

---

## Definition of Done

- [ ] `go test ./... -race` passes
- [ ] receiver/ coverage ≥ 90%
- [ ] tg/ coverage ≥ 90%
- [ ] sender/ coverage ≥ 80%
- [ ] Overall coverage ≥ 80%
- [ ] All new Tier-1 methods have 3-test pattern

---

## Decisions Summary

| Decision | Action |
|----------|--------|
| Skip harness rewrite | ✅ Only patch if specific need |
| Use existing FakeSleeper | ✅ Use `Calls()` API |
| Priority receiver/ + tg/ | ✅ Critical gaps first |
| No retry test duplication | ✅ Skip - 15+ tests exist |
| 3-test pattern for new methods | ✅ Minimal, options, error |
| Coverage-driven backlog | ✅ Adopt as standard process |

---

*galigo Testing Delta Plan - Focused on Actual Gaps*