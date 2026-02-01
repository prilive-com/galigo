# Tier 3 Testing Plan v2.0 — Combined Analysis

**Version:** 2.0 (Synthesized from two independent analyses)  
**Date:** January 2026  
**Scope:** Unit tests + E2E testbot scenarios for ~61 Tier 3 methods  
**Constraints:** No payments provider, no business account, no registered game

---

## Executive Summary

This plan combines insights from two independent analyses to provide a comprehensive testing strategy that:

1. **Minimizes interface churn** — Use embedding instead of expanding `SenderClient`
2. **Enables real payment testing** — Route shipping/pre-checkout updates through new channels
3. **Self-skipping suites** — Env-var gating for restricted features
4. **Actually implements 429 retry** — Runner currently ignores `RetryOn429` config
5. **Deterministic timing tests** — Use Go 1.25's `testing/synctest`
6. **Incremental shippable PRs** — Each PR is deployable independently

---

## Part 1: Architecture Improvements

### 1.1 Avoid Interface Churn — Use Embedding

**Problem:** Continuously expanding `SenderClient` interface causes constant testbot changes.

**Solution:** Embed the real `sender.Client` in the adapter.

```go
// engine/adapter.go — IMPROVED DESIGN

// SenderAdapter wraps sender.Client and provides testbot-specific helpers.
// By embedding, we automatically expose ALL sender methods without interface changes.
type SenderAdapter struct {
    *sender.Client                    // Embed — all methods automatically available
    token      tg.SecretToken
    httpClient *http.Client
}

// NewSenderAdapter creates a new adapter.
func NewSenderAdapter(client *sender.Client) *SenderAdapter {
    return &SenderAdapter{Client: client}
}

// Testbot-specific helper methods (not in sender.Client)
func (a *SenderAdapter) WithToken(token tg.SecretToken) *SenderAdapter {
    a.token = token
    return a
}

// Override methods that need testbot-specific behavior
func (a *SenderAdapter) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
    options := &SendOptions{}
    for _, opt := range opts {
        opt(options)
    }
    
    req := sender.SendMessageRequest{
        ChatID:    chatID,
        Text:      text,
        ParseMode: tg.ParseMode(options.ParseMode),
    }
    if options.ReplyMarkup != nil {
        req.ReplyMarkup = options.ReplyMarkup
    }
    
    return a.Client.SendMessage(ctx, req)
}

// Webhook methods still need manual implementation (use receiver package)
func (a *SenderAdapter) SetWebhook(ctx context.Context, url string) error {
    return receiver.SetWebhook(ctx, a.httpClient, a.token, url, "")
}
```

**Benefits:**
- New Tier 3 methods automatically available in testbot
- No need to update `SenderClient` interface for each new method
- Only override methods that need testbot-specific behavior

### 1.2 Extend Runtime with Payment Channels

**Problem:** Interactive mode only routes callback queries.

**Solution:** Add channels for all interactive update types.

```go
// engine/scenario.go — EXTENDED RUNTIME

// Runtime provides context for step execution.
type Runtime struct {
    Sender *SenderAdapter  // Changed from interface to concrete type
    ChatID int64

    // State shared between steps
    CreatedMessages    []CreatedMessage
    LastMessage        *tg.Message
    LastMessageID      *tg.MessageID
    CapturedFileIDs    map[string]string
    CreatedStickerSets []string  // NEW: Track for cleanup

    // Interactive channels (populated in interactive mode)
    CallbackChan    chan *tg.CallbackQuery
    ShippingChan    chan *tg.ShippingQuery      // NEW: For payment flow
    PreCheckoutChan chan *tg.PreCheckoutQuery   // NEW: For payment flow
    InlineQueryChan chan *tg.InlineQuery        // NEW: For inline mode testing
    
    // Payment state
    LastInvoicePayload string  // Track for correlation
}

// NewRuntime creates a new runtime for scenario execution.
func NewRuntime(sender *SenderAdapter, chatID int64) *Runtime {
    return &Runtime{
        Sender:             sender,
        ChatID:             chatID,
        CreatedMessages:    make([]CreatedMessage, 0),
        CapturedFileIDs:    make(map[string]string),
        CreatedStickerSets: make([]string, 0),
    }
}

// TrackStickerSet adds a sticker set to the cleanup list.
func (rt *Runtime) TrackStickerSet(name string) {
    rt.CreatedStickerSets = append(rt.CreatedStickerSets, name)
}
```

### 1.3 Route All Update Types in main.go

```go
// main.go — EXTENDED UPDATE ROUTING

func runInteractiveSuite(cfg *config.Config, senderClient *sender.Client, logger *slog.Logger) {
    // ... existing setup ...
    
    // Create all channels
    callbackChan := make(chan *tg.CallbackQuery, 10)
    shippingChan := make(chan *tg.ShippingQuery, 10)
    preCheckoutChan := make(chan *tg.PreCheckoutQuery, 10)
    inlineQueryChan := make(chan *tg.InlineQuery, 10)
    
    rt := engine.NewRuntime(adapter, cfg.ChatID)
    rt.CallbackChan = callbackChan
    rt.ShippingChan = shippingChan
    rt.PreCheckoutChan = preCheckoutChan
    rt.InlineQueryChan = inlineQueryChan
    
    // Route updates to appropriate channels
    go func() {
        for update := range updates {
            switch {
            case update.CallbackQuery != nil:
                select {
                case callbackChan <- update.CallbackQuery:
                default:
                    logger.Warn("callback channel full, dropping")
                }
            case update.ShippingQuery != nil:
                select {
                case shippingChan <- update.ShippingQuery:
                default:
                    logger.Warn("shipping channel full, dropping")
                }
            case update.PreCheckoutQuery != nil:
                select {
                case preCheckoutChan <- update.PreCheckoutQuery:
                default:
                    logger.Warn("pre-checkout channel full, dropping")
                }
            case update.InlineQuery != nil:
                select {
                case inlineQueryChan <- update.InlineQuery:
                default:
                    logger.Warn("inline query channel full, dropping")
                }
            }
        }
    }()
    
    // ... rest of interactive mode ...
}
```

---

## Part 2: Self-Skipping Suites with Config Gating

### 2.1 Extended Config

```go
// config/config.go — EXTENDED

type Config struct {
    // Existing fields...
    Token           string
    ChatID          int64
    Mode            string
    SendInterval    time.Duration
    JitterInterval  time.Duration
    MaxMessagesPerRun int
    RetryOn429      bool
    Max429Retries   int
    StorageDir      string
    Admins          []int64
    
    // NEW: Tier 3 feature flags
    ProviderToken         string // Payment provider token (Stripe test mode)
    BusinessConnectionID  string // Business account connection ID
    GameShortName         string // Registered game short name
    AllowValueOps         bool   // Enable dangerous operations (refunds, transfers)
    StarBudget            int    // Max stars to spend in tests (safety limit)
    
    // NEW: Feature detection
    HasPayments() bool { return c.ProviderToken != "" }
    HasBusiness() bool { return c.BusinessConnectionID != "" }
    HasGames() bool    { return c.GameShortName != "" }
}

func Load() (*Config, error) {
    cfg := &Config{
        // ... existing defaults ...
        
        // Tier 3 features (all optional)
        ProviderToken:        os.Getenv("PROVIDER_TOKEN"),
        BusinessConnectionID: os.Getenv("BUSINESS_CONNECTION_ID"),
        GameShortName:        os.Getenv("GAME_SHORT_NAME"),
        AllowValueOps:        os.Getenv("ALLOW_VALUE_OPS") == "true",
        StarBudget:           parseIntOrDefault(os.Getenv("STAR_BUDGET"), 0),
    }
    return cfg, nil
}
```

### 2.2 Self-Skipping Scenario Pattern

```go
// engine/scenario.go — SKIP SUPPORT

// SkipReason explains why a scenario was skipped.
type SkipReason string

const (
    SkipNone              SkipReason = ""
    SkipNoPaymentProvider SkipReason = "PROVIDER_TOKEN not set"
    SkipNoBusiness        SkipReason = "BUSINESS_CONNECTION_ID not set"
    SkipNoGame            SkipReason = "GAME_SHORT_NAME not set"
    SkipValueOpsDisabled  SkipReason = "ALLOW_VALUE_OPS not true"
    SkipNoInlineMode      SkipReason = "Bot not configured for inline mode"
)

// ConditionalScenario wraps a scenario with skip conditions.
type ConditionalScenario struct {
    Scenario
    SkipIf func(cfg *config.Config) SkipReason
}

func (cs *ConditionalScenario) ShouldSkip(cfg *config.Config) SkipReason {
    if cs.SkipIf == nil {
        return SkipNone
    }
    return cs.SkipIf(cfg)
}
```

### 2.3 Suite Definitions with Gating

```go
// suites/tier3_payments.go

package suites

import (
    "time"
    "github.com/prilive-com/galigo/cmd/galigo-testbot/config"
    "github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// S30_StarsReadOnly tests star balance and transactions (always safe).
func S30_StarsReadOnly() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S30-StarsReadOnly",
        ScenarioDescription: "Read star balance and transaction history (no spending)",
        CoveredMethods:      []string{"getMyStarBalance", "getStarTransactions"},
        ScenarioTimeout:     30 * time.Second,
        ScenarioSteps: []engine.Step{
            &engine.GetMyStarBalanceStep{},
            &engine.GetStarTransactionsStep{Limit: 10},
        },
    }
}

// S31_PaymentFlow tests the full invoice → payment → confirmation flow.
// Requires: PROVIDER_TOKEN, interactive mode, human to complete payment.
func S31_PaymentFlow() *engine.ConditionalScenario {
    return &engine.ConditionalScenario{
        Scenario: &engine.BaseScenario{
            ScenarioName:        "S31-PaymentFlow",
            ScenarioDescription: "Full payment flow: invoice → shipping → pre-checkout → confirmation",
            CoveredMethods: []string{
                "sendInvoice",
                "answerShippingQuery",
                "answerPreCheckoutQuery",
            },
            ScenarioTimeout: 5 * time.Minute, // Human interaction needed
            ScenarioSteps: []engine.Step{
                &engine.SendInvoiceStep{
                    Title:       "Test Product",
                    Description: "galigo testbot payment test",
                    Payload:     "test_payload_{{timestamp}}",
                    Currency:    "XTR",
                    Prices:      []engine.LabeledPrice{{Label: "Test Item", Amount: 1}},
                },
                &engine.WaitForShippingQueryStep{
                    Timeout: 2 * time.Minute,
                },
                &engine.AnswerShippingQueryStep{
                    OK: true,
                    ShippingOptions: []engine.ShippingOption{
                        {ID: "free", Title: "Free Shipping", Prices: []engine.LabeledPrice{{Label: "Free", Amount: 0}}},
                    },
                },
                &engine.WaitForPreCheckoutQueryStep{
                    Timeout: 2 * time.Minute,
                },
                &engine.AnswerPreCheckoutQueryStep{
                    OK: true,
                    // CRITICAL: No delay here! Must respond within 10 seconds.
                },
                &engine.CleanupStep{},
            },
        },
        SkipIf: func(cfg *config.Config) engine.SkipReason {
            if !cfg.HasPayments() {
                return engine.SkipNoPaymentProvider
            }
            return engine.SkipNone
        },
    }
}

// S32_RefundPayment tests refunding a star payment (dangerous!).
// Requires: PROVIDER_TOKEN, ALLOW_VALUE_OPS, completed payment to refund.
func S32_RefundPayment() *engine.ConditionalScenario {
    return &engine.ConditionalScenario{
        Scenario: &engine.BaseScenario{
            ScenarioName:        "S32-RefundPayment",
            ScenarioDescription: "Refund a star payment (VALUE OPERATION - no retry)",
            CoveredMethods:      []string{"refundStarPayment"},
            ScenarioTimeout:     30 * time.Second,
            ScenarioSteps: []engine.Step{
                // This step would use a stored payment charge ID
                &engine.RefundStarPaymentStep{},
            },
        },
        SkipIf: func(cfg *config.Config) engine.SkipReason {
            if !cfg.HasPayments() {
                return engine.SkipNoPaymentProvider
            }
            if !cfg.AllowValueOps {
                return engine.SkipValueOpsDisabled
            }
            return engine.SkipNone
        },
    }
}
```

### 2.4 Sticker Suite (Fully Reversible)

```go
// suites/tier3_stickers.go

// S40_StickerSetLifecycle tests complete sticker set CRUD.
// Always safe: bot-created sets can be deleted.
func S40_StickerSetLifecycle() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S40-StickerSetLifecycle",
        ScenarioDescription: "Create, modify, and delete a sticker set (fully reversible)",
        CoveredMethods: []string{
            "uploadStickerFile",
            "createNewStickerSet",
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
            
            // Phase 2: Modify
            &engine.UploadStickerFileStep{Format: "static"}, // Second sticker
            &engine.AddStickerToSetStep{Emojis: []string{"✅"}},
            &engine.SetStickerPositionInSetStep{Position: 0},
            &engine.SetStickerEmojiListStep{Emojis: []string{"🧪", "🔬"}},
            &engine.SetStickerKeywordsStep{Keywords: []string{"test", "galigo"}},
            &engine.SetStickerSetTitleStep{Title: "galigo Test Set (Updated)"},
            &engine.SetStickerSetThumbnailStep{},
            
            // Phase 3: Cleanup (ALWAYS runs, even on failure)
            &engine.DeleteStickerFromSetStep{}, // Remove one sticker
            &engine.DeleteStickerSetStep{},     // Delete entire set
        },
    }
}

// S41_GetStickerSet tests reading an existing public sticker set.
func S41_GetStickerSet() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S41-GetStickerSet",
        ScenarioDescription: "Get info about a public sticker set",
        CoveredMethods:      []string{"getStickerSet"},
        ScenarioTimeout:     15 * time.Second,
        ScenarioSteps: []engine.Step{
            &engine.GetStickerSetStep{Name: "Animals"},
        },
    }
}

// S42_CustomEmoji tests custom emoji sticker retrieval.
func S42_CustomEmoji() engine.Scenario {
    return &engine.BaseScenario{
        ScenarioName:        "S42-CustomEmoji",
        ScenarioDescription: "Get custom emoji stickers by ID",
        CoveredMethods:      []string{"getCustomEmojiStickers"},
        ScenarioTimeout:     15 * time.Second,
        ScenarioSteps: []engine.Step{
            &engine.GetCustomEmojiStickersStep{
                // Use known custom emoji IDs (from a message with custom emoji)
                CustomEmojiIDs: []string{"5368324170671202286"}, // Example ID
            },
        },
    }
}
```

### 2.5 Business Suite (Gated)

```go
// suites/tier3_business.go

// S50_BusinessConnection tests business account connection info.
func S50_BusinessConnection() *engine.ConditionalScenario {
    return &engine.ConditionalScenario{
        Scenario: &engine.BaseScenario{
            ScenarioName:        "S50-BusinessConnection",
            ScenarioDescription: "Get and verify business connection",
            CoveredMethods:      []string{"getBusinessConnection"},
            ScenarioTimeout:     15 * time.Second,
            ScenarioSteps: []engine.Step{
                &engine.GetBusinessConnectionStep{},
            },
        },
        SkipIf: func(cfg *config.Config) engine.SkipReason {
            if !cfg.HasBusiness() {
                return engine.SkipNoBusiness
            }
            return engine.SkipNone
        },
    }
}

// S51_BusinessProfile tests setting business account profile.
func S51_BusinessProfile() *engine.ConditionalScenario {
    return &engine.ConditionalScenario{
        Scenario: &engine.BaseScenario{
            ScenarioName:        "S51-BusinessProfile",
            ScenarioDescription: "Update business account name, bio, username",
            CoveredMethods: []string{
                "setBusinessAccountName",
                "setBusinessAccountBio",
                "setBusinessAccountUsername",
            },
            ScenarioTimeout: 30 * time.Second,
            ScenarioSteps: []engine.Step{
                &engine.SaveBusinessProfileStep{}, // Store original values
                &engine.SetBusinessAccountNameStep{
                    FirstName: "galigo",
                    LastName:  "Test",
                },
                &engine.SetBusinessAccountBioStep{
                    Bio: "galigo testbot: testing business features",
                },
                &engine.RestoreBusinessProfileStep{}, // Restore original
            },
        },
        SkipIf: func(cfg *config.Config) engine.SkipReason {
            if !cfg.HasBusiness() {
                return engine.SkipNoBusiness
            }
            return engine.SkipNone
        },
    }
}

// S52_PostStory tests posting and editing stories.
func S52_PostStory() *engine.ConditionalScenario {
    return &engine.ConditionalScenario{
        Scenario: &engine.BaseScenario{
            ScenarioName:        "S52-PostStory",
            ScenarioDescription: "Post, edit, and delete a story",
            CoveredMethods:      []string{"postStory", "editStory", "deleteStory"},
            ScenarioTimeout:     60 * time.Second,
            ScenarioSteps: []engine.Step{
                &engine.PostStoryStep{},
                &engine.EditStoryStep{},
                &engine.DeleteStoryStep{}, // Cleanup
            },
        },
        SkipIf: func(cfg *config.Config) engine.SkipReason {
            if !cfg.HasBusiness() {
                return engine.SkipNoBusiness
            }
            return engine.SkipNone
        },
    }
}
```

---

## Part 3: Implement 429 Retry in Runner

**Critical Finding:** The runner has `RetryOn429` config but doesn't actually use it!

```go
// engine/runner.go — ADD 429 RETRY LOGIC

func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    // ... existing setup ...
    
    for _, step := range scenario.Steps() {
        stepResult := r.runStepWithRetry(ctx, step, rt)
        result.Steps = append(result.Steps, *stepResult)
        
        if !stepResult.Success {
            result.Success = false
            result.Error = stepResult.Error
            break
        }
        
        // Apply pacing between steps (but NOT after time-critical answers)
        if !isTimeCriticalStep(step) {
            r.applyPacing()
        }
    }
    
    // ... rest of method ...
}

func (r *Runner) runStepWithRetry(ctx context.Context, step Step, rt *Runtime) *StepResult {
    var lastErr error
    
    for attempt := 0; attempt <= r.config.Max429Retries; attempt++ {
        if attempt > 0 {
            r.logger.Info("retrying step after 429", "step", step.Name(), "attempt", attempt)
        }
        
        start := time.Now()
        result, err := step.Execute(ctx, rt)
        duration := time.Since(start)
        
        if err == nil {
            result.StepName = step.Name()
            result.Duration = duration
            result.Success = true
            return result
        }
        
        lastErr = err
        
        // Check if this is a 429 and we should retry
        if r.config.RetryOn429 && attempt < r.config.Max429Retries {
            if retryAfter, is429 := extract429RetryAfter(err); is429 {
                r.logger.Warn("rate limited, waiting",
                    "step", step.Name(),
                    "retry_after", retryAfter,
                    "attempt", attempt+1,
                    "max_attempts", r.config.Max429Retries+1)
                
                // Sleep for retry_after + small jitter
                sleepDuration := retryAfter + time.Duration(rand.Intn(500))*time.Millisecond
                time.Sleep(sleepDuration)
                continue
            }
        }
        
        // Not a 429, or 429 retry disabled, or max retries exceeded
        break
    }
    
    return &StepResult{
        StepName: step.Name(),
        Success:  false,
        Error:    lastErr.Error(),
    }
}

// extract429RetryAfter checks if error is a 429 and extracts retry_after duration.
func extract429RetryAfter(err error) (time.Duration, bool) {
    var apiErr *sender.APIError
    if errors.As(err, &apiErr) && apiErr.Code == 429 {
        retryAfter := time.Duration(apiErr.RetryAfter) * time.Second
        if retryAfter == 0 {
            retryAfter = 5 * time.Second // Default if not specified
        }
        return retryAfter, true
    }
    return 0, false
}

// isTimeCriticalStep returns true for steps that must not have pacing delays.
func isTimeCriticalStep(step Step) bool {
    switch step.(type) {
    case *AnswerPreCheckoutQueryStep,
         *AnswerShippingQueryStep,
         *AnswerCallbackQueryStep,
         *AnswerInlineQueryStep:
        return true
    }
    return false
}
```

---

## Part 4: Payment Interactive Steps

### 4.1 Wait Steps (10-Second Rule)

```go
// engine/steps_payments.go

// WaitForShippingQueryStep waits for a shipping query from the user.
type WaitForShippingQueryStep struct {
    Timeout time.Duration
}

func (s *WaitForShippingQueryStep) Name() string { return "waitForShippingQuery" }

func (s *WaitForShippingQueryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.ShippingChan == nil {
        return nil, fmt.Errorf("ShippingChan not set — run in interactive mode")
    }
    
    timeout := s.Timeout
    if timeout == 0 {
        timeout = 2 * time.Minute
    }
    
    select {
    case sq := <-rt.ShippingChan:
        rt.CapturedFileIDs["shipping_query_id"] = sq.ID
        return &StepResult{
            Method: "waitForShippingQuery",
            Evidence: map[string]any{
                "shipping_query_id": sq.ID,
                "invoice_payload":   sq.InvoicePayload,
            },
        }, nil
    case <-time.After(timeout):
        return nil, fmt.Errorf("timeout waiting for shipping query (%s) — user needs to tap Pay", timeout)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// WaitForPreCheckoutQueryStep waits for a pre-checkout query.
// CRITICAL: Must respond within 10 seconds after receiving!
type WaitForPreCheckoutQueryStep struct {
    Timeout time.Duration
}

func (s *WaitForPreCheckoutQueryStep) Name() string { return "waitForPreCheckoutQuery" }

func (s *WaitForPreCheckoutQueryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    if rt.PreCheckoutChan == nil {
        return nil, fmt.Errorf("PreCheckoutChan not set — run in interactive mode")
    }
    
    timeout := s.Timeout
    if timeout == 0 {
        timeout = 2 * time.Minute
    }
    
    select {
    case pcq := <-rt.PreCheckoutChan:
        rt.CapturedFileIDs["pre_checkout_query_id"] = pcq.ID
        rt.CapturedFileIDs["pre_checkout_currency"] = pcq.Currency
        return &StepResult{
            Method: "waitForPreCheckoutQuery",
            Evidence: map[string]any{
                "pre_checkout_query_id": pcq.ID,
                "total_amount":          pcq.TotalAmount,
                "currency":              pcq.Currency,
                "invoice_payload":       pcq.InvoicePayload,
            },
        }, nil
    case <-time.After(timeout):
        return nil, fmt.Errorf("timeout waiting for pre-checkout query (%s)", timeout)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// AnswerPreCheckoutQueryStep answers the pre-checkout query.
// CRITICAL: This is time-sensitive! Must complete within 10 seconds of receiving the query.
// The runner should NOT apply pacing delay before this step.
type AnswerPreCheckoutQueryStep struct {
    OK           bool
    ErrorMessage string // Required if OK is false
}

func (s *AnswerPreCheckoutQueryStep) Name() string { return "answerPreCheckoutQuery" }

func (s *AnswerPreCheckoutQueryStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
    queryID := rt.CapturedFileIDs["pre_checkout_query_id"]
    if queryID == "" {
        return nil, fmt.Errorf("no pre_checkout_query_id — run WaitForPreCheckoutQueryStep first")
    }
    
    // Create a tight context to ensure we respond quickly
    // (The 10-second rule is from Telegram's side, but we want to be well under)
    tightCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    err := rt.Sender.AnswerPreCheckoutQuery(tightCtx, sender.AnswerPreCheckoutQueryRequest{
        PreCheckoutQueryID: queryID,
        OK:                 s.OK,
        ErrorMessage:       s.ErrorMessage,
    })
    if err != nil {
        return nil, fmt.Errorf("answerPreCheckoutQuery failed (10s deadline): %w", err)
    }
    
    return &StepResult{
        Method: "answerPreCheckoutQuery",
        Evidence: map[string]any{
            "pre_checkout_query_id": queryID,
            "ok":                    s.OK,
        },
    }, nil
}
```

---

## Part 5: Unit Tests with Go 1.25 synctest

### 5.1 Deterministic Rate Limiter Tests

```go
// sender/rate_limiter_test.go

//go:build go1.25

package sender_test

import (
    "context"
    "testing"
    "testing/synctest"
    "time"
    
    "github.com/prilive-com/galigo/sender"
    "github.com/prilive-com/galigo/internal/testutil"
)

func TestRateLimiter_SkippedForPreCheckout(t *testing.T) {
    synctest.Run(func() {
        server := testutil.NewMockServer(t)
        server.On("answerPreCheckoutQuery", testutil.ReplyOK)
        
        // Create client with very slow rate limiter (1 request per hour)
        client, err := sender.New(
            testutil.TestToken,
            sender.WithBaseURL(server.BaseURL()),
            sender.WithRateLimiter(rate.NewLimiter(rate.Every(time.Hour), 1)),
        )
        require.NoError(t, err)
        
        // Consume the one allowed request
        client.GetMe(context.Background())
        
        // This should NOT wait for rate limiter (time-critical)
        ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
        defer cancel()
        
        err = client.AnswerPreCheckoutQuery(ctx, sender.AnswerPreCheckoutQueryRequest{
            PreCheckoutQueryID: "query_123",
            OK:                 true,
        })
        
        // Should succeed without timeout
        assert.NoError(t, err)
        assert.NotEqual(t, context.DeadlineExceeded, err)
    })
}

func TestBackoff_429RetryAfterRespected(t *testing.T) {
    synctest.Run(func() {
        attempts := 0
        server := testutil.NewMockServer(t)
        server.On("sendMessage", func(w http.ResponseWriter, r *http.Request) {
            attempts++
            if attempts == 1 {
                testutil.ReplyRateLimitWithRetryAfter(w, 5) // 5 seconds
                return
            }
            testutil.ReplyMessage(w, 1)
        })
        
        client := testutil.NewRetryTestClient(t, server.BaseURL())
        
        start := time.Now()
        _, err := client.SendMessage(context.Background(), sender.SendMessageRequest{
            ChatID: 123,
            Text:   "test",
        })
        elapsed := time.Since(start)
        
        assert.NoError(t, err)
        assert.Equal(t, 2, attempts)
        // With synctest, we can verify timing without real delays
        assert.GreaterOrEqual(t, elapsed, 5*time.Second)
    })
}
```

### 5.2 Union Type Decoding Tests

```go
// tg/transaction_partner_test.go

func TestUnmarshalTransactionPartner_AllVariants(t *testing.T) {
    tests := []struct {
        name           string
        json           string
        wantType       string
        wantFields     map[string]any
    }{
        {
            name:     "user",
            json:     `{"type":"user","user":{"id":123,"is_bot":false,"first_name":"Test"}}`,
            wantType: "TransactionPartnerUser",
            wantFields: map[string]any{
                "User.ID": int64(123),
            },
        },
        {
            name:     "fragment_with_succeeded_withdrawal",
            json:     `{"type":"fragment","withdrawal_state":{"type":"succeeded","date":1706600000,"url":"https://fragment.com/tx/123"}}`,
            wantType: "TransactionPartnerFragment",
            wantFields: map[string]any{
                "WithdrawalState.Type": "succeeded",
            },
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
            wantFields: map[string]any{
                "RequestCount": 100,
            },
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
        // CRITICAL: Unknown discriminator must not panic
        {
            name:     "unknown_future_type",
            json:     `{"type":"super_premium_partner","new_field":"value"}`,
            wantType: "TransactionPartnerUnknown",
            wantFields: map[string]any{
                "Type": "super_premium_partner",
            },
        },
        // CRITICAL: Malformed known type falls back to Unknown
        {
            name:     "malformed_user",
            json:     `{"type":"user","user":"not_an_object"}`,
            wantType: "TransactionPartnerUnknown",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tg.UnmarshalTransactionPartner(json.RawMessage(tt.json))
            
            typeName := reflect.TypeOf(result).Name()
            assert.Equal(t, tt.wantType, typeName, "type mismatch")
            
            // Verify specific fields if provided
            for path, want := range tt.wantFields {
                got := getFieldByPath(result, path)
                assert.Equal(t, want, got, "field %s mismatch", path)
            }
        })
    }
}

func TestUnmarshalRevenueWithdrawalState_AllVariants(t *testing.T) {
    tests := []struct {
        name     string
        json     string
        wantType string
    }{
        {
            name:     "pending",
            json:     `{"type":"pending"}`,
            wantType: "RevenueWithdrawalStatePending",
        },
        {
            name:     "succeeded",
            json:     `{"type":"succeeded","date":1706600000,"url":"https://fragment.com"}`,
            wantType: "RevenueWithdrawalStateSucceeded",
        },
        {
            name:     "failed",
            json:     `{"type":"failed"}`,
            wantType: "RevenueWithdrawalStateFailed",
        },
        {
            name:     "unknown_future",
            json:     `{"type":"processing"}`,
            wantType: "RevenueWithdrawalStateUnknown",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := tg.UnmarshalRevenueWithdrawalState(json.RawMessage(tt.json))
            typeName := reflect.TypeOf(result).Name()
            assert.Equal(t, tt.wantType, typeName)
        })
    }
}
```

---

## Part 6: Cleanup System for Tier 3

### 6.1 Sticker Set Cleanup

```go
// cleanup/stickers.go

package cleanup

import (
    "context"
    "log/slog"
    
    "github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// CleanupStickerSets deletes all sticker sets tracked in the runtime.
func CleanupStickerSets(ctx context.Context, rt *engine.Runtime, logger *slog.Logger) {
    for _, setName := range rt.CreatedStickerSets {
        logger.Info("cleaning up sticker set", "name", setName)
        
        if err := rt.Sender.DeleteStickerSet(ctx, setName); err != nil {
            logger.Warn("failed to delete sticker set", "name", setName, "error", err)
        }
    }
    
    rt.CreatedStickerSets = rt.CreatedStickerSets[:0]
}
```

### 6.2 Extend Runner for Cleanup-on-Failure

```go
// engine/runner.go — CLEANUP ON FAILURE

func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
    // ... existing code ...
    
    // Always run cleanup, even on failure
    defer func() {
        cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        
        // Message cleanup
        for _, cm := range rt.CreatedMessages {
            _ = rt.Sender.DeleteMessage(cleanupCtx, cm.ChatID, cm.MessageID)
        }
        
        // Sticker set cleanup
        for _, setName := range rt.CreatedStickerSets {
            _ = rt.Sender.DeleteStickerSet(cleanupCtx, setName)
        }
        
        r.logger.Info("cleanup completed",
            "messages_deleted", len(rt.CreatedMessages),
            "sticker_sets_deleted", len(rt.CreatedStickerSets))
    }()
    
    // ... rest of method ...
}
```

---

## Part 7: Shippable PR Sequence

### PR1: Plumbing & Infrastructure (No New Tests)
- Extend `Runtime` with new channels
- Route shipping/pre-checkout/inline updates in `main.go`
- Add `RetryOn429` implementation to runner
- Add `isTimeCriticalStep()` function
- Add sticker set tracking to `Runtime`

### PR2: Stickers Suite (Non-Monetary, Fully Reversible)
- Add `suites/tier3_stickers.go` (S40, S41, S42)
- Add `engine/steps_stickers.go`
- Add sticker cleanup in `cleanup/stickers.go`
- Add sticker fixture if needed
- Update registry with sticker methods

### PR3: Stars Read-Only Suite (Always Safe)
- Add `suites/tier3_stars.go` (S30)
- Add `engine/steps_stars.go`
- Update registry with star methods
- Works even if bot has no star balance

### PR4: Gifts & Boosts Read-Only
- Add gift/boost scenarios (S60-S62)
- Read-only operations, always safe
- Update registry

### PR5: Payment Interactive Suite (Gated)
- Add `suites/tier3_payments.go` (S31, S32)
- Add `engine/steps_payments.go` (wait + answer steps)
- Gated behind `PROVIDER_TOKEN` + `ALLOW_VALUE_OPS`
- Requires interactive mode

### PR6: Business Suite (Gated)
- Add `suites/tier3_business.go` (S50-S52)
- Gated behind `BUSINESS_CONNECTION_ID`
- Includes save/restore for profile changes

### PR7: Library Unit Tests Sweep
- `sender/payments_test.go`
- `sender/stickers_test.go`
- `sender/business_test.go`
- `tg/*_test.go` (union decoding)
- Fuzz tests
- Go 1.25 synctest timing tests

---

## Part 8: Final Coverage Matrix

### Unit Tests (Library)

| Epic | Methods | Tests | Pattern |
|------|---------|-------|---------|
| Payments | 7 | 21+ | Success, validation, no-retry |
| Stickers | 15 | 45+ | Success, validation, multipart |
| Games | 5 | 15+ | Success, validation, returns |
| Business | 15 | 45+ | Success, validation, multipart |
| Verification | 4 | 12+ | Success, 403 handling |
| Gifts | 5 | 15+ | Success, validation, no-retry |
| Inline | 4 | 12+ | Success, validation |
| **Total** | **55** | **165+** | |

### E2E Tests (Testbot)

| Suite | Methods | Scenarios | Gating |
|-------|---------|-----------|--------|
| Stars Read-Only | 2 | S30 | None (always safe) |
| Stickers | 13 | S40-S42 | None (reversible) |
| Gifts Read-Only | 2 | S60-S61 | None (always safe) |
| Boosts | 1 | S62 | None (always safe) |
| Checklists | 2 | S63 | None (always safe) |
| **Always Safe** | **20** | **7** | — |
| Payments Full | 3 | S31 | `PROVIDER_TOKEN` |
| Refunds | 1 | S32 | `PROVIDER_TOKEN` + `ALLOW_VALUE_OPS` |
| Business | 10 | S50-S52 | `BUSINESS_CONNECTION_ID` |
| Games | 5 | S70-S72 | `GAME_SHORT_NAME` |
| **Gated** | **19** | **6** | Env vars required |
| **Total** | **39** | **13** | |

---

## Appendix: Environment Variables

```bash
# Required
TESTBOT_TOKEN=123456:ABC...
TESTBOT_CHAT_ID=-100123456789

# Optional: Tier 3 features
PROVIDER_TOKEN=284685063:TEST:...  # Stripe test mode from @BotFather
BUSINESS_CONNECTION_ID=abc123...   # From getBusinessConnection
GAME_SHORT_NAME=my_game            # Registered via @BotFather

# Safety switches
ALLOW_VALUE_OPS=true               # Enable refunds, transfers, etc.
STAR_BUDGET=100                    # Max stars to spend in tests

# Behavior tuning
TESTBOT_LOG_LEVEL=debug
TESTBOT_SEND_INTERVAL=1200ms
TESTBOT_JITTER=300ms
TESTBOT_RETRY_ON_429=true
TESTBOT_MAX_429_RETRIES=3
```

---

## Appendix: Command Reference

```bash
# Run all safe Tier 3 tests
./galigo-testbot -run tier3

# Run specific suites
./galigo-testbot -run stickers
./galigo-testbot -run stars
./galigo-testbot -run gifts

# Run payment flow (requires PROVIDER_TOKEN + interactive mode)
./galigo-testbot -run payments

# Run with all features enabled
PROVIDER_TOKEN=... ALLOW_VALUE_OPS=true ./galigo-testbot -run tier3-full

# Check coverage
./galigo-testbot --status

# Run in stress mode (faster pacing)
./galigo-testbot -run all --stress
```