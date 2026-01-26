# galigo Tier 1 Implementation - Final Consolidated Adjustments

## Overview

This document consolidates the best solutions from multiple code reviews for the Tier 1 implementation concerns.

---

## 1. P0.1 Update Loss Bug - Delivery Policy + Timeout

### Final Solution: Configurable Policy with Safe Defaults

**Key Insight:** Make the delivery policy explicit rather than implicit blocking.

```go
// receiver/config.go
package receiver

import "time"

// UpdateDeliveryPolicy defines how updates are handled when channel is full.
type UpdateDeliveryPolicy int

const (
    // DeliveryPolicyBlock waits for channel space (with timeout).
    // This is the safest default - no updates lost unless timeout.
    DeliveryPolicyBlock UpdateDeliveryPolicy = iota
    
    // DeliveryPolicyDropNewest drops the current update if channel is full.
    // Offset advances - update is lost but polling continues.
    DeliveryPolicyDropNewest
    
    // DeliveryPolicyDropOldest drops oldest update to make room.
    // Requires ring buffer implementation.
    DeliveryPolicyDropOldest
)

type PollingConfig struct {
    // ... existing fields ...
    
    // UpdateDeliveryPolicy defines behavior when update channel is full.
    // Default: DeliveryPolicyBlock
    UpdateDeliveryPolicy UpdateDeliveryPolicy
    
    // UpdateDeliveryTimeout is max time to wait in Block mode.
    // After timeout, falls back to drop + advance offset.
    // Zero means block forever (dangerous - use with caution).
    // Default: 5 seconds
    UpdateDeliveryTimeout time.Duration
    
    // OnUpdateDropped is called when an update is dropped (for metrics/alerting).
    // Optional.
    OnUpdateDropped func(updateID int, reason string)
}

func DefaultPollingConfig() PollingConfig {
    return PollingConfig{
        Timeout:               30,
        Limit:                 100,
        UpdateDeliveryPolicy:  DeliveryPolicyBlock,
        UpdateDeliveryTimeout: 5 * time.Second,
    }
}
```

```go
// receiver/polling.go
package receiver

import (
    "context"
    "fmt"
    "time"
)

func (c *PollingClient) deliverUpdates(ctx context.Context, updates []tg.Update) error {
    for _, update := range updates {
        if err := c.deliverUpdate(ctx, update); err != nil {
            return err
        }
    }
    return nil
}

func (c *PollingClient) deliverUpdate(ctx context.Context, update tg.Update) error {
    switch c.config.UpdateDeliveryPolicy {
    case DeliveryPolicyBlock:
        return c.deliverBlocking(ctx, update)
    case DeliveryPolicyDropNewest:
        return c.deliverDropNewest(ctx, update)
    case DeliveryPolicyDropOldest:
        return c.deliverDropOldest(ctx, update)
    default:
        return c.deliverBlocking(ctx, update)
    }
}

func (c *PollingClient) deliverBlocking(ctx context.Context, update tg.Update) error {
    // Create delivery context with timeout (if configured)
    deliveryCtx := ctx
    var cancel context.CancelFunc
    
    if c.config.UpdateDeliveryTimeout > 0 {
        deliveryCtx, cancel = context.WithTimeout(ctx, c.config.UpdateDeliveryTimeout)
        defer cancel()
    }
    
    select {
    case c.updates <- update:
        // ✅ Only advance offset after successful delivery
        c.offset = update.UpdateID + 1
        return nil
        
    case <-deliveryCtx.Done():
        if ctx.Err() != nil {
            // Parent context cancelled - normal shutdown
            return ctx.Err()
        }
        
        // Delivery timeout - drop update, advance offset, continue
        c.logger.Warn("update delivery timeout, dropping",
            "update_id", update.UpdateID,
            "timeout", c.config.UpdateDeliveryTimeout,
        )
        
        if c.config.OnUpdateDropped != nil {
            c.config.OnUpdateDropped(update.UpdateID, "delivery_timeout")
        }
        
        // ✅ Advance offset to prevent infinite retry loop
        c.offset = update.UpdateID + 1
        return nil
    }
}

func (c *PollingClient) deliverDropNewest(ctx context.Context, update tg.Update) error {
    select {
    case c.updates <- update:
        c.offset = update.UpdateID + 1
        return nil
        
    default:
        // Channel full - drop this update
        c.logger.Warn("channel full, dropping newest update",
            "update_id", update.UpdateID,
        )
        
        if c.config.OnUpdateDropped != nil {
            c.config.OnUpdateDropped(update.UpdateID, "channel_full_drop_newest")
        }
        
        // Advance offset - intentionally dropping
        c.offset = update.UpdateID + 1
        return nil
        
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (c *PollingClient) deliverDropOldest(ctx context.Context, update tg.Update) error {
    for {
        select {
        case c.updates <- update:
            c.offset = update.UpdateID + 1
            return nil
            
        default:
            // Channel full - try to drain oldest
            select {
            case dropped := <-c.updates:
                c.logger.Warn("channel full, dropping oldest update",
                    "dropped_id", dropped.UpdateID,
                    "new_id", update.UpdateID,
                )
                if c.config.OnUpdateDropped != nil {
                    c.config.OnUpdateDropped(dropped.UpdateID, "channel_full_drop_oldest")
                }
                // Loop and try to send again
                
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
}
```

### Configuration Options for Users

```go
// Option functions for easy configuration
func WithDeliveryPolicy(policy UpdateDeliveryPolicy) PollingOption {
    return func(c *PollingConfig) {
        c.UpdateDeliveryPolicy = policy
    }
}

func WithDeliveryTimeout(timeout time.Duration) PollingOption {
    return func(c *PollingConfig) {
        c.UpdateDeliveryTimeout = timeout
    }
}

func WithUpdateDroppedCallback(fn func(updateID int, reason string)) PollingOption {
    return func(c *PollingConfig) {
        c.OnUpdateDropped = fn
    }
}
```

### Usage Examples

```go
// Safe default (recommended)
receiver, _ := receiver.NewPolling(token) // Block + 5s timeout

// High-throughput bot that can't afford to wait
receiver, _ := receiver.NewPolling(token,
    receiver.WithDeliveryPolicy(receiver.DeliveryPolicyDropNewest),
)

// Bot with monitoring
receiver, _ := receiver.NewPolling(token,
    receiver.WithDeliveryTimeout(10*time.Second),
    receiver.WithUpdateDroppedCallback(func(id int, reason string) {
        metrics.IncrCounter("updates_dropped", 1, "reason", reason)
        alerting.Warn("Update dropped", "id", id, "reason", reason)
    }),
)
```

---

## 2. Error Model - Compatibility Layer

### Final Solution: Type Aliases with Deprecation Path

**Key Insight:** Keep canonical errors in `tg`, provide aliases in `sender` for backward compatibility.

```go
// tg/errors.go - Canonical location (new)
package tg

import (
    "errors"
    "fmt"
    "time"
)

// Sentinel errors for Telegram API responses
var (
    ErrUnauthorized       = errors.New("galigo: unauthorized")
    ErrForbidden          = errors.New("galigo: forbidden")
    ErrNotFound           = errors.New("galigo: not found")
    ErrConflict           = errors.New("galigo: conflict")
    ErrTooManyRequests    = errors.New("galigo: too many requests")
    ErrBotBlocked         = errors.New("galigo: bot was blocked by user")
    ErrChatNotFound       = errors.New("galigo: chat not found")
    ErrMessageNotFound    = errors.New("galigo: message not found")
    ErrMessageNotModified = errors.New("galigo: message not modified")
    ErrQueryTooOld        = errors.New("galigo: query is too old")
)

// Circuit breaker / retry errors
var (
    ErrCircuitOpen = errors.New("galigo: circuit breaker is open")
    ErrMaxRetries  = errors.New("galigo: max retries exceeded")
    ErrRateLimited = errors.New("galigo: rate limited")
)

// APIError represents an error returned by the Telegram Bot API.
type APIError struct {
    Code        int
    Description string
    RetryAfter  time.Duration
    Parameters  *ResponseParameters
}

func (e *APIError) Error() string {
    if e.RetryAfter > 0 {
        return fmt.Sprintf("telegram: %s (code %d, retry after %v)", 
            e.Description, e.Code, e.RetryAfter)
    }
    return fmt.Sprintf("telegram: %s (code %d)", e.Description, e.Code)
}

// Is implements errors.Is for sentinel error matching.
func (e *APIError) Is(target error) bool {
    switch {
    case target == ErrUnauthorized:
        return e.Code == 401
    case target == ErrForbidden:
        return e.Code == 403
    case target == ErrNotFound:
        return e.Code == 404
    case target == ErrConflict:
        return e.Code == 409
    case target == ErrTooManyRequests:
        return e.Code == 429
    case target == ErrBotBlocked:
        return e.Code == 403 && containsAny(e.Description, "blocked", "kicked")
    case target == ErrChatNotFound:
        return e.Code == 400 && contains(e.Description, "chat not found")
    case target == ErrMessageNotFound:
        return e.Code == 400 && contains(e.Description, "message to edit not found")
    case target == ErrMessageNotModified:
        return e.Code == 400 && contains(e.Description, "message is not modified")
    }
    return false
}

// IsRetryable returns true if the error is transient and request can be retried.
func (e *APIError) IsRetryable() bool {
    return e.Code == 429 || (e.Code >= 500 && e.Code <= 504)
}
```

```go
// sender/errors.go - Backward compatibility aliases
package sender

import "github.com/prliv-com/galigo/tg"

// Type alias for APIError - maintains full compatibility
type APIError = tg.APIError

// Sentinel error aliases
// Deprecated: Use tg.ErrUnauthorized instead. Will be removed in v2.0.
var ErrUnauthorized = tg.ErrUnauthorized

// Deprecated: Use tg.ErrForbidden instead. Will be removed in v2.0.
var ErrForbidden = tg.ErrForbidden

// Deprecated: Use tg.ErrNotFound instead. Will be removed in v2.0.
var ErrNotFound = tg.ErrNotFound

// Deprecated: Use tg.ErrConflict instead. Will be removed in v2.0.
var ErrConflict = tg.ErrConflict

// Deprecated: Use tg.ErrTooManyRequests instead. Will be removed in v2.0.
var ErrTooManyRequests = tg.ErrTooManyRequests

// Deprecated: Use tg.ErrBotBlocked instead. Will be removed in v2.0.
var ErrBotBlocked = tg.ErrBotBlocked

// Deprecated: Use tg.ErrChatNotFound instead. Will be removed in v2.0.
var ErrChatNotFound = tg.ErrChatNotFound

// Deprecated: Use tg.ErrMessageNotFound instead. Will be removed in v2.0.
var ErrMessageNotFound = tg.ErrMessageNotFound

// Deprecated: Use tg.ErrCircuitOpen instead. Will be removed in v2.0.
var ErrCircuitOpen = tg.ErrCircuitOpen

// Deprecated: Use tg.ErrMaxRetries instead. Will be removed in v2.0.
var ErrMaxRetries = tg.ErrMaxRetries

// Deprecated: Use tg.ErrRateLimited instead. Will be removed in v2.0.
var ErrRateLimited = tg.ErrRateLimited
```

### Benefits

| Aspect | Result |
|--------|--------|
| Existing tests | ✅ `sender.ErrXxx` still works |
| New code | ✅ Can use canonical `tg.ErrXxx` |
| `errors.Is()` | ✅ Works with both (same underlying value) |
| IDE warnings | ✅ Deprecation notices guide migration |
| v2.0 cleanup | ✅ Easy removal of aliases |

---

## 3. Multipart Builder - Hybrid Explicit + Reflection

### Final Solution: Explicit File Handling + Structured Param Encoding

**Key Insight:** Files are explicit, params are structured, reflection only for field iteration.

```go
// sender/multipart.go
package sender

import (
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "reflect"
    "strconv"
    "strings"
)

// FilePart represents a file to be uploaded via multipart.
type FilePart struct {
    FieldName string    // e.g., "photo", "document", "thumbnail"
    FileName  string    // e.g., "photo.jpg"
    Reader    io.Reader // File content
}

// MultipartRequest represents a request with files and parameters.
type MultipartRequest struct {
    Files  []FilePart        // Explicit file parts
    Params map[string]string // String-encoded parameters
}

// MultipartEncoder encodes requests as multipart/form-data.
type MultipartEncoder struct {
    w *multipart.Writer
}

func NewMultipartEncoder(w io.Writer) *MultipartEncoder {
    return &MultipartEncoder{
        w: multipart.NewWriter(w),
    }
}

func (e *MultipartEncoder) ContentType() string {
    return e.w.FormDataContentType()
}

func (e *MultipartEncoder) Close() error {
    return e.w.Close()
}

// Encode writes the multipart request.
func (e *MultipartEncoder) Encode(req MultipartRequest) error {
    // 1. Write all file parts (explicit, type-safe)
    for _, file := range req.Files {
        if err := e.writeFile(file); err != nil {
            return fmt.Errorf("file %s: %w", file.FieldName, err)
        }
    }
    
    // 2. Write all parameter fields
    for name, value := range req.Params {
        if err := e.w.WriteField(name, value); err != nil {
            return fmt.Errorf("param %s: %w", name, err)
        }
    }
    
    return nil
}

func (e *MultipartEncoder) writeFile(file FilePart) error {
    part, err := e.w.CreateFormFile(file.FieldName, file.FileName)
    if err != nil {
        return fmt.Errorf("create form file: %w", err)
    }
    
    // Stream directly - no buffering
    _, err = io.Copy(part, file.Reader)
    return err
}

// BuildMultipartRequest creates a MultipartRequest from a typed request struct.
// Uses reflection for field iteration, but explicit handling for known types.
func BuildMultipartRequest(req any) (MultipartRequest, error) {
    result := MultipartRequest{
        Files:  make([]FilePart, 0),
        Params: make(map[string]string),
    }
    
    rv := reflect.ValueOf(req)
    if rv.Kind() == reflect.Ptr {
        rv = rv.Elem()
    }
    
    rt := rv.Type()
    attachIdx := 0
    
    for i := 0; i < rt.NumField(); i++ {
        field := rt.Field(i)
        value := rv.Field(i)
        
        // Skip unexported fields
        if !field.IsExported() {
            continue
        }
        
        // Skip zero values (omitempty behavior)
        if value.IsZero() {
            continue
        }
        
        // Get JSON field name
        fieldName := getJSONFieldName(field)
        if fieldName == "-" {
            continue
        }
        
        // Handle by type (explicit, fast path)
        switch v := value.Interface().(type) {
        case InputFile:
            if err := handleInputFile(&result, fieldName, v, &attachIdx); err != nil {
                return result, fmt.Errorf("field %s: %w", fieldName, err)
            }
            
        case *InputFile:
            if v != nil {
                if err := handleInputFile(&result, fieldName, *v, &attachIdx); err != nil {
                    return result, fmt.Errorf("field %s: %w", fieldName, err)
                }
            }
            
        case []InputFile:
            if err := handleInputFileSlice(&result, fieldName, v, &attachIdx); err != nil {
                return result, fmt.Errorf("field %s: %w", fieldName, err)
            }
            
        case string:
            result.Params[fieldName] = v
            
        case int, int64:
            result.Params[fieldName] = fmt.Sprint(v)
            
        case float64:
            result.Params[fieldName] = strconv.FormatFloat(v, 'f', -1, 64)
            
        case bool:
            result.Params[fieldName] = strconv.FormatBool(v)
            
        default:
            // Complex types (structs, slices, maps) -> JSON encode
            data, err := json.Marshal(v)
            if err != nil {
                return result, fmt.Errorf("field %s: JSON marshal: %w", fieldName, err)
            }
            result.Params[fieldName] = string(data)
        }
    }
    
    return result, nil
}

func handleInputFile(req *MultipartRequest, fieldName string, file InputFile, attachIdx *int) error {
    switch {
    case file.FileID != "":
        req.Params[fieldName] = file.FileID
        
    case file.URL != "":
        req.Params[fieldName] = file.URL
        
    case file.Reader != nil:
        // Generate attach:// reference
        attachName := fmt.Sprintf("file%d", *attachIdx)
        *attachIdx++
        
        req.Params[fieldName] = "attach://" + attachName
        req.Files = append(req.Files, FilePart{
            FieldName: attachName,
            FileName:  file.FileName,
            Reader:    file.Reader,
        })
        
    default:
        return fmt.Errorf("InputFile must have FileID, URL, or Reader set")
    }
    
    return nil
}

func handleInputFileSlice(req *MultipartRequest, fieldName string, files []InputFile, attachIdx *int) error {
    // For media groups, each file needs attach:// reference
    mediaItems := make([]map[string]any, 0, len(files))
    
    for i, file := range files {
        item := map[string]any{
            "type": file.MediaType,
        }
        
        switch {
        case file.FileID != "":
            item["media"] = file.FileID
            
        case file.URL != "":
            item["media"] = file.URL
            
        case file.Reader != nil:
            attachName := fmt.Sprintf("file%d", *attachIdx)
            *attachIdx++
            
            item["media"] = "attach://" + attachName
            req.Files = append(req.Files, FilePart{
                FieldName: attachName,
                FileName:  file.FileName,
                Reader:    file.Reader,
            })
            
        default:
            return fmt.Errorf("item %d: InputFile must have FileID, URL, or Reader set", i)
        }
        
        if file.Caption != "" {
            item["caption"] = file.Caption
        }
        if file.ParseMode != "" {
            item["parse_mode"] = file.ParseMode
        }
        
        mediaItems = append(mediaItems, item)
    }
    
    data, err := json.Marshal(mediaItems)
    if err != nil {
        return fmt.Errorf("JSON marshal media array: %w", err)
    }
    req.Params[fieldName] = string(data)
    
    return nil
}

func getJSONFieldName(field reflect.StructField) string {
    tag := field.Tag.Get("json")
    if tag == "" {
        return strings.ToLower(field.Name)
    }
    parts := strings.Split(tag, ",")
    return parts[0]
}
```

### Benefits

| Aspect | Solution |
|--------|----------|
| File handling | Explicit `FilePart` struct - type-safe |
| Error messages | Clear field names in all errors |
| Streaming | `io.Reader` → no buffering of large files |
| `attach://` | Correctly generated for media groups |
| Params | JSON for complex types, fmt for primitives |
| Reflection | Minimal - only field name iteration |

---

## 4. ChatID Type Safety - v2 Migration Strategy

### Final Solution: Full Break in v2 + Migration Helper

**Key Insight:** Clean break is better than prolonged dual-API.

```go
// tg/chat_id.go
package tg

import (
    "encoding/json"
    "fmt"
    "strconv"
    "strings"
)

// ChatID represents a Telegram chat identifier.
// Use ChatIDFromInt64 or ChatIDFromUsername to create.
type ChatID struct {
    id       int64
    username string
    isInt    bool
}

// Constructors

// ChatIDFromInt64 creates a ChatID from a numeric ID.
func ChatIDFromInt64(id int64) ChatID {
    return ChatID{id: id, isInt: true}
}

// ChatIDFromUsername creates a ChatID from a @username.
func ChatIDFromUsername(username string) ChatID {
    if !strings.HasPrefix(username, "@") {
        username = "@" + username
    }
    return ChatID{username: username, isInt: false}
}

// ParseChatID converts any supported type to ChatID.
// Supports: int64, int, string, ChatID.
// This is the migration helper for v1 → v2.
func ParseChatID(v any) (ChatID, error) {
    switch id := v.(type) {
    case ChatID:
        return id, nil
    case int64:
        return ChatIDFromInt64(id), nil
    case int:
        return ChatIDFromInt64(int64(id)), nil
    case string:
        // Try parsing as number first
        if n, err := strconv.ParseInt(id, 10, 64); err == nil {
            return ChatIDFromInt64(n), nil
        }
        // Treat as username
        return ChatIDFromUsername(id), nil
    default:
        return ChatID{}, fmt.Errorf("unsupported ChatID type: %T", v)
    }
}

// MustParseChatID is like ParseChatID but panics on error.
func MustParseChatID(v any) ChatID {
    cid, err := ParseChatID(v)
    if err != nil {
        panic(err)
    }
    return cid
}

// Methods

// IsNumeric returns true if this is a numeric chat ID.
func (c ChatID) IsNumeric() bool {
    return c.isInt
}

// Int64 returns the numeric ID (0 if username-based).
func (c ChatID) Int64() int64 {
    return c.id
}

// Username returns the username (empty if numeric).
func (c ChatID) Username() string {
    return c.username
}

// String returns the string representation.
func (c ChatID) String() string {
    if c.isInt {
        return strconv.FormatInt(c.id, 10)
    }
    return c.username
}

// MarshalJSON implements json.Marshaler.
func (c ChatID) MarshalJSON() ([]byte, error) {
    if c.isInt {
        return json.Marshal(c.id)
    }
    return json.Marshal(c.username)
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *ChatID) UnmarshalJSON(data []byte) error {
    // Try number first
    var n int64
    if err := json.Unmarshal(data, &n); err == nil {
        c.id = n
        c.isInt = true
        return nil
    }
    
    // Try string
    var s string
    if err := json.Unmarshal(data, &s); err == nil {
        c.username = s
        c.isInt = false
        return nil
    }
    
    return fmt.Errorf("ChatID must be number or string")
}
```

### Migration Guide (for v2 release notes)

```markdown
## Breaking Changes in v2.0

### ChatID Type Change

The `ChatID` parameter type has changed from `any` to `tg.ChatID` for type safety.

**Before (v1.x):**
```go
bot.SendMessage(ctx, 123456789, "Hello")
bot.SendMessage(ctx, "@username", "Hello")
```

**After (v2.0):**
```go
bot.SendMessage(ctx, tg.ChatIDFromInt64(123456789), "Hello")
bot.SendMessage(ctx, tg.ChatIDFromUsername("@username"), "Hello")

// Migration helper for existing code:
chatID, _ := tg.ParseChatID(oldChatIDValue)
bot.SendMessage(ctx, chatID, "Hello")
```

**Quick migration script:**
```bash
# Find all SendMessage calls and update
sed -i 's/SendMessage(ctx, \([0-9]*\),/SendMessage(ctx, tg.ChatIDFromInt64(\1),/g' *.go
```
```

---

## 5. retry_after Parsing - Both Sources

### Final Solution: JSON Primary + HTTP Header Fallback

```go
// sender/response.go
package sender

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"
    
    "github.com/prliv-com/galigo/tg"
)

// parseAPIError creates an APIError from HTTP response and body.
func parseAPIError(resp *http.Response, body []byte) *tg.APIError {
    // Parse JSON envelope
    var envelope struct {
        OK          bool   `json:"ok"`
        ErrorCode   int    `json:"error_code"`
        Description string `json:"description"`
        Parameters  *struct {
            RetryAfter      int   `json:"retry_after,omitempty"`
            MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
        } `json:"parameters,omitempty"`
    }
    
    if err := json.Unmarshal(body, &envelope); err != nil {
        // Non-JSON response (e.g., proxy error)
        return &tg.APIError{
            Code:        resp.StatusCode,
            Description: string(body),
        }
    }
    
    apiErr := &tg.APIError{
        Code:        envelope.ErrorCode,
        Description: envelope.Description,
    }
    
    // Parse retry_after from JSON body (primary source)
    if envelope.Parameters != nil {
        if envelope.Parameters.RetryAfter > 0 {
            apiErr.RetryAfter = time.Duration(envelope.Parameters.RetryAfter) * time.Second
        }
        if envelope.Parameters.MigrateToChatID != 0 {
            apiErr.Parameters = &tg.ResponseParameters{
                MigrateToChatID: envelope.Parameters.MigrateToChatID,
            }
        }
    }
    
    // Fallback: HTTP Retry-After header (if JSON didn't have it)
    if apiErr.RetryAfter == 0 {
        if retryHeader := resp.Header.Get("Retry-After"); retryHeader != "" {
            if seconds, err := strconv.Atoi(retryHeader); err == nil {
                apiErr.RetryAfter = time.Duration(seconds) * time.Second
            }
        }
    }
    
    return apiErr
}
```

---

## 6. Coverage Target - Explicit in CI

### Final Solution: 80% Threshold Enforced

```makefile
# Makefile
COVERAGE_THRESHOLD := 80
COVERAGE_SENDER := 85
COVERAGE_RECEIVER := 85
COVERAGE_TG := 90

.PHONY: test-coverage-check

test-coverage:
	go test -v -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tee coverage.txt

test-coverage-check: test-coverage
	@echo "Checking coverage thresholds..."
	@TOTAL=$$(tail -1 coverage.txt | awk '{print $$3}' | tr -d '%'); \
	if [ $$(echo "$$TOTAL < $(COVERAGE_THRESHOLD)" | bc) -eq 1 ]; then \
		echo "❌ Total coverage $$TOTAL% below $(COVERAGE_THRESHOLD)%"; \
		exit 1; \
	fi; \
	echo "✅ Total coverage $$TOTAL% meets threshold"

ci: lint test-race test-coverage-check
```

---

## 7. Large File Handling - Streaming Only

### Final Solution: Document Limits + Streaming

```go
// sender/upload.go
package sender

import "io"

const (
    // MaxUploadSize is the maximum file size for Bot API uploads (50MB).
    // For larger files, use external storage and send URL.
    MaxUploadSize = 50 * 1024 * 1024
    
    // MaxPhotoSize is the maximum photo file size (10MB).
    MaxPhotoSize = 10 * 1024 * 1024
)

// InputFile represents a file to upload or reference.
type InputFile struct {
    // FileID references an existing file on Telegram servers.
    FileID string
    
    // URL references a file by HTTP URL (Telegram will download).
    URL string
    
    // Reader provides file content for upload.
    // ⚠️ Must be streamable - content is NOT buffered.
    Reader io.Reader
    
    // FileName is required when Reader is set.
    FileName string
    
    // MediaType is used for media groups (e.g., "photo", "video").
    MediaType string
    
    // Caption for media items.
    Caption string
    
    // ParseMode for caption (HTML, Markdown, MarkdownV2).
    ParseMode string
}

// FromReader creates an InputFile from an io.Reader.
// The reader is streamed directly - not buffered in memory.
func FromReader(r io.Reader, filename string) InputFile {
    return InputFile{
        Reader:   r,
        FileName: filename,
    }
}

// FromFileID creates an InputFile referencing an existing Telegram file.
func FromFileID(fileID string) InputFile {
    return InputFile{FileID: fileID}
}

// FromURL creates an InputFile from a URL (Telegram will download).
func FromURL(url string) InputFile {
    return InputFile{URL: url}
}
```

```go
// Documentation in doc.go
/*
File Upload Limits:

The Telegram Bot API has the following file size limits:
- Photos: 10 MB
- Other files: 50 MB

For files larger than 50 MB, you have two options:
1. Upload to external storage (S3, etc.) and send the URL
2. Use Telegram's TDLib for larger uploads (not supported by this SDK)

Files are streamed directly from io.Reader without buffering,
so memory usage stays constant regardless of file size.

Example:
    file, _ := os.Open("large_video.mp4")
    defer file.Close()
    
    // File is streamed, not loaded into memory
    bot.SendDocument(ctx, chatID, sender.FromReader(file, "video.mp4"))
*/
```

---

## Updated PR Order

| PR | Focus | Key Adjustments |
|----|-------|-----------------|
| **PR0** | CI + Dependencies | Add 80% coverage gate |
| **PR1** | Response + Error Model | Type aliases for backward compat |
| **PR2** | JSON Executor | Verify retry_after dual-source parsing |
| **PR3** | InputFile + Multipart | Hybrid explicit + streaming |
| **PR4-6** | Methods | As planned |
| **PR7** | sendMessageDraft (9.3) | As planned |
| **PR8** | Modern params | As planned |
| **PR9** | Receiver Delivery Policy | Configurable policy + timeout |
| **PR10** | ChatID + v2 Release | Full break + ParseChatID helper |

---

## Summary: Key Decisions

| Concern | Final Decision | Rationale |
|---------|----------------|-----------|
| Blocking send | Policy enum + timeout + callback | Explicit, safe, observable |
| Error aliases | Keep both with deprecation | No breaking change in v1 |
| Multipart | Explicit files + JSON params | Type-safe, clear errors |
| ChatID | Full v2 break + ParseChatID | Clean API, easy migration |
| retry_after | JSON primary + header fallback | Matches Telegram docs |
| Coverage | 80% overall, 85%+ critical | Enforced in CI |
| Large files | Streaming only, document limits | Memory-safe |