# Tier 3 Testing Plan — Final Synthesis v3.0

**Version:** 3.0 (Synthesized from three independent analyses)  
**Date:** January 2026  
**Scope:** Unit tests + E2E testbot scenarios for ~61 Tier 3 methods  
**Status:** Ready for implementation

---

## Executive Summary

This plan synthesizes three independent analyses into a single authoritative implementation guide. Key decisions:

| Decision | Rationale |
|----------|-----------|
| **Domain naming** (`--run stickers`) | Clear, not internal jargon |
| **Keep interface pattern** | Existing 42-method interface works; embedding breaks adapter ergonomics |
| **Don't rewrite runner** | 429 retry already implemented correctly |
| **Unit tests for ALL methods** | Even business/games/payments — no credentials needed |
| **E2E only when safe** | Gated by env vars, not "skip forever" |
| **Multipart retry verification** | Critical: ensure reader not consumed on retry |
| **Fuzz polymorphic unmarshal** | Highest ROI for preventing panics |

---

## Part 1: Unit Tests (sender/)

### 1.1 Test Patterns (Apply to ALL Methods)

#### Pattern A: Validation Table Tests

```go
func TestSendInvoice_Validation(t *testing.T) {
    tests := []struct {
        name    string
        req     sender.SendInvoiceRequest
        wantErr string
    }{
        {
            name:    "missing_title",
            req:     sender.SendInvoiceRequest{ChatID: 123, Currency: "XTR"},
            wantErr: "title",
        },
        {
            name:    "title_too_long",
            req:     sender.SendInvoiceRequest{ChatID: 123, Title: strings.Repeat("a", 33)},
            wantErr: "must be 1-32 characters",
        },
        {
            name:    "missing_prices",
            req:     sender.SendInvoiceRequest{ChatID: 123, Title: "X", Description: "Y", Payload: "Z", Currency: "XTR"},
            wantErr: "prices",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            server := testutil.NewMockServer(t)
            // Handler should NOT be called — validation fails first
            
            client := testutil.NewTestClient(t, server.BaseURL())
            _, err := client.SendInvoice(context.Background(), tt.req)
            
            require.Error(t, err)
            assert.Contains(t, err.Error(), tt.wantErr)
            assert.Equal(t, 0, server.CaptureCount(), "validation must prevent network call")
        })
    }
}
```

#### Pattern B: Happy-Path Request Shape Tests

```go
func TestCreateNewStickerSet_RequestShape(t *testing.T) {
    server := testutil.NewMockServer(t)
    server.On("createNewStickerSet", func(w http.ResponseWriter, r *http.Request) {
        // Verify endpoint
        assert.Equal(t, "/bot"+testutil.TestToken+"/createNewStickerSet", r.URL.Path)
        
        // Verify multipart
        assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
        
        err := r.ParseMultipartForm(32 << 20)
        require.NoError(t, err)
        
        // Verify required fields
        assert.Equal(t, "123", r.FormValue("user_id"))
        assert.Equal(t, "test_set_by_testbot", r.FormValue("name"))
        assert.Equal(t, "Test Set", r.FormValue("title"))
        
        // Verify stickers JSON array
        stickersJSON := r.FormValue("stickers")
        var stickers []map[string]any
        json.Unmarshal([]byte(stickersJSON), &stickers)
        assert.Len(t, stickers, 1)
        assert.Equal(t, []any{"🧪"}, stickers[0]["emoji_list"])
        
        testutil.ReplyOK(w)
    })
    
    client := testutil.NewTestClient(t, server.BaseURL())
    err := client.CreateNewStickerSet(context.Background(), sender.CreateNewStickerSetRequest{
        UserID: 123,
        Name:   "test_set_by_testbot",
        Title:  "Test Set",
        Stickers: []sender.InputSticker{{
            Sticker:   sender.FromBytes([]byte{0x89, 0x50, 0x4E, 0x47}, "sticker.png"),
            Format:    "static",
            EmojiList: []string{"🧪"},
        }},
    })
    
    require.NoError(t, err)
}
```

#### Pattern C: API Error Mapping Tests

```go
func TestSendInvoice_APIErrors(t *testing.T) {
    tests := []struct {
        name       string
        statusCode int
        body       string
        wantCode   int
        wantRetry  int
    }{
        {
            name:       "bad_request",
            statusCode: 400,
            body:       `{"ok":false,"error_code":400,"description":"Bad Request: CURRENCY_INVALID"}`,
            wantCode:   400,
        },
        {
            name:       "rate_limited",
            statusCode: 429,
            body:       `{"ok":false,"error_code":429,"parameters":{"retry_after":30}}`,
            wantCode:   429,
            wantRetry:  30,
        },
        {
            name:       "forbidden",
            statusCode: 403,
            body:       `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked"}`,
            wantCode:   403,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            server := testutil.NewMockServer(t)
            server.On("sendInvoice", func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(tt.statusCode)
                w.Write([]byte(tt.body))
            })
            
            client := testutil.NewTestClient(t, server.BaseURL())
            _, err := client.SendInvoice(context.Background(), sender.SendInvoiceRequest{
                ChatID:      123,
                Title:       "Test",
                Description: "Test",
                Payload:     "test",
                Currency:    "XTR",
                Prices:      []tg.LabeledPrice{{Label: "Item", Amount: 100}},
            })
            
            var apiErr *tg.APIError
            require.ErrorAs(t, err, &apiErr)
            assert.Equal(t, tt.wantCode, apiErr.Code)
            if tt.wantRetry > 0 {
                assert.Equal(t, tt.wantRetry, apiErr.RetryAfter)
            }
        })
    }
}
```

#### Pattern D: Multipart Retry Test (CRITICAL)

```go
// TestMultipartRetry_ReaderNotConsumed verifies that multipart uploads
// can be retried after a 429 without losing file content.
// This catches the classic "reader consumed on retry" bug.
func TestUploadStickerFile_RetryPreservesContent(t *testing.T) {
    var attempts int
    var receivedContent []byte
    
    server := testutil.NewMockServer(t)
    server.On("uploadStickerFile", func(w http.ResponseWriter, r *http.Request) {
        attempts++
        
        err := r.ParseMultipartForm(32 << 20)
        require.NoError(t, err)
        
        file, _, err := r.FormFile("sticker")
        require.NoError(t, err)
        defer file.Close()
        
        content, _ := io.ReadAll(file)
        
        if attempts == 1 {
            // First attempt: return 429
            testutil.ReplyRateLimitWithRetryAfter(w, 1)
            return
        }
        
        // Second attempt: capture content and succeed
        receivedContent = content
        testutil.ReplyJSON(w, `{"ok":true,"result":{"file_id":"sticker_123","file_unique_id":"abc"}}`)
    })
    
    // Use retry client
    client := testutil.NewRetryTestClient(t, server.BaseURL())
    
    originalContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG header
    
    _, err := client.UploadStickerFile(context.Background(), sender.UploadStickerFileRequest{
        UserID:        123,
        Sticker:       sender.FromBytes(originalContent, "sticker.png"),
        StickerFormat: "static",
    })
    
    require.NoError(t, err)
    assert.Equal(t, 2, attempts, "should have retried after 429")
    assert.Equal(t, originalContent, receivedContent, "retry must send full content again")
}
```

#### Pattern E: NO RETRY Tests for Value Operations

```go
func TestRefundStarPayment_NeverRetries(t *testing.T) {
    var attempts atomic.Int32
    
    server := testutil.NewMockServer(t)
    server.On("refundStarPayment", func(w http.ResponseWriter, r *http.Request) {
        attempts.Add(1)
        testutil.ReplyRateLimitWithRetryAfter(w, 5)
    })
    
    // Use retry client (retries enabled) to prove it STILL doesn't retry
    client := testutil.NewRetryTestClient(t, server.BaseURL())
    
    err := client.RefundStarPayment(context.Background(), sender.RefundStarPaymentRequest{
        UserID:                  123,
        TelegramPaymentChargeID: "charge_123",
    })
    
    require.Error(t, err)
    assert.Equal(t, int32(1), attempts.Load(), "value operation must NOT retry even with retry client")
}

// Apply same pattern to ALL value operations:
// - refundStarPayment
// - answerPreCheckoutQuery
// - transferBusinessAccountStars  
// - transferGift
// - sendGift
// - upgradeGift
// - convertGiftToStars
```

### 1.2 Test File Organization

```
sender/
├── payments_test.go      # NEW: 7 methods (sendInvoice, createInvoiceLink, answerShippingQuery,
│                         #      answerPreCheckoutQuery, refundStarPayment, getStarTransactions,
│                         #      getMyStarBalance)
├── stickers_test.go      # NEW: 15 methods
├── games_test.go         # NEW: 5 methods (even without registered game!)
├── business_test.go      # EXPAND: 15 methods (unit tests work without business connection)
├── gifts_test.go         # EXPAND: 5 methods
├── verification_test.go  # EXPAND: 4 methods (unit tests work without authorization)
└── inline_test.go        # EXPAND: 4 methods
```

**Key insight:** Unit tests for business/games/verification methods work fine — they only need mock server responses, not actual Telegram credentials.

---

## Part 2: Unit Tests (tg/)

### 2.1 Polymorphic Unmarshal Tests

```go
// tg/transaction_partner_test.go

func TestUnmarshalTransactionPartner(t *testing.T) {
    tests := []struct {
        name     string
        json     string
        wantType string
        check    func(t *testing.T, p tg.TransactionPartner)
    }{
        // All known variants
        {
            name:     "user",
            json:     `{"type":"user","user":{"id":123,"is_bot":false,"first_name":"Test"}}`,
            wantType: "TransactionPartnerUser",
        },
        {
            name:     "fragment",
            json:     `{"type":"fragment","withdrawal_state":{"type":"succeeded","date":1706600000,"url":"https://fragment.com"}}`,
            wantType: "TransactionPartnerFragment",
        },
        {
            name:     "telegram_ads",
            json:     `{"type":"telegram_ads"}`,
            wantType: "TransactionPartnerTelegramAds",
        },
        {
            name:     "telegram_api",
            json:     `{"type":"telegram_api","request_count":100}`,
            wantType: "TransactionPartnerTelegramApi",
        },
        {
            name:     "affiliate_program",
            json:     `{"type":"affiliate_program","commission_per_mille":50}`,
            wantType: "TransactionPartnerAffiliateProgram",
        },
        {
            name:     "chat",
            json:     `{"type":"chat","chat":{"id":-100123,"type":"channel","title":"Test"}}`,
            wantType: "TransactionPartnerChat",
        },
        {
            name:     "other",
            json:     `{"type":"other"}`,
            wantType: "TransactionPartnerOther",
        },
        
        // CRITICAL: Unknown fallback tests
        {
            name:     "unknown_future_type",
            json:     `{"type":"super_premium_partner","new_field":"value"}`,
            wantType: "TransactionPartnerUnknown",
            check: func(t *testing.T, p tg.TransactionPartner) {
                u := p.(tg.TransactionPartnerUnknown)
                assert.Equal(t, "super_premium_partner", u.Type)
                assert.Contains(t, string(u.Raw), "new_field")
            },
        },
        {
            name:     "malformed_known_type",
            json:     `{"type":"user","user":"not_an_object"}`,
            wantType: "TransactionPartnerUnknown",
            check: func(t *testing.T, p tg.TransactionPartner) {
                u := p.(tg.TransactionPartnerUnknown)
                assert.Equal(t, "user", u.Type) // Preserves original type
            },
        },
        {
            name:     "missing_type",
            json:     `{"user":{"id":123}}`,
            wantType: "TransactionPartnerUnknown",
        },
        {
            name:     "empty_object",
            json:     `{}`,
            wantType: "TransactionPartnerUnknown",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tg.UnmarshalTransactionPartner(json.RawMessage(tt.json))
            
            typeName := reflect.TypeOf(result).Name()
            assert.Equal(t, tt.wantType, typeName)
            
            if tt.check != nil {
                tt.check(t, result)
            }
        })
    }
}
```

### 2.2 Fuzz Tests (Scoped to Unmarshal)

```go
// tg/fuzz_test.go

func FuzzUnmarshalTransactionPartner(f *testing.F) {
    // Seed with realistic examples
    f.Add([]byte(`{"type":"user","user":{"id":1}}`))
    f.Add([]byte(`{"type":"fragment"}`))
    f.Add([]byte(`{"type":"unknown_future"}`))
    f.Add([]byte(`{}`))
    f.Add([]byte(`null`))
    f.Add([]byte(`"string"`))
    f.Add([]byte(`[]`))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        // Must never panic
        _ = tg.UnmarshalTransactionPartner(data)
    })
}

func FuzzUnmarshalRevenueWithdrawalState(f *testing.F) {
    f.Add([]byte(`{"type":"pending"}`))
    f.Add([]byte(`{"type":"succeeded","date":1706600000,"url":"https://example.com"}`))
    f.Add([]byte(`{"type":"failed"}`))
    f.Add([]byte(`{"type":"future_state"}`))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        _ = tg.UnmarshalRevenueWithdrawalState(data)
    })
}

func FuzzUnmarshalChatBoostSource(f *testing.F) {
    f.Add([]byte(`{"source":"premium","user":{"id":1}}`))
    f.Add([]byte(`{"source":"gift_code","user":{"id":1}}`))
    f.Add([]byte(`{"source":"giveaway","user":{"id":1}}`))
    f.Add([]byte(`{"source":"future_source"}`))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        _ = tg.UnmarshalChatBoostSource(data)
    })
}

func FuzzChatBoostUnmarshalJSON(f *testing.F) {
    f.Add([]byte(`{"boost_id":"1","add_date":1706600000,"expiration_date":1709200000,"source":{"source":"premium","user":{"id":1}}}`))
    
    f.Fuzz(func(t *testing.T, data []byte) {
        var cb tg.ChatBoost
        _ = json.Unmarshal(data, &cb) // Must not panic
    })
}
```

---

## Part 3: Testbot E2E Suites

### 3.1 Suite Naming (Domain Names, Not Jargon)

```
suites/
├── stickers.go      # NOT tier3_stickers.go
├── stars.go         # NOT tier3_stars.go  
├── gifts.go         # NOT tier3_gifts.go
├── checklists.go    # NOT tier3_checklists.go
├── payments.go      # Gated (interactive)
├── business.go      # Gated (needs connection)
└── games.go         # Gated (needs registered game)
```

### 3.2 CLI Commands

```go
// main.go

// New commands
case "stickers":
    scenarios = suites.AllStickerScenarios()
case "stars":
    scenarios = suites.AllStarsScenarios()
case "gifts":
    scenarios = suites.AllGiftsScenarios()
case "checklists":
    scenarios = suites.AllChecklistScenarios()

// Gated commands (only run if env vars present)
case "payments":
    if !cfg.PaymentsEnabled() {
        logger.Warn("payments suite skipped: TESTBOT_ENABLE_PAYMENTS not set")
        return
    }
    scenarios = suites.AllPaymentScenarios()
case "business":
    if cfg.BusinessConnectionID == "" {
        logger.Warn("business suite skipped: BUSINESS_CONNECTION_ID not set")
        return
    }
    scenarios = suites.AllBusinessScenarios()
case "games":
    if cfg.GameShortName == "" {
        logger.Warn("games suite skipped: GAME_SHORT_NAME not set")
        return
    }
    scenarios = suites.AllGameScenarios()

// DEPRECATED: Keep for one release, then remove
case "tier3":
    logger.Warn("--run tier3 is deprecated, use --run stickers, --run stars, etc.")
    scenarios = append(suites.AllStickerScenarios(), suites.AllStarsScenarios()...)
    scenarios = append(scenarios, suites.AllGiftsScenarios()...)
    scenarios = append(scenarios, suites.AllChecklistScenarios()...)
```

### 3.3 Scenario Gating Mechanism (Simple)

```go
// engine/scenario.go

type BaseScenario struct {
    ScenarioName        string
    ScenarioDescription string
    CoveredMethods      []string
    ScenarioSteps       []Step
    ScenarioTimeout     time.Duration
    RequiresEnv         []string  // NEW: Required env vars
}

// ShouldSkip checks if scenario requirements are met.
func (s *BaseScenario) ShouldSkip() (bool, string) {
    for _, env := range s.RequiresEnv {
        if os.Getenv(env) == "" {
            return true, fmt.Sprintf("missing required env: %s", env)
        }
    }
    return false, ""
}
```

```go
// engine/runner.go — Filter before running

func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    // Check skip conditions
    if bs, ok := scenario.(*BaseScenario); ok {
        if skip, reason := bs.ShouldSkip(); skip {
            r.logger.Info("skipping scenario", "name", scenario.Name(), "reason", reason)
            return &ScenarioResult{
                ScenarioName: scenario.Name(),
                Success:      true, // Skipped is not failure
                Skipped:      true,
                SkipReason:   reason,
            }
        }
    }
    
    // ... rest of existing run logic ...
}
```

### 3.4 Sticker Suite (Self-Contained, No External Dependencies)

```go
// suites/stickers.go

package suites

import (
    "time"
    "github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S20_StickerSetLifecycle tests complete sticker set CRUD.
// Does NOT depend on any public sticker sets — fully self-contained.
func S20_StickerSetLifecycle() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S20-StickerSetLifecycle",
        ScenarioDescription: "Create, modify, query, and delete a sticker set (fully reversible)",
        CoveredMethods: []string{
            "uploadStickerFile",
            "createNewStickerSet",
            "getStickerSet",        // Test on OUR set, not "Animals"
            "addStickerToSet",
            "setStickerPositionInSet",
            "setStickerEmojiList",
            "setStickerKeywords",
            "setStickerSetTitle",
            "setStickerSetThumbnail",
            "deleteStickerFromSet",
            "deleteStickerSet",
        },
        ScenarioTimeout: 120 * time.Second,
        ScenarioSteps: []engine.Step{
            // Phase 1: Create
            &engine.UploadStickerFileStep{Format: "static"},
            &engine.CreateNewStickerSetStep{
                Title:  "galigo Test Set",
                Emojis: []string{"🧪"},
            },
            
            // Phase 2: Query OUR set (not a public one!)
            &engine.GetStickerSetStep{UseCreated: true}, // Uses rt.CreatedStickerSets[0]
            
            // Phase 3: Modify
            &engine.UploadStickerFileStep{Format: "static"},
            &engine.AddStickerToSetStep{Emojis: []string{"✅"}},
            &engine.SetStickerPositionInSetStep{Position: 0},
            &engine.SetStickerEmojiListStep{Emojis: []string{"🧪", "🔬"}},
            &engine.SetStickerKeywordsStep{Keywords: []string{"test", "galigo"}},
            &engine.SetStickerSetTitleStep{Title: "galigo Test Set (Updated)"},
            &engine.SetStickerSetThumbnailStep{},
            
            // Phase 4: Partial cleanup
            &engine.DeleteStickerFromSetStep{},
            
            // Phase 5: Full cleanup (delete entire set)
            &engine.DeleteStickerSetStep{},
        },
    }
}

// S21_CustomEmoji tests custom emoji retrieval.
func S21_CustomEmoji() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S21-CustomEmoji",
        ScenarioDescription: "Get custom emoji stickers by ID",
        CoveredMethods:      []string{"getCustomEmojiStickers"},
        ScenarioTimeout:     15 * time.Second,
        ScenarioSteps: []engine.Step{
            &engine.GetCustomEmojiStickersStep{
                // Use well-known custom emoji IDs (from Telegram Premium stickers)
                CustomEmojiIDs: []string{"5368324170671202286"},
            },
        },
    }
}

func AllStickerScenarios() []engine.Scenario {
    return []engine.Scenario{
        S20_StickerSetLifecycle(),
        S21_CustomEmoji(),
    }
}
```

### 3.5 Runtime Extensions for Cleanup

```go
// engine/scenario.go

type Runtime struct {
    // ... existing fields ...
    
    // Sticker set tracking for cleanup
    CreatedStickerSets []string
}

func (rt *Runtime) TrackStickerSet(name string) {
    rt.CreatedStickerSets = append(rt.CreatedStickerSets, name)
}
```

### 3.6 Runner Cleanup (Best-Effort, Don't Mask Errors)

```go
// engine/runner.go

func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    result := &ScenarioResult{
        ScenarioName: scenario.Name(),
        Covers:       scenario.Covers(),
        StartTime:    time.Now(),
    }
    
    // CRITICAL: Cleanup runs even on failure
    defer func() {
        cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        var cleanupErrors []string
        
        // Message cleanup
        for _, cm := range rt.CreatedMessages {
            if err := rt.Sender.DeleteMessage(cleanupCtx, cm.ChatID, cm.MessageID); err != nil {
                cleanupErrors = append(cleanupErrors, fmt.Sprintf("message %d: %v", cm.MessageID, err))
            }
        }
        
        // Sticker set cleanup
        for _, setName := range rt.CreatedStickerSets {
            if err := rt.Sender.DeleteStickerSet(cleanupCtx, setName); err != nil {
                cleanupErrors = append(cleanupErrors, fmt.Sprintf("sticker set %s: %v", setName, err))
            }
        }
        
        if len(cleanupErrors) > 0 {
            r.logger.Warn("cleanup had errors (not masking original result)",
                "errors", cleanupErrors,
                "messages_attempted", len(rt.CreatedMessages),
                "sticker_sets_attempted", len(rt.CreatedStickerSets))
        } else {
            r.logger.Info("cleanup completed",
                "messages_deleted", len(rt.CreatedMessages),
                "sticker_sets_deleted", len(rt.CreatedStickerSets))
        }
    }()
    
    // ... rest of run logic ...
}
```

### 3.7 Payment Suite (Gated, Interactive)

**Important correction:** Stars invoices allow empty provider_token, so gating should be by explicit opt-in, not token presence.

```go
// suites/payments.go

// S30_StarsReadOnly tests star balance and transactions (always safe).
func S30_StarsReadOnly() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S30-StarsReadOnly",
        ScenarioDescription: "Read star balance and transaction history (no spending)",
        CoveredMethods:      []string{"getMyStarBalance", "getStarTransactions"},
        ScenarioTimeout:     30 * time.Second,
        // NO RequiresEnv — always safe to run
        ScenarioSteps: []engine.Step{
            &engine.GetMyStarBalanceStep{},
            &engine.GetStarTransactionsStep{Limit: 10},
        },
    }
}

// S31_InvoiceFlow tests sending a Stars invoice (interactive, human must pay).
func S31_InvoiceFlow() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S31-InvoiceFlow",
        ScenarioDescription: "Send Stars invoice and handle payment callbacks (INTERACTIVE)",
        CoveredMethods:      []string{"sendInvoice", "answerShippingQuery", "answerPreCheckoutQuery"},
        ScenarioTimeout:     5 * time.Minute,
        RequiresEnv:         []string{"TESTBOT_ENABLE_PAYMENTS"}, // Explicit opt-in
        ScenarioSteps: []engine.Step{
            &engine.SendInvoiceStep{
                Title:         "galigo Test Item",
                Description:   "Test purchase for galigo-testbot",
                Payload:       "test_{{timestamp}}",
                Currency:      "XTR",
                ProviderToken: "", // Empty for Stars
                Prices:        []engine.LabeledPrice{{Label: "Test", Amount: 1}},
            },
            &engine.WaitForShippingQueryStep{Timeout: 2 * time.Minute},
            &engine.AnswerShippingQueryStep{OK: true},
            &engine.WaitForPreCheckoutQueryStep{Timeout: 2 * time.Minute},
            &engine.AnswerPreCheckoutQueryStep{OK: true}, // CRITICAL: No delay!
            &engine.CleanupStep{},
        },
    }
}
```

---

## Part 4: Registry Updates

```go
// registry/registry.go

const (
    CategoryMessaging  MethodCategory = "messaging"
    CategoryChatAdmin  MethodCategory = "chat-admin"
    CategoryLegacy     MethodCategory = "legacy"
    CategoryPayments   MethodCategory = "payments"
    CategoryStickers   MethodCategory = "stickers"
    CategoryGames      MethodCategory = "games"
    CategoryBusiness   MethodCategory = "business"
    CategoryGifts      MethodCategory = "gifts"
)

var AllMethods = []Method{
    // ... existing methods ...
    
    // Payments & Stars
    {Name: "sendInvoice", Category: CategoryPayments},
    {Name: "createInvoiceLink", Category: CategoryPayments},
    {Name: "answerShippingQuery", Category: CategoryPayments},
    {Name: "answerPreCheckoutQuery", Category: CategoryPayments, Notes: "10s deadline, skips rate limiter"},
    {Name: "refundStarPayment", Category: CategoryPayments, Notes: "value op, no retry"},
    {Name: "getStarTransactions", Category: CategoryPayments},
    {Name: "getMyStarBalance", Category: CategoryPayments},
    
    // Stickers (15 methods)
    {Name: "getStickerSet", Category: CategoryStickers},
    {Name: "getCustomEmojiStickers", Category: CategoryStickers},
    {Name: "uploadStickerFile", Category: CategoryStickers},
    {Name: "createNewStickerSet", Category: CategoryStickers},
    {Name: "addStickerToSet", Category: CategoryStickers},
    {Name: "setStickerPositionInSet", Category: CategoryStickers},
    {Name: "deleteStickerFromSet", Category: CategoryStickers},
    {Name: "replaceStickerInSet", Category: CategoryStickers},
    {Name: "setStickerEmojiList", Category: CategoryStickers},
    {Name: "setStickerKeywords", Category: CategoryStickers},
    {Name: "setStickerMaskPosition", Category: CategoryStickers},
    {Name: "setStickerSetTitle", Category: CategoryStickers},
    {Name: "setStickerSetThumbnail", Category: CategoryStickers},
    {Name: "setCustomEmojiStickerSetThumbnail", Category: CategoryStickers},
    {Name: "deleteStickerSet", Category: CategoryStickers},
    
    // Games (5 methods)
    {Name: "sendGame", Category: CategoryGames},
    {Name: "setGameScore", Category: CategoryGames},
    {Name: "getGameHighScores", Category: CategoryGames},
    
    // Business (15 methods)
    {Name: "getBusinessConnection", Category: CategoryBusiness},
    {Name: "setBusinessAccountName", Category: CategoryBusiness},
    {Name: "setBusinessAccountBio", Category: CategoryBusiness},
    {Name: "setBusinessAccountProfilePhoto", Category: CategoryBusiness},
    {Name: "removeBusinessAccountProfilePhoto", Category: CategoryBusiness},
    {Name: "setBusinessAccountUsername", Category: CategoryBusiness},
    {Name: "setBusinessAccountGiftSettings", Category: CategoryBusiness},
    {Name: "getBusinessAccountGifts", Category: CategoryBusiness},
    {Name: "getBusinessAccountStarBalance", Category: CategoryBusiness},
    {Name: "transferBusinessAccountStars", Category: CategoryBusiness, Notes: "value op, no retry"},
    {Name: "transferGift", Category: CategoryBusiness, Notes: "value op, no retry"},
    {Name: "postStory", Category: CategoryBusiness},
    {Name: "editStory", Category: CategoryBusiness},
    {Name: "deleteStory", Category: CategoryBusiness},
    {Name: "repostStory", Category: CategoryBusiness},
    
    // Gifts (5 methods)
    {Name: "getAvailableGifts", Category: CategoryGifts},
    {Name: "sendGift", Category: CategoryGifts, Notes: "value op, no retry"},
    {Name: "getUserGifts", Category: CategoryGifts},
    {Name: "upgradeGift", Category: CategoryGifts, Notes: "value op, no retry"},
    {Name: "convertGiftToStars", Category: CategoryGifts, Notes: "value op, no retry"},
    
    // Verification (4 methods)
    {Name: "verifyUser", Category: CategoryChatAdmin},
    {Name: "verifyChat", Category: CategoryChatAdmin},
    {Name: "removeUserVerification", Category: CategoryChatAdmin},
    {Name: "removeChatVerification", Category: CategoryChatAdmin},
    
    // Inline (4 methods)
    {Name: "answerInlineQuery", Category: CategoryMessaging},
    {Name: "answerWebAppQuery", Category: CategoryMessaging},
    {Name: "getUserChatBoosts", Category: CategoryMessaging},
    {Name: "setPassportDataErrors", Category: CategoryMessaging},
    
    // Bot API 9.x
    {Name: "sendChecklist", Category: CategoryMessaging},
    {Name: "editChecklist", Category: CategoryMessaging},
}
```

---

## Part 5: Implementation Checklist (PR Sequence)

### PR1: Unit Test Infrastructure
- [ ] Add `sender/payments_test.go` with all 5 patterns
- [ ] Add `sender/stickers_test.go` with all 5 patterns
- [ ] Add `sender/games_test.go` with all 5 patterns
- [ ] Expand `sender/business_test.go`
- [ ] Expand `sender/gifts_test.go`
- [ ] Run: `go test -race ./sender/...`

### PR2: Polymorphic Unmarshal Tests
- [ ] Add `tg/transaction_partner_test.go`
- [ ] Add `tg/revenue_withdrawal_state_test.go`
- [ ] Add `tg/fuzz_test.go`
- [ ] Run: `go test -fuzz=. -fuzztime=30s ./tg/...`

### PR3: Testbot Sticker Suite
- [ ] Create `suites/stickers.go` (S20, S21)
- [ ] Add `engine/steps_stickers.go`
- [ ] Add `Runtime.CreatedStickerSets` + cleanup
- [ ] Update registry with sticker methods
- [ ] Wire `--run stickers` in main.go
- [ ] Run: `./galigo-testbot -run stickers`

### PR4: Testbot Stars/Gifts/Checklists Suites
- [ ] Create `suites/stars.go` (S30)
- [ ] Create `suites/gifts.go` (S32)
- [ ] Create `suites/checklists.go` (S33)
- [ ] Wire new commands in main.go
- [ ] Update registry
- [ ] Run: `./galigo-testbot -run stars`

### PR5: Gating Infrastructure + Payment Suite
- [ ] Add `RequiresEnv` to BaseScenario
- [ ] Add skip logic to runner
- [ ] Add `Runtime.ShippingChan`, `PreCheckoutChan`
- [ ] Add update routing in main.go
- [ ] Create `suites/payments.go` (S31)
- [ ] Add `engine/steps_payments.go` (wait + answer steps)
- [ ] Wire `--run payments` with gating

### PR6: Deprecation + Cleanup
- [ ] Add `--run tier3` as deprecated alias (warns, then runs domain suites)
- [ ] Update README with new commands
- [ ] Remove deprecated alias in next release

---

## Summary of Final Decisions

| Decision | Source | Rationale |
|----------|--------|-----------|
| Domain naming | All three | Clearer than internal jargon |
| Keep interface | Developer + 3rd analyst | Embedding breaks existing ergonomics |
| Don't touch runner 429 | Developer + 3rd analyst | Already works |
| Unit tests for ALL methods | 3rd analyst | No credentials needed |
| Gating by opt-in, not token | 3rd analyst | Stars invoices allow empty token |
| Multipart retry test | 3rd analyst | Catches reader-consumed bug |
| Fuzz only unmarshal | Developer + 3rd analyst | Highest ROI, scoped |
| Test getStickerSet on self-created | Developer + 3rd analyst | Public sets are fragile |
| Best-effort cleanup | 3rd analyst | Don't mask original failure |
| FakeSleeper over synctest | Developer (conclusion) | Existing infra sufficient |
| RequiresEnv field | Developer | Simpler than ConditionalScenario |