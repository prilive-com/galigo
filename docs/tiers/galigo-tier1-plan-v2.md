# galigo Tier 1 Implementation Plan

## Detailed Technical Specification

**Version:** 1.0  
**Target:** galigo v2.0.0  
**Telegram Bot API:** 9.3  
**Go Version:** 1.25  
**Estimated Effort:** 3-4 weeks

---

## Table of Contents

1. [Overview](#1-overview)
2. [Implementation Order](#2-implementation-order)
3. [Phase 0: Bug Fixes (Prerequisites)](#3-phase-0-bug-fixes)
4. [Phase 1: Architecture Foundation](#4-phase-1-architecture-foundation)
5. [Phase 2: Core Methods](#5-phase-2-core-methods)
6. [Phase 3: Media Methods](#6-phase-3-media-methods)
7. [Phase 4: Utility Methods](#7-phase-4-utility-methods)
8. [Phase 5: Bot API 9.x Methods](#8-phase-5-bot-api-9x-methods)
9. [Type Definitions](#9-type-definitions)
10. [Test Strategy](#10-test-strategy)

---

## 1. Overview

### 1.1 Scope

Tier 1 covers the **essential methods** needed for 90% of bot use cases:

| Category | Methods | Count |
|----------|---------|-------|
| Bot Identity | getMe, logOut, close | 3 |
| Core Media | sendDocument, sendVideo, sendAudio, sendVoice, sendAnimation, sendVideoNote, sendSticker, sendMediaGroup | 8 |
| Utilities | getFile, sendChatAction, getUserProfilePhotos | 3 |
| Message Editing | editMessageMedia, deleteMessages | 2 |
| Location/Contact | sendLocation, sendVenue, sendContact, sendPoll, sendDice | 5 |
| Live Location | editMessageLiveLocation, stopMessageLiveLocation, stopPoll | 3 |
| Bulk Operations | forwardMessages, copyMessages, setMessageReaction | 3 |
| Bot API 9.x | sendMessageDraft | 1 |
| **TOTAL** | | **28** |

### 1.2 Dependencies

```
Phase 0 (Bug Fixes)
    │
    ▼
Phase 1 (Architecture)
    ├── A.1 Generic Executor
    ├── A.2 InputFile Abstraction  
    ├── A.3 Response/Error Model
    └── A.4 int64 Safety
         │
         ▼
Phase 2-5 (Methods) ─── Can be parallelized after Phase 1
```

---

## 2. Implementation Order

### 2.1 Critical Path

```
Week 1:
├── Day 1-2: Phase 0 (P0 Bug Fixes)
├── Day 3-4: A.1 Generic Executor
└── Day 5:   A.3 Response/Error Model

Week 2:
├── Day 1-2: A.2 InputFile Abstraction
├── Day 3:   A.4 int64 Safety
├── Day 4:   getMe, logOut, close
└── Day 5:   getFile, sendChatAction

Week 3:
├── Day 1-2: sendDocument, sendVideo, sendAudio
├── Day 3:   sendVoice, sendAnimation, sendVideoNote
├── Day 4:   sendSticker, sendMediaGroup
└── Day 5:   editMessageMedia, deleteMessages

Week 4:
├── Day 1:   sendLocation, sendVenue, sendContact
├── Day 2:   sendPoll, sendDice, stopPoll
├── Day 3:   editMessageLiveLocation, stopMessageLiveLocation
├── Day 4:   forwardMessages, copyMessages, setMessageReaction
└── Day 5:   sendMessageDraft, getUserProfilePhotos, testing
```

---

## 3. Phase 0: Bug Fixes (Prerequisites)

**MUST complete before architecture work.**

### 3.1 P0.1 - Fix Update Loss Bug

**File:** `receiver/polling.go:288-299`

```go
// BEFORE (BUGGY):
for _, update := range updates {
    if update.UpdateID >= c.offset {
        c.offset = update.UpdateID + 1  // Advances BEFORE send
    }
    select {
    case c.updates <- update:
    default:
        c.logger.Warn("updates channel full")  // LOST FOREVER
    }
}

// AFTER (FIXED):
for _, update := range updates {
    select {
    case c.updates <- update:
        // Only advance offset AFTER successful delivery
        if update.UpdateID >= c.offset {
            c.offset = update.UpdateID + 1
        }
        c.logger.Debug("update sent", "update_id", update.UpdateID)
    case <-ctx.Done():
        // Don't advance offset - updates will be redelivered on restart
        c.logger.Info("context cancelled, stopping update processing")
        return
    }
}
```

**Test:**
```go
func TestPollingClient_BackpressureNoLoss(t *testing.T) {
    // Create client with buffer size 1
    client := NewPollingClient(token, WithUpdateBuffer(1))
    
    // Don't consume updates (simulate backpressure)
    // Send 3 updates from mock server
    // Verify offset only advances for delivered updates
    // Verify updates are redelivered on next poll
}
```

---

### 3.2 P0.2 - Fix URL Encoding for allowed_updates

**File:** `receiver/polling.go:310-324`

```go
// BEFORE (BUGGY):
if len(c.allowedUpdates) > 0 {
    encoded, _ := json.Marshal(c.allowedUpdates)
    url += "&allowed_updates=" + string(encoded)  // Raw JSON!
}

// AFTER (FIXED):
import "net/url"

func (c *PollingClient) buildGetUpdatesURL() string {
    params := url.Values{}
    params.Set("timeout", strconv.Itoa(c.timeout))
    params.Set("limit", strconv.Itoa(c.limit))
    params.Set("offset", strconv.FormatInt(c.offset, 10))
    
    if len(c.allowedUpdates) > 0 {
        encoded, err := json.Marshal(c.allowedUpdates)
        if err == nil {
            params.Set("allowed_updates", string(encoded))
        }
    }
    
    return fmt.Sprintf("%s%s/getUpdates?%s",
        c.baseURL,
        c.token.Value(),
        params.Encode(),  // Properly URL-encoded
    )
}
```

---

### 3.3 P0.3 - Fix retry_after Parsing

**File:** `sender/client.go:460-473`

```go
// BEFORE (BUGGY):
retryAfter := resp.Header.Get("Retry-After")  // Wrong! It's in JSON body

// AFTER (FIXED):
// First, update apiResponse struct:
type apiResponse struct {
    OK          bool                `json:"ok"`
    Result      json.RawMessage     `json:"result,omitempty"`
    ErrorCode   int                 `json:"error_code,omitempty"`
    Description string              `json:"description,omitempty"`
    Parameters  *ResponseParameters `json:"parameters,omitempty"`
}

type ResponseParameters struct {
    RetryAfter      int   `json:"retry_after,omitempty"`
    MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
}

// Then in doRequest:
if !apiResp.OK {
    var retryAfter time.Duration
    if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
        retryAfter = time.Duration(apiResp.Parameters.RetryAfter) * time.Second
    }
    
    if retryAfter > 0 {
        return nil, NewAPIErrorWithRetry(method, apiResp.ErrorCode, apiResp.Description, retryAfter)
    }
    return nil, NewAPIError(method, apiResp.ErrorCode, apiResp.Description)
}
```

**Test:**
```go
func TestDoRequest_ParsesRetryAfterFromJSON(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusTooManyRequests)
        json.NewEncoder(w).Encode(map[string]any{
            "ok":          false,
            "error_code":  429,
            "description": "Too Many Requests: retry after 35",
            "parameters": map[string]any{
                "retry_after": 35,
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(server.URL)
    _, err := client.SendMessage(ctx, SendMessageRequest{ChatID: 123, Text: "test"})
    
    var apiErr *APIError
    require.ErrorAs(t, err, &apiErr)
    assert.Equal(t, 429, apiErr.Code)
    assert.Equal(t, 35*time.Second, apiErr.RetryAfter)
}
```

---

### 3.4 P0.4 - Fix Response Size Boundary

**File:** `sender/client.go:456-458`

```go
// BEFORE (BUGGY):
if len(body) == maxResponseSize {
    return nil, ErrResponseTooLarge  // False positive at exact boundary
}

// AFTER (FIXED):
const maxResponseSize = 10 << 20 // 10MB

limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
body, err := io.ReadAll(limitedReader)
if err != nil {
    return nil, fmt.Errorf("failed to read response: %w", err)
}

if int64(len(body)) > maxResponseSize {
    return nil, ErrResponseTooLarge
}
```

---

## 4. Phase 1: Architecture Foundation

### 4.1 A.1 - Generic Request Executor

**File:** `sender/executor.go` (new file)

```go
package sender

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    
    "github.com/prilive-com/galigo/tg"
)

// RequestMode determines how the request body is formatted
type RequestMode int

const (
    RequestModeJSON RequestMode = iota
    RequestModeMultipart
)

// ExecuteOptions configures request execution
type ExecuteOptions struct {
    Mode          RequestMode
    SkipRateLimit bool
}

// Execute sends a request to the Telegram Bot API
// This is the core method that all other methods should use internally
func (c *Client) Execute(ctx context.Context, method string, params any, result any) error {
    return c.ExecuteWithOptions(ctx, method, params, result, ExecuteOptions{Mode: RequestModeJSON})
}

// ExecuteWithOptions sends a request with custom options
func (c *Client) ExecuteWithOptions(ctx context.Context, method string, params any, result any, opts ExecuteOptions) error {
    // Apply rate limiting unless skipped
    if !opts.SkipRateLimit {
        if err := c.globalLimiter.Wait(ctx); err != nil {
            return fmt.Errorf("rate limit wait: %w", err)
        }
    }
    
    // Execute through circuit breaker
    apiResp, err := c.breaker.Execute(func() (*apiResponse, error) {
        switch opts.Mode {
        case RequestModeMultipart:
            return c.doMultipartRequest(ctx, method, params)
        default:
            return c.doJSONRequest(ctx, method, params)
        }
    })
    
    if err != nil {
        return err
    }
    
    // Unmarshal result if provided
    if result != nil && len(apiResp.Result) > 0 {
        if err := json.Unmarshal(apiResp.Result, result); err != nil {
            return fmt.Errorf("failed to unmarshal result: %w", err)
        }
    }
    
    return nil
}

// doJSONRequest sends a JSON-encoded request
func (c *Client) doJSONRequest(ctx context.Context, method string, params any) (*apiResponse, error) {
    var body io.Reader
    
    if params != nil {
        jsonData, err := json.Marshal(params)
        if err != nil {
            return nil, fmt.Errorf("failed to marshal params: %w", err)
        }
        body = bytes.NewReader(jsonData)
    }
    
    return c.sendRequest(ctx, method, "application/json", body)
}

// doMultipartRequest sends a multipart/form-data request
func (c *Client) doMultipartRequest(ctx context.Context, method string, params any) (*apiResponse, error) {
    // Convert params to multipart form
    body, contentType, err := buildMultipartBody(params)
    if err != nil {
        return nil, fmt.Errorf("failed to build multipart body: %w", err)
    }
    
    return c.sendRequest(ctx, method, contentType, body)
}

// sendRequest is the low-level HTTP request sender
func (c *Client) sendRequest(ctx context.Context, method, contentType string, body io.Reader) (*apiResponse, error) {
    url := fmt.Sprintf("%s/bot%s/%s", c.config.BaseURL, c.config.Token.Value(), method)
    
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Accept", "application/json")
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    return c.parseResponse(method, resp)
}

// parseResponse reads and parses the API response
func (c *Client) parseResponse(method string, resp *http.Response) (*apiResponse, error) {
    // Read with size limit (+1 to detect overflow)
    limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
    body, err := io.ReadAll(limitedReader)
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }
    
    if int64(len(body)) > maxResponseSize {
        return nil, ErrResponseTooLarge
    }
    
    var apiResp apiResponse
    if err := json.Unmarshal(body, &apiResp); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }
    
    if !apiResp.OK {
        return nil, c.buildAPIError(method, &apiResp)
    }
    
    return &apiResp, nil
}

// buildAPIError constructs an APIError from the response
func (c *Client) buildAPIError(method string, resp *apiResponse) error {
    var retryAfter time.Duration
    if resp.Parameters != nil && resp.Parameters.RetryAfter > 0 {
        retryAfter = time.Duration(resp.Parameters.RetryAfter) * time.Second
    }
    
    if retryAfter > 0 {
        return NewAPIErrorWithRetry(method, resp.ErrorCode, resp.Description, retryAfter)
    }
    
    return NewAPIError(method, resp.ErrorCode, resp.Description)
}
```

**Usage Example:**
```go
// Simple method using Execute
func (c *Client) GetMe(ctx context.Context) (*tg.User, error) {
    var user tg.User
    if err := c.Execute(ctx, "getMe", nil, &user); err != nil {
        return nil, err
    }
    return &user, nil
}

// Method with parameters
func (c *Client) SendChatAction(ctx context.Context, chatID tg.ChatID, action ChatAction) error {
    params := map[string]any{
        "chat_id": chatID,
        "action":  action,
    }
    return c.Execute(ctx, "sendChatAction", params, nil)
}
```

---

### 4.2 A.2 - InputFile Abstraction

**File:** `tg/inputfile.go` (new file)

```go
package tg

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"
)

// InputFile represents a file to send to Telegram
// It can be a file_id, URL, or file upload
type InputFile interface {
    inputFile() // marker method to ensure type safety
}

// InputFileID represents a file already on Telegram servers
type InputFileID string

func (f InputFileID) inputFile() {}

// MarshalJSON implements json.Marshaler
func (f InputFileID) MarshalJSON() ([]byte, error) {
    return json.Marshal(string(f))
}

// InputFileURL represents a file URL for Telegram to download
type InputFileURL string

func (f InputFileURL) inputFile() {}

// MarshalJSON implements json.Marshaler
func (f InputFileURL) MarshalJSON() ([]byte, error) {
    return json.Marshal(string(f))
}

// InputFileUpload represents a file to upload
type InputFileUpload struct {
    // Filename is the name to use for the uploaded file
    Filename string
    
    // Reader provides the file content
    // The caller is responsible for closing if it's a closer
    Reader io.Reader
    
    // AttachName is used internally for multipart references (attach://name)
    AttachName string
}

func (f InputFileUpload) inputFile() {}

// MarshalJSON returns the attach:// reference for multipart forms
func (f InputFileUpload) MarshalJSON() ([]byte, error) {
    if f.AttachName != "" {
        return json.Marshal("attach://" + f.AttachName)
    }
    return nil, fmt.Errorf("InputFileUpload requires AttachName for JSON serialization")
}

// InputFileFromPath creates an InputFile from a local file path
func InputFileFromPath(path string) (InputFileUpload, error) {
    file, err := os.Open(path)
    if err != nil {
        return InputFileUpload{}, fmt.Errorf("failed to open file: %w", err)
    }
    
    // Extract filename from path
    parts := strings.Split(path, "/")
    filename := parts[len(parts)-1]
    if filename == "" {
        filename = "file"
    }
    
    return InputFileUpload{
        Filename: filename,
        Reader:   file,
    }, nil
}

// InputFileFromReader creates an InputFile from an io.Reader
func InputFileFromReader(filename string, r io.Reader) InputFileUpload {
    return InputFileUpload{
        Filename: filename,
        Reader:   r,
    }
}

// InputFileFromBytes creates an InputFile from a byte slice
func InputFileFromBytes(filename string, data []byte) InputFileUpload {
    return InputFileUpload{
        Filename: filename,
        Reader:   bytes.NewReader(data),
    }
}

// NewInputFile creates an InputFile from various types
// Supported types:
//   - string starting with "http://" or "https://" → InputFileURL
//   - other string → InputFileID (assumed to be file_id)
//   - io.Reader → InputFileUpload
//   - InputFile → returned as-is
func NewInputFile(v any) InputFile {
    switch x := v.(type) {
    case InputFile:
        return x
    case string:
        if strings.HasPrefix(x, "http://") || strings.HasPrefix(x, "https://") {
            return InputFileURL(x)
        }
        return InputFileID(x)
    case io.Reader:
        return InputFileUpload{Filename: "file", Reader: x}
    default:
        // Return as file_id string
        return InputFileID(fmt.Sprint(v))
    }
}

// IsUpload returns true if this InputFile requires multipart upload
func IsUpload(f InputFile) bool {
    _, ok := f.(InputFileUpload)
    return ok
}
```

---

### 4.3 A.2 (continued) - Multipart Builder

**File:** `sender/multipart.go` (new file)

```go
package sender

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "reflect"
    "strings"
    
    "github.com/prilive-com/galigo/tg"
)

// buildMultipartBody creates a multipart/form-data request body
func buildMultipartBody(params any) (io.Reader, string, error) {
    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)
    
    // Track file uploads for attach:// references
    attachCounter := 0
    
    // Use reflection to iterate struct fields
    v := reflect.ValueOf(params)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    
    if v.Kind() != reflect.Struct {
        return nil, "", fmt.Errorf("params must be a struct, got %T", params)
    }
    
    t := v.Type()
    
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)
        
        // Get JSON tag for field name
        jsonTag := fieldType.Tag.Get("json")
        if jsonTag == "" || jsonTag == "-" {
            continue
        }
        
        // Parse json tag (handle ",omitempty")
        tagParts := strings.Split(jsonTag, ",")
        fieldName := tagParts[0]
        omitempty := len(tagParts) > 1 && tagParts[1] == "omitempty"
        
        // Skip zero values if omitempty
        if omitempty && isZeroValue(field) {
            continue
        }
        
        // Handle InputFile specially
        if inputFile, ok := field.Interface().(tg.InputFile); ok {
            if upload, ok := inputFile.(tg.InputFileUpload); ok {
                // File upload - add as multipart file
                attachName := fmt.Sprintf("file%d", attachCounter)
                attachCounter++
                
                part, err := writer.CreateFormFile(fieldName, upload.Filename)
                if err != nil {
                    return nil, "", fmt.Errorf("failed to create form file: %w", err)
                }
                
                if _, err := io.Copy(part, upload.Reader); err != nil {
                    return nil, "", fmt.Errorf("failed to write file: %w", err)
                }
                
                // Close reader if it's a Closer
                if closer, ok := upload.Reader.(io.Closer); ok {
                    closer.Close()
                }
            } else {
                // file_id or URL - add as form field
                if err := writeFormField(writer, fieldName, inputFile); err != nil {
                    return nil, "", err
                }
            }
            continue
        }
        
        // Handle InputMedia array (for sendMediaGroup)
        if isInputMediaSlice(field) {
            if err := writeInputMediaArray(writer, fieldName, field, &attachCounter); err != nil {
                return nil, "", err
            }
            continue
        }
        
        // Regular field - add as form field
        if err := writeFormField(writer, fieldName, field.Interface()); err != nil {
            return nil, "", err
        }
    }
    
    if err := writer.Close(); err != nil {
        return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
    }
    
    return &buf, writer.FormDataContentType(), nil
}

func writeFormField(w *multipart.Writer, name string, value any) error {
    var strValue string
    
    switch v := value.(type) {
    case string:
        strValue = v
    case tg.InputFileID:
        strValue = string(v)
    case tg.InputFileURL:
        strValue = string(v)
    case int, int64, float64, bool:
        strValue = fmt.Sprint(v)
    default:
        // JSON encode complex types
        jsonBytes, err := json.Marshal(v)
        if err != nil {
            return fmt.Errorf("failed to marshal field %s: %w", name, err)
        }
        strValue = string(jsonBytes)
    }
    
    return w.WriteField(name, strValue)
}

func writeInputMediaArray(w *multipart.Writer, name string, field reflect.Value, attachCounter *int) error {
    // Process each InputMedia item, extracting file uploads
    mediaItems := make([]map[string]any, field.Len())
    
    for i := 0; i < field.Len(); i++ {
        item := field.Index(i)
        mediaMap := make(map[string]any)
        
        // Extract fields from InputMedia struct
        itemType := item.Type()
        for j := 0; j < item.NumField(); j++ {
            itemField := item.Field(j)
            itemFieldType := itemType.Field(j)
            
            jsonTag := itemFieldType.Tag.Get("json")
            if jsonTag == "" || jsonTag == "-" {
                continue
            }
            
            tagParts := strings.Split(jsonTag, ",")
            fieldName := tagParts[0]
            
            // Handle media field (the actual file)
            if fieldName == "media" {
                if inputFile, ok := itemField.Interface().(tg.InputFile); ok {
                    if upload, ok := inputFile.(tg.InputFileUpload); ok {
                        // Upload file and use attach:// reference
                        attachName := fmt.Sprintf("file%d", *attachCounter)
                        *attachCounter++
                        
                        part, err := w.CreateFormFile(attachName, upload.Filename)
                        if err != nil {
                            return err
                        }
                        if _, err := io.Copy(part, upload.Reader); err != nil {
                            return err
                        }
                        if closer, ok := upload.Reader.(io.Closer); ok {
                            closer.Close()
                        }
                        
                        mediaMap[fieldName] = "attach://" + attachName
                        continue
                    }
                }
            }
            
            // Regular field
            if !isZeroValue(itemField) {
                mediaMap[fieldName] = itemField.Interface()
            }
        }
        
        mediaItems[i] = mediaMap
    }
    
    // Write JSON array as form field
    jsonBytes, err := json.Marshal(mediaItems)
    if err != nil {
        return err
    }
    
    return w.WriteField(name, string(jsonBytes))
}

func isZeroValue(v reflect.Value) bool {
    return v.IsZero()
}

func isInputMediaSlice(v reflect.Value) bool {
    if v.Kind() != reflect.Slice {
        return false
    }
    elemType := v.Type().Elem()
    return strings.HasPrefix(elemType.Name(), "InputMedia")
}
```

---

### 4.4 A.3 - Unified Error Model

**File:** `tg/errors.go` (update existing)

```go
package tg

import (
    "errors"
    "fmt"
    "time"
)

// Sentinel errors
var (
    ErrInvalidToken      = errors.New("galigo: invalid token format")
    ErrUnauthorized      = errors.New("galigo: unauthorized (invalid token)")
    ErrForbidden         = errors.New("galigo: forbidden")
    ErrNotFound          = errors.New("galigo: not found")
    ErrConflict          = errors.New("galigo: conflict (webhook/polling)")
    ErrTooManyRequests   = errors.New("galigo: too many requests")
    ErrBadRequest        = errors.New("galigo: bad request")
    ErrResponseTooLarge  = errors.New("galigo: response too large")
)

// APIError represents an error returned by the Telegram Bot API
type APIError struct {
    // Method is the API method that failed
    Method string
    
    // Code is the Telegram error code
    Code int
    
    // Description is the human-readable error message
    Description string
    
    // RetryAfter is set for 429 errors (rate limiting)
    RetryAfter time.Duration
    
    // MigrateToChatID is set when a group migrates to supergroup
    MigrateToChatID int64
}

func (e *APIError) Error() string {
    if e.RetryAfter > 0 {
        return fmt.Sprintf("galigo: %s failed: [%d] %s (retry after %v)",
            e.Method, e.Code, e.Description, e.RetryAfter)
    }
    return fmt.Sprintf("galigo: %s failed: [%d] %s", e.Method, e.Code, e.Description)
}

// Is implements errors.Is for APIError
func (e *APIError) Is(target error) bool {
    switch e.Code {
    case 401:
        return target == ErrUnauthorized
    case 403:
        return target == ErrForbidden
    case 404:
        return target == ErrNotFound
    case 409:
        return target == ErrConflict
    case 429:
        return target == ErrTooManyRequests
    case 400:
        return target == ErrBadRequest
    }
    return false
}

// Unwrap returns the underlying sentinel error
func (e *APIError) Unwrap() error {
    switch e.Code {
    case 401:
        return ErrUnauthorized
    case 403:
        return ErrForbidden
    case 404:
        return ErrNotFound
    case 409:
        return ErrConflict
    case 429:
        return ErrTooManyRequests
    case 400:
        return ErrBadRequest
    }
    return nil
}

// NewAPIError creates a new APIError
func NewAPIError(method string, code int, description string) *APIError {
    return &APIError{
        Method:      method,
        Code:        code,
        Description: description,
    }
}

// NewAPIErrorWithRetry creates a new APIError with retry information
func NewAPIErrorWithRetry(method string, code int, description string, retryAfter time.Duration) *APIError {
    return &APIError{
        Method:      method,
        Code:        code,
        Description: description,
        RetryAfter:  retryAfter,
    }
}

// NewAPIErrorWithMigration creates a new APIError for group migration
func NewAPIErrorWithMigration(method string, code int, description string, migrateTo int64) *APIError {
    return &APIError{
        Method:          method,
        Code:            code,
        Description:     description,
        MigrateToChatID: migrateTo,
    }
}

// ValidationError represents a client-side validation error
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("galigo: validation error on %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
    return &ValidationError{Field: field, Message: message}
}
```

**Usage:**
```go
// Check for specific errors
_, err := client.SendMessage(ctx, req)
if errors.Is(err, tg.ErrTooManyRequests) {
    var apiErr *tg.APIError
    if errors.As(err, &apiErr) {
        time.Sleep(apiErr.RetryAfter)
        // Retry...
    }
}

if errors.Is(err, tg.ErrUnauthorized) {
    log.Fatal("Invalid bot token")
}
```

---

## 5. Phase 2: Core Methods

### 5.1 getMe

**File:** `sender/methods_bot.go` (new file)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// GetMe returns basic information about the bot
// Use this method to test your bot's auth token.
// 
// Telegram docs: https://core.telegram.org/bots/api#getme
func (c *Client) GetMe(ctx context.Context) (*tg.User, error) {
    var user tg.User
    if err := c.Execute(ctx, "getMe", nil, &user); err != nil {
        return nil, err
    }
    return &user, nil
}

// LogOut logs out from the cloud Bot API server
// You must log out the bot before moving it from one local server to another.
// Returns True on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#logout
func (c *Client) LogOut(ctx context.Context) error {
    return c.Execute(ctx, "logOut", nil, nil)
}

// Close closes the bot instance before moving it from one local server to another
// Use this method to close the bot instance before moving it from one local server
// to another. You need to delete the webhook before calling this method.
// Returns True on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#close
func (c *Client) Close(ctx context.Context) error {
    return c.Execute(ctx, "close", nil, nil)
}
```

**Facade in bot.go:**
```go
// GetMe returns basic information about the bot
func (b *Bot) GetMe(ctx context.Context) (*tg.User, error) {
    return b.sender.GetMe(ctx)
}
```

**Test:**
```go
func TestClient_GetMe(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "/bot123:ABC/getMe", r.URL.Path)
        assert.Equal(t, http.MethodPost, r.Method)
        
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": map[string]any{
                "id":         int64(123456789),
                "is_bot":     true,
                "first_name": "Test Bot",
                "username":   "test_bot",
                "can_join_groups": true,
                "can_read_all_group_messages": false,
                "supports_inline_queries": true,
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "123:ABC")
    user, err := client.GetMe(context.Background())
    
    require.NoError(t, err)
    assert.Equal(t, int64(123456789), user.ID)
    assert.True(t, user.IsBot)
    assert.Equal(t, "Test Bot", user.FirstName)
    assert.Equal(t, "test_bot", user.Username)
}

func TestClient_GetMe_InvalidToken(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
        json.NewEncoder(w).Encode(map[string]any{
            "ok":          false,
            "error_code":  401,
            "description": "Unauthorized",
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "invalid")
    _, err := client.GetMe(context.Background())
    
    require.Error(t, err)
    assert.True(t, errors.Is(err, tg.ErrUnauthorized))
}
```

---

### 5.2 getFile

**File:** `sender/methods_files.go` (new file)

```go
package sender

import (
    "context"
    "fmt"
    "io"
    "net/http"
    
    "github.com/prilive-com/galigo/tg"
)

// GetFileRequest contains parameters for getFile
type GetFileRequest struct {
    FileID string `json:"file_id"`
}

// GetFile retrieves basic info about a file and prepares it for download
// The file can then be downloaded via the link:
// https://api.telegram.org/file/bot<token>/<file_path>
// 
// Note: The link is valid for at least 1 hour. The maximum file size to 
// download is 20 MB.
// 
// Telegram docs: https://core.telegram.org/bots/api#getfile
func (c *Client) GetFile(ctx context.Context, fileID string) (*tg.File, error) {
    req := GetFileRequest{FileID: fileID}
    
    var file tg.File
    if err := c.Execute(ctx, "getFile", req, &file); err != nil {
        return nil, err
    }
    return &file, nil
}

// FileDownloadURL returns the full URL to download a file
// Use this after calling GetFile to construct the download URL
func (c *Client) FileDownloadURL(filePath string) string {
    return fmt.Sprintf("%s/file/bot%s/%s", 
        c.config.BaseURL, 
        c.config.Token.Value(), 
        filePath,
    )
}

// DownloadFile downloads a file directly to the provided writer
// This is a convenience method that combines GetFile and downloading
func (c *Client) DownloadFile(ctx context.Context, fileID string, w io.Writer) error {
    // First get the file info
    file, err := c.GetFile(ctx, fileID)
    if err != nil {
        return fmt.Errorf("failed to get file info: %w", err)
    }
    
    if file.FilePath == "" {
        return fmt.Errorf("file path is empty")
    }
    
    // Download the file
    downloadURL := c.FileDownloadURL(file.FilePath)
    
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
    if err != nil {
        return fmt.Errorf("failed to create download request: %w", err)
    }
    
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("download request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("download failed with status: %d", resp.StatusCode)
    }
    
    // Limit download size (20MB max per Telegram docs)
    const maxDownloadSize = 20 << 20
    limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)
    
    n, err := io.Copy(w, limitedReader)
    if err != nil {
        return fmt.Errorf("failed to write file: %w", err)
    }
    
    if n > maxDownloadSize {
        return fmt.Errorf("file exceeds maximum download size of 20MB")
    }
    
    return nil
}
```

**Add to tg/types.go:**
```go
// File represents a file ready to be downloaded
type File struct {
    FileID       string `json:"file_id"`
    FileUniqueID string `json:"file_unique_id"`
    FileSize     int64  `json:"file_size,omitempty"`
    FilePath     string `json:"file_path,omitempty"`
}
```

---

### 5.3 sendChatAction

**File:** `sender/methods_chat.go` (new file)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// ChatAction represents a chat action type
type ChatAction string

const (
    ActionTyping          ChatAction = "typing"
    ActionUploadPhoto     ChatAction = "upload_photo"
    ActionRecordVideo     ChatAction = "record_video"
    ActionUploadVideo     ChatAction = "upload_video"
    ActionRecordVoice     ChatAction = "record_voice"
    ActionUploadVoice     ChatAction = "upload_voice"
    ActionUploadDocument  ChatAction = "upload_document"
    ActionChooseSticker   ChatAction = "choose_sticker"
    ActionFindLocation    ChatAction = "find_location"
    ActionRecordVideoNote ChatAction = "record_video_note"
    ActionUploadVideoNote ChatAction = "upload_video_note"
)

// SendChatActionRequest contains parameters for sendChatAction
type SendChatActionRequest struct {
    // ChatID is the unique identifier for the target chat
    ChatID tg.ChatID `json:"chat_id"`
    
    // Action is the type of action to broadcast
    Action ChatAction `json:"action"`
    
    // MessageThreadID is the unique identifier for the target message thread (topic)
    // For supergroups only
    MessageThreadID int `json:"message_thread_id,omitempty"`
    
    // BusinessConnectionID is the unique identifier of the business connection
    BusinessConnectionID string `json:"business_connection_id,omitempty"`
}

// SendChatAction broadcasts a chat action to tell the user that something 
// is happening on the bot's side. The status is set for 5 seconds or less 
// (when a message arrives from your bot, Telegram clients clear its typing status).
// 
// Use this method when you need some time to process a request and want your 
// users to know that the bot is working.
// 
// Telegram docs: https://core.telegram.org/bots/api#sendchataction
func (c *Client) SendChatAction(ctx context.Context, chatID tg.ChatID, action ChatAction, opts ...ChatActionOption) error {
    req := SendChatActionRequest{
        ChatID: chatID,
        Action: action,
    }
    
    for _, opt := range opts {
        opt(&req)
    }
    
    return c.Execute(ctx, "sendChatAction", req, nil)
}

// ChatActionOption configures a SendChatAction request
type ChatActionOption func(*SendChatActionRequest)

// WithMessageThread sets the message thread ID for forum topics
func WithMessageThread(threadID int) ChatActionOption {
    return func(r *SendChatActionRequest) {
        r.MessageThreadID = threadID
    }
}

// WithBusinessConnection sets the business connection ID
func WithBusinessConnection(connID string) ChatActionOption {
    return func(r *SendChatActionRequest) {
        r.BusinessConnectionID = connID
    }
}
```

---

## 6. Phase 3: Media Methods

### 6.1 sendDocument

**File:** `sender/methods_media.go` (new file)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// SendDocumentRequest contains parameters for sendDocument
type SendDocumentRequest struct {
    // ChatID is the unique identifier for the target chat
    ChatID tg.ChatID `json:"chat_id"`
    
    // Document is the file to send
    // Pass a file_id to send a file that exists on the Telegram servers,
    // pass an HTTP URL for Telegram to get a file from the Internet,
    // or use InputFileUpload for a new upload
    Document tg.InputFile `json:"document"`
    
    // MessageThreadID is the unique identifier for the target message thread (topic)
    MessageThreadID int `json:"message_thread_id,omitempty"`
    
    // Thumbnail of the file sent (JPEG, max 200KB, max 320x320)
    Thumbnail tg.InputFile `json:"thumbnail,omitempty"`
    
    // Caption for the document, 0-1024 characters
    Caption string `json:"caption,omitempty"`
    
    // ParseMode for the caption (HTML, Markdown, MarkdownV2)
    ParseMode tg.ParseMode `json:"parse_mode,omitempty"`
    
    // CaptionEntities is a list of special entities in the caption
    CaptionEntities []tg.MessageEntity `json:"caption_entities,omitempty"`
    
    // DisableContentTypeDetection disables automatic content type detection
    DisableContentTypeDetection bool `json:"disable_content_type_detection,omitempty"`
    
    // DisableNotification sends the message silently
    DisableNotification bool `json:"disable_notification,omitempty"`
    
    // ProtectContent protects the message from forwarding and saving
    ProtectContent bool `json:"protect_content,omitempty"`
    
    // MessageEffectID is the unique identifier of the message effect
    MessageEffectID string `json:"message_effect_id,omitempty"`
    
    // ReplyParameters describes the message to reply to
    ReplyParameters *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    
    // ReplyMarkup is the inline keyboard or other reply markup
    ReplyMarkup any `json:"reply_markup,omitempty"`
    
    // BusinessConnectionID is the unique identifier of the business connection
    BusinessConnectionID string `json:"business_connection_id,omitempty"`
}

// SendDocument sends a general file
// Bots can currently send files of up to 50 MB in size.
// 
// Telegram docs: https://core.telegram.org/bots/api#senddocument
func (c *Client) SendDocument(ctx context.Context, req SendDocumentRequest) (*tg.Message, error) {
    // Validate required fields
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Document == nil {
        return nil, tg.NewValidationError("document", "is required")
    }
    
    // Determine request mode
    mode := RequestModeJSON
    if tg.IsUpload(req.Document) || (req.Thumbnail != nil && tg.IsUpload(req.Thumbnail)) {
        mode = RequestModeMultipart
    }
    
    var msg tg.Message
    if err := c.ExecuteWithOptions(ctx, "sendDocument", req, &msg, ExecuteOptions{Mode: mode}); err != nil {
        return nil, err
    }
    return &msg, nil
}

// DocumentOption configures a SendDocument request
type DocumentOption func(*SendDocumentRequest)

// WithDocCaption sets the caption for the document
func WithDocCaption(caption string, parseMode tg.ParseMode) DocumentOption {
    return func(r *SendDocumentRequest) {
        r.Caption = caption
        r.ParseMode = parseMode
    }
}

// WithDocThumbnail sets a custom thumbnail for the document
func WithDocThumbnail(thumbnail tg.InputFile) DocumentOption {
    return func(r *SendDocumentRequest) {
        r.Thumbnail = thumbnail
    }
}

// WithDocDisableContentTypeDetection disables automatic content type detection
func WithDocDisableContentTypeDetection() DocumentOption {
    return func(r *SendDocumentRequest) {
        r.DisableContentTypeDetection = true
    }
}
```

---

### 6.2 sendVideo

```go
// SendVideoRequest contains parameters for sendVideo
type SendVideoRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    Video               tg.InputFile        `json:"video"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Duration            int                 `json:"duration,omitempty"`
    Width               int                 `json:"width,omitempty"`
    Height              int                 `json:"height,omitempty"`
    Thumbnail           tg.InputFile        `json:"thumbnail,omitempty"`
    Caption             string              `json:"caption,omitempty"`
    ParseMode           tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities     []tg.MessageEntity  `json:"caption_entities,omitempty"`
    ShowCaptionAboveMedia bool              `json:"show_caption_above_media,omitempty"`
    HasSpoiler          bool                `json:"has_spoiler,omitempty"`
    SupportsStreaming   bool                `json:"supports_streaming,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

// SendVideo sends a video file
// Telegram supports MP4 videos (other formats may work but aren't guaranteed).
// Bots can send video files of up to 50 MB in size.
// 
// Telegram docs: https://core.telegram.org/bots/api#sendvideo
func (c *Client) SendVideo(ctx context.Context, req SendVideoRequest) (*tg.Message, error) {
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Video == nil {
        return nil, tg.NewValidationError("video", "is required")
    }
    
    mode := RequestModeJSON
    if tg.IsUpload(req.Video) || (req.Thumbnail != nil && tg.IsUpload(req.Thumbnail)) {
        mode = RequestModeMultipart
    }
    
    var msg tg.Message
    if err := c.ExecuteWithOptions(ctx, "sendVideo", req, &msg, ExecuteOptions{Mode: mode}); err != nil {
        return nil, err
    }
    return &msg, nil
}
```

---

### 6.3 sendAudio

```go
// SendAudioRequest contains parameters for sendAudio
type SendAudioRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    Audio               tg.InputFile        `json:"audio"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Caption             string              `json:"caption,omitempty"`
    ParseMode           tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities     []tg.MessageEntity  `json:"caption_entities,omitempty"`
    Duration            int                 `json:"duration,omitempty"`
    Performer           string              `json:"performer,omitempty"`
    Title               string              `json:"title,omitempty"`
    Thumbnail           tg.InputFile        `json:"thumbnail,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

// SendAudio sends an audio file
// For sending voice messages, use sendVoice instead.
// Bots can send audio files of up to 50 MB in size.
// 
// Telegram docs: https://core.telegram.org/bots/api#sendaudio
func (c *Client) SendAudio(ctx context.Context, req SendAudioRequest) (*tg.Message, error) {
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Audio == nil {
        return nil, tg.NewValidationError("audio", "is required")
    }
    
    mode := RequestModeJSON
    if tg.IsUpload(req.Audio) || (req.Thumbnail != nil && tg.IsUpload(req.Thumbnail)) {
        mode = RequestModeMultipart
    }
    
    var msg tg.Message
    if err := c.ExecuteWithOptions(ctx, "sendAudio", req, &msg, ExecuteOptions{Mode: mode}); err != nil {
        return nil, err
    }
    return &msg, nil
}
```

---

### 6.4 sendVoice, sendAnimation, sendVideoNote, sendSticker

```go
// SendVoiceRequest contains parameters for sendVoice
type SendVoiceRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    Voice               tg.InputFile        `json:"voice"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Caption             string              `json:"caption,omitempty"`
    ParseMode           tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities     []tg.MessageEntity  `json:"caption_entities,omitempty"`
    Duration            int                 `json:"duration,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

func (c *Client) SendVoice(ctx context.Context, req SendVoiceRequest) (*tg.Message, error) {
    // Implementation similar to SendAudio...
}

// SendAnimationRequest contains parameters for sendAnimation
type SendAnimationRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    Animation           tg.InputFile        `json:"animation"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Duration            int                 `json:"duration,omitempty"`
    Width               int                 `json:"width,omitempty"`
    Height              int                 `json:"height,omitempty"`
    Thumbnail           tg.InputFile        `json:"thumbnail,omitempty"`
    Caption             string              `json:"caption,omitempty"`
    ParseMode           tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities     []tg.MessageEntity  `json:"caption_entities,omitempty"`
    ShowCaptionAboveMedia bool              `json:"show_caption_above_media,omitempty"`
    HasSpoiler          bool                `json:"has_spoiler,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

func (c *Client) SendAnimation(ctx context.Context, req SendAnimationRequest) (*tg.Message, error) {
    // Implementation similar to SendVideo...
}

// SendVideoNoteRequest contains parameters for sendVideoNote
type SendVideoNoteRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    VideoNote           tg.InputFile        `json:"video_note"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Duration            int                 `json:"duration,omitempty"`
    Length              int                 `json:"length,omitempty"` // Video width and height (diameter)
    Thumbnail           tg.InputFile        `json:"thumbnail,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

func (c *Client) SendVideoNote(ctx context.Context, req SendVideoNoteRequest) (*tg.Message, error) {
    // Implementation...
}

// SendStickerRequest contains parameters for sendSticker
type SendStickerRequest struct {
    ChatID              tg.ChatID           `json:"chat_id"`
    Sticker             tg.InputFile        `json:"sticker"`
    MessageThreadID     int                 `json:"message_thread_id,omitempty"`
    Emoji               string              `json:"emoji,omitempty"`
    DisableNotification bool                `json:"disable_notification,omitempty"`
    ProtectContent      bool                `json:"protect_content,omitempty"`
    MessageEffectID     string              `json:"message_effect_id,omitempty"`
    ReplyParameters     *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup         any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string             `json:"business_connection_id,omitempty"`
}

func (c *Client) SendSticker(ctx context.Context, req SendStickerRequest) (*tg.Message, error) {
    // Implementation...
}
```

---

### 6.5 sendMediaGroup

**File:** `sender/methods_media_group.go` (new file)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// InputMedia represents content of a media message to be sent
type InputMedia interface {
    inputMedia()
}

// InputMediaPhoto represents a photo to be sent in a media group
type InputMediaPhoto struct {
    Type                  string              `json:"type"` // Always "photo"
    Media                 tg.InputFile        `json:"media"`
    Caption               string              `json:"caption,omitempty"`
    ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
    ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
    HasSpoiler            bool                `json:"has_spoiler,omitempty"`
}

func (m InputMediaPhoto) inputMedia() {}

// NewInputMediaPhoto creates a new InputMediaPhoto
func NewInputMediaPhoto(media tg.InputFile) InputMediaPhoto {
    return InputMediaPhoto{
        Type:  "photo",
        Media: media,
    }
}

// InputMediaVideo represents a video to be sent in a media group
type InputMediaVideo struct {
    Type                  string              `json:"type"` // Always "video"
    Media                 tg.InputFile        `json:"media"`
    Thumbnail             tg.InputFile        `json:"thumbnail,omitempty"`
    Caption               string              `json:"caption,omitempty"`
    ParseMode             tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities       []tg.MessageEntity  `json:"caption_entities,omitempty"`
    ShowCaptionAboveMedia bool                `json:"show_caption_above_media,omitempty"`
    Width                 int                 `json:"width,omitempty"`
    Height                int                 `json:"height,omitempty"`
    Duration              int                 `json:"duration,omitempty"`
    SupportsStreaming     bool                `json:"supports_streaming,omitempty"`
    HasSpoiler            bool                `json:"has_spoiler,omitempty"`
}

func (m InputMediaVideo) inputMedia() {}

// NewInputMediaVideo creates a new InputMediaVideo
func NewInputMediaVideo(media tg.InputFile) InputMediaVideo {
    return InputMediaVideo{
        Type:  "video",
        Media: media,
    }
}

// InputMediaDocument represents a document to be sent in a media group
type InputMediaDocument struct {
    Type                        string              `json:"type"` // Always "document"
    Media                       tg.InputFile        `json:"media"`
    Thumbnail                   tg.InputFile        `json:"thumbnail,omitempty"`
    Caption                     string              `json:"caption,omitempty"`
    ParseMode                   tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities             []tg.MessageEntity  `json:"caption_entities,omitempty"`
    DisableContentTypeDetection bool                `json:"disable_content_type_detection,omitempty"`
}

func (m InputMediaDocument) inputMedia() {}

// InputMediaAudio represents an audio file to be sent in a media group
type InputMediaAudio struct {
    Type            string              `json:"type"` // Always "audio"
    Media           tg.InputFile        `json:"media"`
    Thumbnail       tg.InputFile        `json:"thumbnail,omitempty"`
    Caption         string              `json:"caption,omitempty"`
    ParseMode       tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities []tg.MessageEntity  `json:"caption_entities,omitempty"`
    Duration        int                 `json:"duration,omitempty"`
    Performer       string              `json:"performer,omitempty"`
    Title           string              `json:"title,omitempty"`
}

func (m InputMediaAudio) inputMedia() {}

// SendMediaGroupRequest contains parameters for sendMediaGroup
type SendMediaGroupRequest struct {
    ChatID               tg.ChatID    `json:"chat_id"`
    Media                []InputMedia `json:"media"`
    MessageThreadID      int          `json:"message_thread_id,omitempty"`
    DisableNotification  bool         `json:"disable_notification,omitempty"`
    ProtectContent       bool         `json:"protect_content,omitempty"`
    MessageEffectID      string       `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    BusinessConnectionID string       `json:"business_connection_id,omitempty"`
}

// SendMediaGroup sends a group of photos, videos, documents or audios as an album
// Documents and audio files can only be grouped together.
// On success, an array of Messages that were sent is returned.
// 
// Telegram docs: https://core.telegram.org/bots/api#sendmediagroup
func (c *Client) SendMediaGroup(ctx context.Context, req SendMediaGroupRequest) ([]tg.Message, error) {
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if len(req.Media) < 2 {
        return nil, tg.NewValidationError("media", "must contain at least 2 items")
    }
    if len(req.Media) > 10 {
        return nil, tg.NewValidationError("media", "must contain at most 10 items")
    }
    
    // Check if any media requires upload
    needsMultipart := false
    for _, m := range req.Media {
        if hasFileUpload(m) {
            needsMultipart = true
            break
        }
    }
    
    mode := RequestModeJSON
    if needsMultipart {
        mode = RequestModeMultipart
    }
    
    var messages []tg.Message
    if err := c.ExecuteWithOptions(ctx, "sendMediaGroup", req, &messages, ExecuteOptions{Mode: mode}); err != nil {
        return nil, err
    }
    return messages, nil
}

func hasFileUpload(m InputMedia) bool {
    // Check if any field in the InputMedia is an upload
    // Implementation depends on reflection or type switches
    switch media := m.(type) {
    case InputMediaPhoto:
        return tg.IsUpload(media.Media)
    case InputMediaVideo:
        return tg.IsUpload(media.Media) || (media.Thumbnail != nil && tg.IsUpload(media.Thumbnail))
    case InputMediaDocument:
        return tg.IsUpload(media.Media) || (media.Thumbnail != nil && tg.IsUpload(media.Thumbnail))
    case InputMediaAudio:
        return tg.IsUpload(media.Media) || (media.Thumbnail != nil && tg.IsUpload(media.Thumbnail))
    }
    return false
}
```

---

## 7. Phase 4: Utility Methods

### 7.1 sendLocation, sendVenue, sendContact

```go
// SendLocationRequest contains parameters for sendLocation
type SendLocationRequest struct {
    ChatID               tg.ChatID           `json:"chat_id"`
    Latitude             float64             `json:"latitude"`
    Longitude            float64             `json:"longitude"`
    MessageThreadID      int                 `json:"message_thread_id,omitempty"`
    HorizontalAccuracy   float64             `json:"horizontal_accuracy,omitempty"`
    LivePeriod           int                 `json:"live_period,omitempty"` // 60-86400 for live location
    Heading              int                 `json:"heading,omitempty"` // 1-360
    ProximityAlertRadius int                 `json:"proximity_alert_radius,omitempty"` // 1-100000 meters
    DisableNotification  bool                `json:"disable_notification,omitempty"`
    ProtectContent       bool                `json:"protect_content,omitempty"`
    MessageEffectID      string              `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup          any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string              `json:"business_connection_id,omitempty"`
}

func (c *Client) SendLocation(ctx context.Context, req SendLocationRequest) (*tg.Message, error) {
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Latitude < -90 || req.Latitude > 90 {
        return nil, tg.NewValidationError("latitude", "must be between -90 and 90")
    }
    if req.Longitude < -180 || req.Longitude > 180 {
        return nil, tg.NewValidationError("longitude", "must be between -180 and 180")
    }
    
    var msg tg.Message
    if err := c.Execute(ctx, "sendLocation", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

// SendVenueRequest contains parameters for sendVenue
type SendVenueRequest struct {
    ChatID               tg.ChatID           `json:"chat_id"`
    Latitude             float64             `json:"latitude"`
    Longitude            float64             `json:"longitude"`
    Title                string              `json:"title"`
    Address              string              `json:"address"`
    MessageThreadID      int                 `json:"message_thread_id,omitempty"`
    FoursquareID         string              `json:"foursquare_id,omitempty"`
    FoursquareType       string              `json:"foursquare_type,omitempty"`
    GooglePlaceID        string              `json:"google_place_id,omitempty"`
    GooglePlaceType      string              `json:"google_place_type,omitempty"`
    DisableNotification  bool                `json:"disable_notification,omitempty"`
    ProtectContent       bool                `json:"protect_content,omitempty"`
    MessageEffectID      string              `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup          any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string              `json:"business_connection_id,omitempty"`
}

func (c *Client) SendVenue(ctx context.Context, req SendVenueRequest) (*tg.Message, error) {
    // Validation and execution...
}

// SendContactRequest contains parameters for sendContact
type SendContactRequest struct {
    ChatID               tg.ChatID           `json:"chat_id"`
    PhoneNumber          string              `json:"phone_number"`
    FirstName            string              `json:"first_name"`
    LastName             string              `json:"last_name,omitempty"`
    Vcard                string              `json:"vcard,omitempty"`
    MessageThreadID      int                 `json:"message_thread_id,omitempty"`
    DisableNotification  bool                `json:"disable_notification,omitempty"`
    ProtectContent       bool                `json:"protect_content,omitempty"`
    MessageEffectID      string              `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup          any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID string              `json:"business_connection_id,omitempty"`
}

func (c *Client) SendContact(ctx context.Context, req SendContactRequest) (*tg.Message, error) {
    // Validation and execution...
}
```

---

### 7.2 sendPoll

```go
// SendPollRequest contains parameters for sendPoll
type SendPollRequest struct {
    ChatID                tg.ChatID           `json:"chat_id"`
    Question              string              `json:"question"`
    Options               []tg.InputPollOption `json:"options"`
    MessageThreadID       int                 `json:"message_thread_id,omitempty"`
    QuestionParseMode     tg.ParseMode        `json:"question_parse_mode,omitempty"`
    QuestionEntities      []tg.MessageEntity  `json:"question_entities,omitempty"`
    IsAnonymous           *bool               `json:"is_anonymous,omitempty"` // Default: true
    Type                  string              `json:"type,omitempty"` // "quiz" or "regular"
    AllowsMultipleAnswers bool                `json:"allows_multiple_answers,omitempty"`
    CorrectOptionID       *int                `json:"correct_option_id,omitempty"` // For quiz
    Explanation           string              `json:"explanation,omitempty"`
    ExplanationParseMode  tg.ParseMode        `json:"explanation_parse_mode,omitempty"`
    ExplanationEntities   []tg.MessageEntity  `json:"explanation_entities,omitempty"`
    OpenPeriod            int                 `json:"open_period,omitempty"` // 5-600 seconds
    CloseDate             int64               `json:"close_date,omitempty"` // Unix timestamp
    IsClosed              bool                `json:"is_closed,omitempty"`
    DisableNotification   bool                `json:"disable_notification,omitempty"`
    ProtectContent        bool                `json:"protect_content,omitempty"`
    MessageEffectID       string              `json:"message_effect_id,omitempty"`
    ReplyParameters       *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup           any                 `json:"reply_markup,omitempty"`
    BusinessConnectionID  string              `json:"business_connection_id,omitempty"`
}

// InputPollOption contains information about one answer option in a poll
type InputPollOption struct {
    Text          string             `json:"text"`
    TextParseMode tg.ParseMode       `json:"text_parse_mode,omitempty"`
    TextEntities  []tg.MessageEntity `json:"text_entities,omitempty"`
}

func (c *Client) SendPoll(ctx context.Context, req SendPollRequest) (*tg.Message, error) {
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    if req.Question == "" {
        return nil, tg.NewValidationError("question", "is required")
    }
    if len(req.Options) < 2 {
        return nil, tg.NewValidationError("options", "must have at least 2 options")
    }
    if len(req.Options) > 10 {
        return nil, tg.NewValidationError("options", "must have at most 10 options")
    }
    
    var msg tg.Message
    if err := c.Execute(ctx, "sendPoll", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

// StopPoll stops a poll
func (c *Client) StopPoll(ctx context.Context, chatID tg.ChatID, messageID int, replyMarkup any) (*tg.Poll, error) {
    params := map[string]any{
        "chat_id":    chatID,
        "message_id": messageID,
    }
    if replyMarkup != nil {
        params["reply_markup"] = replyMarkup
    }
    
    var poll tg.Poll
    if err := c.Execute(ctx, "stopPoll", params, &poll); err != nil {
        return nil, err
    }
    return &poll, nil
}
```

---

### 7.3 deleteMessages, forwardMessages, copyMessages

```go
// DeleteMessagesRequest contains parameters for deleteMessages
type DeleteMessagesRequest struct {
    ChatID     tg.ChatID `json:"chat_id"`
    MessageIDs []int     `json:"message_ids"` // 1-100 message IDs
}

// DeleteMessages deletes multiple messages simultaneously
// Returns True on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#deletemessages
func (c *Client) DeleteMessages(ctx context.Context, chatID tg.ChatID, messageIDs []int) error {
    if len(messageIDs) < 1 {
        return tg.NewValidationError("message_ids", "must have at least 1 message")
    }
    if len(messageIDs) > 100 {
        return tg.NewValidationError("message_ids", "must have at most 100 messages")
    }
    
    req := DeleteMessagesRequest{
        ChatID:     chatID,
        MessageIDs: messageIDs,
    }
    return c.Execute(ctx, "deleteMessages", req, nil)
}

// ForwardMessagesRequest contains parameters for forwardMessages
type ForwardMessagesRequest struct {
    ChatID              tg.ChatID `json:"chat_id"`
    FromChatID          tg.ChatID `json:"from_chat_id"`
    MessageIDs          []int     `json:"message_ids"` // 1-100 message IDs
    MessageThreadID     int       `json:"message_thread_id,omitempty"`
    DisableNotification bool      `json:"disable_notification,omitempty"`
    ProtectContent      bool      `json:"protect_content,omitempty"`
}

// ForwardMessages forwards multiple messages of any kind
// Returns an array of MessageId of the sent messages on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#forwardmessages
func (c *Client) ForwardMessages(ctx context.Context, req ForwardMessagesRequest) ([]tg.MessageID, error) {
    if len(req.MessageIDs) < 1 || len(req.MessageIDs) > 100 {
        return nil, tg.NewValidationError("message_ids", "must have 1-100 messages")
    }
    
    var ids []tg.MessageID
    if err := c.Execute(ctx, "forwardMessages", req, &ids); err != nil {
        return nil, err
    }
    return ids, nil
}

// CopyMessagesRequest contains parameters for copyMessages
type CopyMessagesRequest struct {
    ChatID              tg.ChatID `json:"chat_id"`
    FromChatID          tg.ChatID `json:"from_chat_id"`
    MessageIDs          []int     `json:"message_ids"`
    MessageThreadID     int       `json:"message_thread_id,omitempty"`
    DisableNotification bool      `json:"disable_notification,omitempty"`
    ProtectContent      bool      `json:"protect_content,omitempty"`
    RemoveCaption       bool      `json:"remove_caption,omitempty"`
}

// CopyMessages copies messages of any kind
// Returns an array of MessageId of the sent messages on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#copymessages
func (c *Client) CopyMessages(ctx context.Context, req CopyMessagesRequest) ([]tg.MessageID, error) {
    if len(req.MessageIDs) < 1 || len(req.MessageIDs) > 100 {
        return nil, tg.NewValidationError("message_ids", "must have 1-100 messages")
    }
    
    var ids []tg.MessageID
    if err := c.Execute(ctx, "copyMessages", req, &ids); err != nil {
        return nil, err
    }
    return ids, nil
}
```

---

## 8. Phase 5: Bot API 9.x Methods

### 8.1 sendMessageDraft (Bot API 9.3)

```go
// SendMessageDraftRequest contains parameters for sendMessageDraft
// This method is used to stream partial messages while generating content.
type SendMessageDraftRequest struct {
    // BusinessConnectionID is required for business bots
    BusinessConnectionID string `json:"business_connection_id"`
    
    // ChatID is the unique identifier for the target chat
    ChatID tg.ChatID `json:"chat_id"`
    
    // DraftMessageID identifies the draft to update (returned from previous call)
    DraftMessageID int `json:"draft_message_id,omitempty"`
    
    // Text is the partial text of the message being generated
    Text string `json:"text"`
    
    // ParseMode for the text
    ParseMode tg.ParseMode `json:"parse_mode,omitempty"`
    
    // Entities is a list of special entities in the text
    Entities []tg.MessageEntity `json:"entities,omitempty"`
    
    // DisableWebPagePreview disables link previews
    DisableWebPagePreview bool `json:"disable_web_page_preview,omitempty"`
}

// SendMessageDraftResult contains the result of sendMessageDraft
type SendMessageDraftResult struct {
    // DraftMessageID to use in subsequent calls
    DraftMessageID int `json:"draft_message_id"`
}

// SendMessageDraft streams partial message content while generating
// Use this to show users that content is being generated (like ChatGPT-style streaming).
// Call repeatedly with increasing text to show progress.
// When complete, use sendMessage to send the final message.
// 
// Telegram docs: https://core.telegram.org/bots/api#sendmessagedraft
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) (*SendMessageDraftResult, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    var result SendMessageDraftResult
    if err := c.Execute(ctx, "sendMessageDraft", req, &result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

### 8.2 setMessageReaction (Bot API 7.0)

```go
// SetMessageReactionRequest contains parameters for setMessageReaction
type SetMessageReactionRequest struct {
    ChatID    tg.ChatID       `json:"chat_id"`
    MessageID int             `json:"message_id"`
    Reaction  []ReactionType  `json:"reaction,omitempty"`
    IsBig     bool            `json:"is_big,omitempty"`
}

// ReactionType represents a reaction type
type ReactionType struct {
    Type        string `json:"type"` // "emoji" or "custom_emoji"
    Emoji       string `json:"emoji,omitempty"`
    CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// NewEmojiReaction creates a new emoji reaction
func NewEmojiReaction(emoji string) ReactionType {
    return ReactionType{Type: "emoji", Emoji: emoji}
}

// NewCustomEmojiReaction creates a new custom emoji reaction
func NewCustomEmojiReaction(emojiID string) ReactionType {
    return ReactionType{Type: "custom_emoji", CustomEmojiID: emojiID}
}

// SetMessageReaction changes the chosen reactions on a message
// Returns True on success.
// 
// Telegram docs: https://core.telegram.org/bots/api#setmessagereaction
func (c *Client) SetMessageReaction(ctx context.Context, chatID tg.ChatID, messageID int, reactions []ReactionType, isBig bool) error {
    req := SetMessageReactionRequest{
        ChatID:    chatID,
        MessageID: messageID,
        Reaction:  reactions,
        IsBig:     isBig,
    }
    return c.Execute(ctx, "setMessageReaction", req, nil)
}
```

---

## 9. Type Definitions

### 9.1 New Types to Add to tg/types.go

```go
// ReplyParameters describes reply parameters for messages
type ReplyParameters struct {
    MessageID                int             `json:"message_id"`
    ChatID                   ChatID          `json:"chat_id,omitempty"`
    AllowSendingWithoutReply bool            `json:"allow_sending_without_reply,omitempty"`
    Quote                    string          `json:"quote,omitempty"`
    QuoteParseMode           ParseMode       `json:"quote_parse_mode,omitempty"`
    QuoteEntities            []MessageEntity `json:"quote_entities,omitempty"`
    QuotePosition            int             `json:"quote_position,omitempty"`
}

// UserProfilePhotos contains a user's profile pictures
type UserProfilePhotos struct {
    TotalCount int           `json:"total_count"`
    Photos     [][]PhotoSize `json:"photos"`
}

// Sticker represents a sticker
type Sticker struct {
    FileID           string        `json:"file_id"`
    FileUniqueID     string        `json:"file_unique_id"`
    Type             string        `json:"type"` // "regular", "mask", "custom_emoji"
    Width            int           `json:"width"`
    Height           int           `json:"height"`
    IsAnimated       bool          `json:"is_animated"`
    IsVideo          bool          `json:"is_video"`
    Thumbnail        *PhotoSize    `json:"thumbnail,omitempty"`
    Emoji            string        `json:"emoji,omitempty"`
    SetName          string        `json:"set_name,omitempty"`
    PremiumAnimation *File         `json:"premium_animation,omitempty"`
    MaskPosition     *MaskPosition `json:"mask_position,omitempty"`
    CustomEmojiID    string        `json:"custom_emoji_id,omitempty"`
    NeedsRepainting  bool          `json:"needs_repainting,omitempty"`
    FileSize         int64         `json:"file_size,omitempty"`
}

// MaskPosition describes the position on a face where a mask should be placed
type MaskPosition struct {
    Point  string  `json:"point"` // "forehead", "eyes", "mouth", "chin"
    XShift float64 `json:"x_shift"`
    YShift float64 `json:"y_shift"`
    Scale  float64 `json:"scale"`
}

// Animation represents an animation file (GIF or H.264/MPEG-4 AVC video without sound)
type Animation struct {
    FileID       string     `json:"file_id"`
    FileUniqueID string     `json:"file_unique_id"`
    Width        int        `json:"width"`
    Height       int        `json:"height"`
    Duration     int        `json:"duration"`
    Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
    FileName     string     `json:"file_name,omitempty"`
    MimeType     string     `json:"mime_type,omitempty"`
    FileSize     int64      `json:"file_size,omitempty"`
}

// Dice represents an animated emoji that displays a random value
type Dice struct {
    Emoji string `json:"emoji"`
    Value int    `json:"value"`
}
```

---

## 10. Test Strategy

### 10.1 Test Categories

| Category | Purpose | Tools |
|----------|---------|-------|
| Unit Tests | Test individual functions | `testing` |
| Integration Tests | Test API interactions | `httptest.Server` |
| Fuzz Tests | Find edge cases | `testing/fuzz` |
| Race Tests | Detect data races | `go test -race` |
| Benchmark Tests | Performance | `testing.B` |

### 10.2 Test Helpers

**File:** `sender/testing.go`

```go
package sender

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/prilive-com/galigo/tg"
)

// newTestClient creates a Client configured for testing
func newTestClient(t *testing.T, baseURL, token string) *Client {
    t.Helper()
    
    client, err := NewFromConfig(Config{
        BaseURL: baseURL,
        Token:   tg.SecretToken{}.WithValue(token),
    })
    if err != nil {
        t.Fatalf("failed to create test client: %v", err)
    }
    return client
}

// mockTelegramServer creates a test server that responds with the given response
func mockTelegramServer(t *testing.T, response any) *httptest.Server {
    t.Helper()
    
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
    }))
}

// mockTelegramError creates a test server that returns an API error
func mockTelegramError(t *testing.T, code int, description string) *httptest.Server {
    t.Helper()
    
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{
            "ok":          false,
            "error_code":  code,
            "description": description,
        })
    }))
}

// assertAPIError checks that err is an APIError with the expected code
func assertAPIError(t *testing.T, err error, expectedCode int) {
    t.Helper()
    
    var apiErr *tg.APIError
    if !errors.As(err, &apiErr) {
        t.Errorf("expected APIError, got %T: %v", err, err)
        return
    }
    if apiErr.Code != expectedCode {
        t.Errorf("expected error code %d, got %d", expectedCode, apiErr.Code)
    }
}
```

### 10.3 Example Test Suite

**File:** `sender/methods_media_test.go`

```go
package sender

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/prilive-com/galigo/tg"
)

func TestSendDocument_FileID(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify JSON request
        assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
        
        var req map[string]any
        json.NewDecoder(r.Body).Decode(&req)
        
        assert.Equal(t, float64(12345), req["chat_id"])
        assert.Equal(t, "AgACAgIAAxkB...", req["document"])
        assert.Equal(t, "Test caption", req["caption"])
        
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": map[string]any{
                "message_id": 100,
                "chat": map[string]any{"id": 12345, "type": "private"},
                "document": map[string]any{
                    "file_id": "AgACAgIAAxkB...",
                    "file_name": "test.pdf",
                },
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "123:ABC")
    
    msg, err := client.SendDocument(context.Background(), SendDocumentRequest{
        ChatID:   int64(12345),
        Document: tg.InputFileID("AgACAgIAAxkB..."),
        Caption:  "Test caption",
    })
    
    require.NoError(t, err)
    assert.Equal(t, 100, msg.MessageID)
    assert.NotNil(t, msg.Document)
}

func TestSendDocument_Upload(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Verify multipart request
        assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))
        
        err := r.ParseMultipartForm(10 << 20)
        require.NoError(t, err)
        
        assert.Equal(t, "12345", r.FormValue("chat_id"))
        
        // Check file upload
        file, header, err := r.FormFile("document")
        require.NoError(t, err)
        defer file.Close()
        
        assert.Equal(t, "test.txt", header.Filename)
        
        content, _ := io.ReadAll(file)
        assert.Equal(t, "Hello, World!", string(content))
        
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": map[string]any{
                "message_id": 101,
                "chat": map[string]any{"id": 12345, "type": "private"},
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "123:ABC")
    
    msg, err := client.SendDocument(context.Background(), SendDocumentRequest{
        ChatID:   int64(12345),
        Document: tg.InputFileFromReader("test.txt", strings.NewReader("Hello, World!")),
    })
    
    require.NoError(t, err)
    assert.Equal(t, 101, msg.MessageID)
}

func TestSendMediaGroup_MixedMedia(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"))
        
        err := r.ParseMultipartForm(10 << 20)
        require.NoError(t, err)
        
        // Check media JSON array
        mediaJSON := r.FormValue("media")
        var media []map[string]any
        json.Unmarshal([]byte(mediaJSON), &media)
        
        assert.Len(t, media, 2)
        assert.Equal(t, "photo", media[0]["type"])
        assert.Equal(t, "video", media[1]["type"])
        
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true,
            "result": []map[string]any{
                {"message_id": 200},
                {"message_id": 201},
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL, "123:ABC")
    
    messages, err := client.SendMediaGroup(context.Background(), SendMediaGroupRequest{
        ChatID: int64(12345),
        Media: []InputMedia{
            NewInputMediaPhoto(tg.InputFileID("photo_file_id")),
            NewInputMediaVideo(tg.InputFileID("video_file_id")),
        },
    })
    
    require.NoError(t, err)
    assert.Len(t, messages, 2)
    assert.Equal(t, 200, messages[0].MessageID)
    assert.Equal(t, 201, messages[1].MessageID)
}
```

---

## Appendix A: File Structure

```
galigo/
├── bot.go                          # Bot facade (update with new methods)
├── options.go                      # Bot options
├── doc.go
│
├── tg/
│   ├── types.go                    # Core types (update with new types)
│   ├── inputfile.go                # NEW: InputFile abstraction
│   ├── errors.go                   # Update: unified error model
│   ├── keyboard.go
│   ├── secret.go
│   └── parsemode.go
│
├── sender/
│   ├── client.go                   # Update: use new executor
│   ├── executor.go                 # NEW: generic request executor
│   ├── multipart.go                # NEW: multipart builder
│   ├── config.go
│   ├── errors.go                   # Deprecate: use tg/errors.go
│   ├── options.go
│   ├── requests.go                 # Update: add new request types
│   │
│   ├── methods_bot.go              # NEW: getMe, logOut, close
│   ├── methods_files.go            # NEW: getFile, download
│   ├── methods_chat.go             # NEW: sendChatAction
│   ├── methods_media.go            # NEW: all media methods
│   ├── methods_media_group.go      # NEW: sendMediaGroup + InputMedia
│   ├── methods_location.go         # NEW: location/venue/contact
│   ├── methods_poll.go             # NEW: poll methods
│   ├── methods_bulk.go             # NEW: forwardMessages, copyMessages, deleteMessages
│   ├── methods_reaction.go         # NEW: setMessageReaction
│   ├── methods_draft.go            # NEW: sendMessageDraft
│   │
│   └── testing.go                  # NEW: test helpers
│
├── receiver/
│   ├── polling.go                  # FIX: P0 bugs
│   ├── webhook.go
│   ├── api.go
│   └── config.go
│
└── internal/
    ├── httpclient/                 # Consider using for shared HTTP setup
    ├── resilience/
    └── validate/                   # Wire into validation
```

---

## Appendix B: Checklist

### Phase 0: Bug Fixes
- [ ] P0.1 Fix update loss bug
- [ ] P0.2 Fix URL encoding
- [ ] P0.3 Fix retry_after parsing
- [ ] P0.4 Fix response size boundary

### Phase 1: Architecture
- [ ] A.1 Generic executor
- [ ] A.2 InputFile abstraction
- [ ] A.2 Multipart builder
- [ ] A.3 Unified error model
- [ ] A.4 int64 safety audit

### Phase 2: Core Methods
- [ ] getMe
- [ ] logOut
- [ ] close

### Phase 3: Media Methods
- [ ] sendDocument
- [ ] sendVideo
- [ ] sendAudio
- [ ] sendVoice
- [ ] sendAnimation
- [ ] sendVideoNote
- [ ] sendSticker
- [ ] sendMediaGroup
- [ ] editMessageMedia

### Phase 4: Utility Methods
- [ ] getFile
- [ ] sendChatAction
- [ ] getUserProfilePhotos
- [ ] sendLocation
- [ ] sendVenue
- [ ] sendContact
- [ ] sendPoll
- [ ] sendDice
- [ ] stopPoll
- [ ] editMessageLiveLocation
- [ ] stopMessageLiveLocation
- [ ] deleteMessages
- [ ] forwardMessages
- [ ] copyMessages
- [ ] setMessageReaction

### Phase 5: Bot API 9.x
- [ ] sendMessageDraft
- [ ] Update all methods with 9.x parameters

### Testing
- [ ] Unit tests for all new methods
- [ ] Integration tests with mock server
- [ ] Multipart upload tests
- [ ] Error handling tests
- [ ] Race condition tests

---

*Plan created January 2026*
*Total estimated effort: 3-4 weeks*

# galigo Tier 1 Implementation Plan v2.0

## Consolidated Technical Specification

**Version:** 2.0 (Enhanced)  
**Target:** galigo v2.0.0  
**Telegram Bot API:** 9.3 (Dec 31, 2025)  
**Go Version:** 1.25  
**Estimated Effort:** 4-5 weeks (11 PRs)

---

## Executive Summary

This plan consolidates multiple independent analyses into a single, actionable implementation roadmap. It's organized as **11 reviewable PRs** that can be merged incrementally.

### Key Improvements in v2.0

| Area | Previous Plan | Enhanced Plan |
|------|---------------|---------------|
| Structure | Phases | **11 specific PRs** with clear boundaries |
| CI/CD | Not covered | **PR0: CI gates before any code changes** |
| API Response | Manual parsing | **Generic `APIResponse[T any]`** |
| ChatID | Interface-based | **Struct with constructors** (safer) |
| File Security | Mentioned | **Explicit allowlist + traversal protection** |
| Versioning | Not covered | **v1.1.0 vs v2.0.0 strategy** |
| Modern Params | Mixed in | **Dedicated PR8 for 9.x parameters** |

---

## PR Dependency Graph

```
PR0 (CI/Deps)
    │
    ▼
PR1 (Response/Error Model)
    │
    ▼
PR2 (JSON Executor) ◄────────────────┐
    │                                │
    ▼                                │
PR3 (InputFile + Multipart) ─────────┤
    │                                │
    ├──────────┬──────────┬──────────┤
    ▼          ▼          ▼          │
PR4 (Core)  PR5 (Media) PR6 (Groups) │
    │          │          │          │
    └──────────┴──────────┴──────────┤
                                     │
PR7 (sendMessageDraft) ◄─────────────┤
    │                                │
    ▼                                │
PR8 (Modern Parameters) ◄────────────┘
    │
    ▼
PR9 (ChatID/int64 Hardening)
    │
    ▼
PR10 (Facade + Docs + Release)
```

---

## PR0: Project Hygiene & CI Gates

**Goal:** Establish foundation for safe, reviewable changes.  
**Estimated Time:** 2-4 hours  
**Breaking Changes:** None

### 0.1 Update Dependencies

```bash
# Current (outdated)
golang.org/x/time v0.5.0          # → v0.14.0+
github.com/sony/gobreaker/v2 v2.0.0  # → v2.4.0+
```

**go.mod changes:**
```go
module github.com/prilive-com/galigo

go 1.25
toolchain go1.25.0  // Lock toolchain for reproducible builds

require (
    github.com/sony/gobreaker/v2 v2.4.0
    golang.org/x/time v0.14.0
)
```

### 0.2 CI Pipeline (.github/workflows/ci.yml)

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Verify dependencies
        run: go mod verify
      
      - name: Build
        run: go build -v ./...
      
      - name: Test
        run: go test -v -race -coverprofile=coverage.out ./...
      
      - name: Vet
        run: go vet ./...
      
      - name: Staticcheck
        uses: dominikh/staticcheck-action@v1
        with:
          version: "latest"
      
      - name: Govulncheck
        run: |
          go install golang.org/x/vuln/cmd/govulncheck@latest
          govulncheck ./...
      
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: coverage.out
```

### Definition of Done (PR0)

- [ ] `go mod tidy` produces no changes
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` reports no issues
- [ ] `govulncheck ./...` reports no vulnerabilities
- [ ] CI pipeline runs on PR

---

## PR1: Unified Response & Error Model

**Goal:** Single source of truth for API responses and errors.  
**Estimated Time:** 4-6 hours  
**Breaking Changes:** Minor (error types)

### 1.1 Generic API Response

**File:** `tg/response.go` (new)

```go
package tg

import (
    "encoding/json"
    "fmt"
    "time"
)

// APIResponse represents a response from the Telegram Bot API.
// T is the type of the result field.
type APIResponse[T any] struct {
    // OK indicates whether the request was successful
    OK bool `json:"ok"`
    
    // Result contains the response data (only if OK is true)
    Result T `json:"result,omitempty"`
    
    // ErrorCode is the Telegram error code (only if OK is false)
    ErrorCode int `json:"error_code,omitempty"`
    
    // Description is the human-readable error description
    Description string `json:"description,omitempty"`
    
    // Parameters contains additional error information
    Parameters *ResponseParameters `json:"parameters,omitempty"`
}

// ResponseParameters contains additional information about errors
type ResponseParameters struct {
    // RetryAfter is the number of seconds to wait before retrying (for 429 errors)
    RetryAfter int `json:"retry_after,omitempty"`
    
    // MigrateToChatID is the new chat ID when a group migrates to supergroup
    MigrateToChatID int64 `json:"migrate_to_chat_id,omitempty"`
}

// ToError converts an unsuccessful APIResponse to an APIError
func (r *APIResponse[T]) ToError(method string) *APIError {
    if r.OK {
        return nil
    }
    
    err := &APIError{
        Method:      method,
        Code:        r.ErrorCode,
        Description: r.Description,
    }
    
    if r.Parameters != nil {
        err.Parameters = r.Parameters
        if r.Parameters.RetryAfter > 0 {
            err.RetryAfter = time.Duration(r.Parameters.RetryAfter) * time.Second
        }
        if r.Parameters.MigrateToChatID != 0 {
            err.MigrateToChatID = r.Parameters.MigrateToChatID
        }
    }
    
    return err
}
```

### 1.2 Canonical Error Type

**File:** `tg/errors.go` (replace existing)

```go
package tg

import (
    "errors"
    "fmt"
    "time"
)

// Sentinel errors for common API error conditions
var (
    ErrUnauthorized    = errors.New("galigo: unauthorized (invalid token)")
    ErrForbidden       = errors.New("galigo: forbidden")
    ErrNotFound        = errors.New("galigo: not found")
    ErrConflict        = errors.New("galigo: conflict (webhook/polling)")
    ErrTooManyRequests = errors.New("galigo: too many requests")
    ErrBadRequest      = errors.New("galigo: bad request")
    ErrInvalidToken    = errors.New("galigo: invalid token format")
)

// APIError represents an error returned by the Telegram Bot API.
// It implements the error interface and supports errors.Is/As.
type APIError struct {
    // Method is the API method that was called
    Method string
    
    // Code is the Telegram error code
    Code int
    
    // Description is the human-readable error message from Telegram
    Description string
    
    // RetryAfter indicates how long to wait before retrying (for 429 errors)
    // Parsed from response parameters, not HTTP headers
    RetryAfter time.Duration
    
    // MigrateToChatID is set when a group has migrated to a supergroup
    MigrateToChatID int64
    
    // Parameters contains the raw response parameters (for advanced use)
    Parameters *ResponseParameters
}

// Error implements the error interface
func (e *APIError) Error() string {
    if e.RetryAfter > 0 {
        return fmt.Sprintf("galigo: %s: [%d] %s (retry after %v)",
            e.Method, e.Code, e.Description, e.RetryAfter)
    }
    return fmt.Sprintf("galigo: %s: [%d] %s", e.Method, e.Code, e.Description)
}

// Is implements errors.Is for matching sentinel errors
func (e *APIError) Is(target error) bool {
    switch e.Code {
    case 400:
        return target == ErrBadRequest
    case 401:
        return target == ErrUnauthorized
    case 403:
        return target == ErrForbidden
    case 404:
        return target == ErrNotFound
    case 409:
        return target == ErrConflict
    case 429:
        return target == ErrTooManyRequests
    }
    return false
}

// Unwrap returns the underlying sentinel error for errors.Unwrap
func (e *APIError) Unwrap() error {
    switch e.Code {
    case 400:
        return ErrBadRequest
    case 401:
        return ErrUnauthorized
    case 403:
        return ErrForbidden
    case 404:
        return ErrNotFound
    case 409:
        return ErrConflict
    case 429:
        return ErrTooManyRequests
    }
    return nil
}

// IsRetryable returns true if the error is potentially retryable
func (e *APIError) IsRetryable() bool {
    // 429: Rate limited (retry after delay)
    // 5xx: Server errors (retry with backoff)
    return e.Code == 429 || (e.Code >= 500 && e.Code < 600)
}

// ValidationError represents a client-side validation error
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("galigo: validation error: %s: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
    return &ValidationError{Field: field, Message: message}
}
```

### 1.3 Tests

**File:** `tg/errors_test.go`

```go
package tg_test

import (
    "encoding/json"
    "errors"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/prilive-com/galigo/tg"
)

func TestAPIResponse_ParseRetryAfter(t *testing.T) {
    // This is the actual JSON Telegram returns for 429 errors
    jsonData := `{
        "ok": false,
        "error_code": 429,
        "description": "Too Many Requests: retry after 35",
        "parameters": {
            "retry_after": 35
        }
    }`
    
    var resp tg.APIResponse[json.RawMessage]
    err := json.Unmarshal([]byte(jsonData), &resp)
    require.NoError(t, err)
    
    assert.False(t, resp.OK)
    assert.Equal(t, 429, resp.ErrorCode)
    assert.NotNil(t, resp.Parameters)
    assert.Equal(t, 35, resp.Parameters.RetryAfter)
    
    apiErr := resp.ToError("sendMessage")
    require.NotNil(t, apiErr)
    assert.Equal(t, 35*time.Second, apiErr.RetryAfter)
    assert.True(t, errors.Is(apiErr, tg.ErrTooManyRequests))
    assert.True(t, apiErr.IsRetryable())
}

func TestAPIResponse_ParseMigration(t *testing.T) {
    jsonData := `{
        "ok": false,
        "error_code": 400,
        "description": "Bad Request: group chat was migrated to a supergroup chat",
        "parameters": {
            "migrate_to_chat_id": -1001234567890
        }
    }`
    
    var resp tg.APIResponse[json.RawMessage]
    err := json.Unmarshal([]byte(jsonData), &resp)
    require.NoError(t, err)
    
    apiErr := resp.ToError("sendMessage")
    assert.Equal(t, int64(-1001234567890), apiErr.MigrateToChatID)
}

func TestAPIError_Is(t *testing.T) {
    tests := []struct {
        code   int
        target error
        want   bool
    }{
        {401, tg.ErrUnauthorized, true},
        {403, tg.ErrForbidden, true},
        {404, tg.ErrNotFound, true},
        {409, tg.ErrConflict, true},
        {429, tg.ErrTooManyRequests, true},
        {400, tg.ErrBadRequest, true},
        {500, tg.ErrBadRequest, false}, // 500 is not BadRequest
    }
    
    for _, tt := range tests {
        t.Run(fmt.Sprintf("code_%d", tt.code), func(t *testing.T) {
            err := &tg.APIError{Code: tt.code}
            assert.Equal(t, tt.want, errors.Is(err, tt.target))
        })
    }
}
```

### Definition of Done (PR1)

- [ ] `tg.APIResponse[T]` can unmarshal all Telegram response formats
- [ ] `tg.APIError` correctly parses `retry_after` from JSON parameters
- [ ] `errors.Is(err, tg.ErrTooManyRequests)` works for 429 errors
- [ ] `errors.As(err, &apiErr)` works for all API errors
- [ ] Unit tests cover: success, error, 429 with retry, migration
- [ ] No changes to existing public API (yet)

---

## PR2: Core Request Executor (JSON)

**Goal:** Single, robust request executor for all API calls.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** Internal only

### 2.1 Executor Interface

**File:** `sender/executor.go` (new)

```go
package sender

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"
    
    "github.com/prilive-com/galigo/tg"
)

const (
    maxResponseSize = 10 << 20 // 10MB
)

// Executor handles HTTP requests to the Telegram Bot API
type Executor struct {
    httpClient    *http.Client
    baseURL       string
    token         string
    globalLimiter *rate.Limiter
    
    // Retry configuration
    maxRetries    int
    baseBackoff   time.Duration
    maxBackoff    time.Duration
}

// ExecutorConfig configures the executor
type ExecutorConfig struct {
    HTTPClient  *http.Client
    BaseURL     string
    Token       string
    GlobalRPS   float64
    MaxRetries  int
    BaseBackoff time.Duration
    MaxBackoff  time.Duration
}

// NewExecutor creates a new Executor with the given configuration
func NewExecutor(cfg ExecutorConfig) *Executor {
    if cfg.HTTPClient == nil {
        cfg.HTTPClient = &http.Client{
            Timeout: 60 * time.Second,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{
                    MinVersion: tls.VersionTLS12,
                },
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        }
    }
    
    if cfg.BaseURL == "" {
        cfg.BaseURL = "https://api.telegram.org"
    }
    
    if cfg.GlobalRPS <= 0 {
        cfg.GlobalRPS = 30 // Telegram's approximate limit
    }
    
    if cfg.MaxRetries <= 0 {
        cfg.MaxRetries = 3
    }
    
    if cfg.BaseBackoff <= 0 {
        cfg.BaseBackoff = 500 * time.Millisecond
    }
    
    if cfg.MaxBackoff <= 0 {
        cfg.MaxBackoff = 30 * time.Second
    }
    
    return &Executor{
        httpClient:    cfg.HTTPClient,
        baseURL:       cfg.BaseURL,
        token:         cfg.Token,
        globalLimiter: rate.NewLimiter(rate.Limit(cfg.GlobalRPS), int(cfg.GlobalRPS)),
        maxRetries:    cfg.MaxRetries,
        baseBackoff:   cfg.BaseBackoff,
        maxBackoff:    cfg.MaxBackoff,
    }
}

// Call executes a JSON API request with automatic retries
func (e *Executor) Call(ctx context.Context, method string, params any, result any) error {
    return e.CallWithOptions(ctx, method, params, result, CallOptions{})
}

// CallOptions configures a single API call
type CallOptions struct {
    // SkipGlobalLimit skips the global rate limiter
    SkipGlobalLimit bool
    
    // ChatID for per-chat rate limiting (optional)
    ChatID int64
}

// CallWithOptions executes a JSON API request with custom options
func (e *Executor) CallWithOptions(ctx context.Context, method string, params any, result any, opts CallOptions) error {
    var lastErr error
    
    for attempt := 0; attempt <= e.maxRetries; attempt++ {
        // Apply rate limiting
        if !opts.SkipGlobalLimit {
            if err := e.globalLimiter.Wait(ctx); err != nil {
                return fmt.Errorf("rate limit wait: %w", err)
            }
        }
        
        // Execute request
        err := e.doJSONRequest(ctx, method, params, result)
        if err == nil {
            return nil
        }
        
        lastErr = err
        
        // Check if retryable
        var apiErr *tg.APIError
        if errors.As(err, &apiErr) {
            if !apiErr.IsRetryable() {
                return err // Non-retryable error
            }
            
            // For 429, use Telegram's retry_after
            if apiErr.RetryAfter > 0 {
                select {
                case <-time.After(apiErr.RetryAfter):
                case <-ctx.Done():
                    return ctx.Err()
                }
                continue
            }
        }
        
        // For other retryable errors, use exponential backoff
        if attempt < e.maxRetries {
            backoff := e.calculateBackoff(attempt)
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return ctx.Err()
            }
        }
    }
    
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doJSONRequest performs a single JSON request (no retries)
func (e *Executor) doJSONRequest(ctx context.Context, method string, params any, result any) error {
    // Build URL using net/url (not string concatenation)
    apiURL, err := url.JoinPath(e.baseURL, "bot"+e.token, method)
    if err != nil {
        return fmt.Errorf("failed to build URL: %w", err)
    }
    
    // Marshal params
    var body io.Reader
    if params != nil {
        jsonData, err := json.Marshal(params)
        if err != nil {
            return fmt.Errorf("failed to marshal params: %w", err)
        }
        body = bytes.NewReader(jsonData)
    }
    
    // Create request
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, body)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")
    
    // Execute request
    resp, err := e.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Read response with size limit (+1 to detect overflow)
    limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
    respBody, err := io.ReadAll(limitedReader)
    if err != nil {
        return fmt.Errorf("failed to read response: %w", err)
    }
    
    if int64(len(respBody)) > maxResponseSize {
        return fmt.Errorf("response too large (>%d bytes)", maxResponseSize)
    }
    
    // Parse response
    var apiResp tg.APIResponse[json.RawMessage]
    if err := json.Unmarshal(respBody, &apiResp); err != nil {
        return fmt.Errorf("failed to parse response: %w", err)
    }
    
    // Check for API error
    if !apiResp.OK {
        return apiResp.ToError(method)
    }
    
    // Unmarshal result if provided
    if result != nil && len(apiResp.Result) > 0 {
        if err := json.Unmarshal(apiResp.Result, result); err != nil {
            return fmt.Errorf("failed to unmarshal result: %w", err)
        }
    }
    
    return nil
}

// calculateBackoff returns the backoff duration for the given attempt
func (e *Executor) calculateBackoff(attempt int) time.Duration {
    backoff := e.baseBackoff * time.Duration(1<<attempt) // Exponential
    if backoff > e.maxBackoff {
        backoff = e.maxBackoff
    }
    // Add jitter (±10%)
    jitter := time.Duration(rand.Int63n(int64(backoff) / 5))
    return backoff - (backoff / 10) + jitter
}

// CloseIdleConnections closes idle HTTP connections
func (e *Executor) CloseIdleConnections() {
    if transport, ok := e.httpClient.Transport.(*http.Transport); ok {
        transport.CloseIdleConnections()
    }
}
```

### 2.2 Migrate Existing Client

**File:** `sender/client.go` (update)

```go
// Update Client to use Executor internally
type Client struct {
    executor *Executor
    config   Config
    
    // Per-chat limiters
    chatLimiters sync.Map
    
    // Circuit breaker
    breaker *gobreaker.CircuitBreaker[*tg.APIResponse[json.RawMessage]]
    
    logger *slog.Logger
}

// NewFromConfig creates a new Client from configuration
func NewFromConfig(cfg Config, opts ...Option) (*Client, error) {
    // Validate config
    if cfg.Token.Value() == "" {
        return nil, tg.ErrInvalidToken
    }
    
    // Create executor
    executor := NewExecutor(ExecutorConfig{
        BaseURL:     cfg.BaseURL,
        Token:       cfg.Token.Value(),
        GlobalRPS:   cfg.GlobalRPS,
        MaxRetries:  cfg.MaxRetries,
        BaseBackoff: cfg.RetryBaseDelay,
        MaxBackoff:  cfg.RetryMaxDelay,
    })
    
    client := &Client{
        executor: executor,
        config:   cfg,
        logger:   slog.Default(),
    }
    
    // Apply options
    for _, opt := range opts {
        opt(client)
    }
    
    // Initialize circuit breaker
    client.breaker = gobreaker.NewCircuitBreaker[*tg.APIResponse[json.RawMessage]](
        gobreaker.Settings{
            Name:        "telegram-api",
            MaxRequests: 5,
            Interval:    10 * time.Second,
            Timeout:     30 * time.Second,
            ReadyToTrip: func(counts gobreaker.Counts) bool {
                return counts.ConsecutiveFailures > 5
            },
        },
    )
    
    return client, nil
}

// Call executes an API method (convenience wrapper)
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
    return c.executor.Call(ctx, method, params, result)
}

// Close releases resources held by the Client
func (c *Client) Close() error {
    c.executor.CloseIdleConnections()
    return nil
}
```

### Definition of Done (PR2)

- [ ] `Executor.Call()` works for all existing methods
- [ ] Retry logic respects `retry_after` from JSON parameters
- [ ] Uses `net/url` for URL construction (no string concat)
- [ ] Response size limit works correctly at boundary
- [ ] Exponential backoff with jitter for non-429 retries
- [ ] All existing tests still pass
- [ ] New tests: retry behavior, rate limiting, timeout handling

---

## PR3: InputFile & Multipart Uploads

**Goal:** Enable file uploads for all media methods.  
**Estimated Time:** 8-10 hours  
**Breaking Changes:** None (additive)

### 3.1 InputFile Type

**File:** `tg/inputfile.go` (new)

```go
package tg

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

// InputFile represents a file to be sent to Telegram.
// Use the constructor functions to create InputFile values.
type InputFile struct {
    // Only one of these should be set
    fileID   string
    url      string
    upload   *FileUpload
}

// FileUpload contains data for uploading a file
type FileUpload struct {
    Name   string    // Filename to use
    Reader io.Reader // File content
}

// FileID creates an InputFile from an existing Telegram file_id
func FileID(id string) InputFile {
    return InputFile{fileID: id}
}

// FileURL creates an InputFile from an HTTP(S) URL
func FileURL(url string) InputFile {
    return InputFile{url: url}
}

// FileUpload creates an InputFile for uploading from an io.Reader
func FileFromReader(name string, r io.Reader) InputFile {
    return InputFile{upload: &FileUpload{Name: name, Reader: r}}
}

// FileFromBytes creates an InputFile for uploading from a byte slice
func FileFromBytes(name string, data []byte) InputFile {
    return InputFile{upload: &FileUpload{Name: name, Reader: bytes.NewReader(data)}}
}

// FileFromPath creates an InputFile for uploading from a local file path.
// SECURITY: This function validates the path against allowed directories.
// If allowedDirs is empty, path traversal attacks are possible.
func FileFromPath(path string, allowedDirs []string) (InputFile, error) {
    // Clean and resolve to absolute path
    absPath, err := filepath.Abs(filepath.Clean(path))
    if err != nil {
        return InputFile{}, fmt.Errorf("invalid path: %w", err)
    }
    
    // Security: Check against allowed directories
    if len(allowedDirs) > 0 {
        allowed := false
        for _, dir := range allowedDirs {
            absDir, err := filepath.Abs(filepath.Clean(dir))
            if err != nil {
                continue
            }
            if strings.HasPrefix(absPath, absDir+string(filepath.Separator)) {
                allowed = true
                break
            }
        }
        if !allowed {
            return InputFile{}, fmt.Errorf("path not in allowed directories: %s", path)
        }
    }
    
    // Open file
    file, err := os.Open(absPath)
    if err != nil {
        return InputFile{}, fmt.Errorf("failed to open file: %w", err)
    }
    
    // Get filename
    name := filepath.Base(absPath)
    
    return InputFile{upload: &FileUpload{Name: name, Reader: file}}, nil
}

// IsUpload returns true if this InputFile requires multipart upload
func (f InputFile) IsUpload() bool {
    return f.upload != nil
}

// IsFileID returns true if this is a file_id reference
func (f InputFile) IsFileID() bool {
    return f.fileID != ""
}

// IsURL returns true if this is a URL reference
func (f InputFile) IsURL() bool {
    return f.url != ""
}

// MarshalJSON implements json.Marshaler
// For file_id and URL, returns the string directly
// For uploads, this should not be called (use multipart instead)
func (f InputFile) MarshalJSON() ([]byte, error) {
    if f.fileID != "" {
        return json.Marshal(f.fileID)
    }
    if f.url != "" {
        return json.Marshal(f.url)
    }
    if f.upload != nil {
        // This will be handled by multipart, but we need a placeholder
        // for JSON serialization in media groups
        return nil, fmt.Errorf("InputFile upload cannot be JSON serialized directly")
    }
    return json.Marshal(nil)
}

// GetUpload returns the upload data, or nil if not an upload
func (f InputFile) GetUpload() *FileUpload {
    return f.upload
}

// GetValue returns the string value (file_id or URL), or empty if upload
func (f InputFile) GetValue() string {
    if f.fileID != "" {
        return f.fileID
    }
    return f.url
}

// Close closes the underlying reader if it implements io.Closer
func (f InputFile) Close() error {
    if f.upload != nil && f.upload.Reader != nil {
        if closer, ok := f.upload.Reader.(io.Closer); ok {
            return closer.Close()
        }
    }
    return nil
}
```

### 3.2 Multipart Executor

**File:** `sender/multipart.go` (new)

```go
package sender

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "net/url"
    "reflect"
    "strings"
    
    "github.com/prilive-com/galigo/tg"
)

// FilePart represents a file to be uploaded in a multipart request
type FilePart struct {
    FieldName string
    FileName  string
    Reader    io.Reader
}

// CallMultipart executes a multipart/form-data API request
func (e *Executor) CallMultipart(ctx context.Context, method string, params any, files []FilePart, result any) error {
    // Apply rate limiting
    if err := e.globalLimiter.Wait(ctx); err != nil {
        return fmt.Errorf("rate limit wait: %w", err)
    }
    
    return e.doMultipartRequest(ctx, method, params, files, result)
}

// doMultipartRequest performs a single multipart request
func (e *Executor) doMultipartRequest(ctx context.Context, method string, params any, files []FilePart, result any) error {
    // Build URL
    apiURL, err := url.JoinPath(e.baseURL, "bot"+e.token, method)
    if err != nil {
        return fmt.Errorf("failed to build URL: %w", err)
    }
    
    // Create multipart body
    body, contentType, err := e.buildMultipartBody(params, files)
    if err != nil {
        return fmt.Errorf("failed to build multipart body: %w", err)
    }
    
    // Create request
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, body)
    if err != nil {
        return fmt.Errorf("failed to create request: %w", err)
    }
    
    req.Header.Set("Content-Type", contentType)
    req.Header.Set("Accept", "application/json")
    
    // Execute request
    resp, err := e.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Read and parse response (same as JSON request)
    limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
    respBody, err := io.ReadAll(limitedReader)
    if err != nil {
        return fmt.Errorf("failed to read response: %w", err)
    }
    
    if int64(len(respBody)) > maxResponseSize {
        return fmt.Errorf("response too large")
    }
    
    var apiResp tg.APIResponse[json.RawMessage]
    if err := json.Unmarshal(respBody, &apiResp); err != nil {
        return fmt.Errorf("failed to parse response: %w", err)
    }
    
    if !apiResp.OK {
        return apiResp.ToError(method)
    }
    
    if result != nil && len(apiResp.Result) > 0 {
        if err := json.Unmarshal(apiResp.Result, result); err != nil {
            return fmt.Errorf("failed to unmarshal result: %w", err)
        }
    }
    
    return nil
}

// buildMultipartBody creates the multipart request body
func (e *Executor) buildMultipartBody(params any, files []FilePart) (io.Reader, string, error) {
    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)
    
    // Add struct fields as form fields
    if params != nil {
        if err := e.addStructFields(writer, params); err != nil {
            return nil, "", err
        }
    }
    
    // Add file parts
    for _, file := range files {
        part, err := writer.CreateFormFile(file.FieldName, file.FileName)
        if err != nil {
            return nil, "", fmt.Errorf("failed to create form file %s: %w", file.FieldName, err)
        }
        
        if _, err := io.Copy(part, file.Reader); err != nil {
            return nil, "", fmt.Errorf("failed to write file %s: %w", file.FieldName, err)
        }
        
        // Close reader if it's a Closer
        if closer, ok := file.Reader.(io.Closer); ok {
            closer.Close()
        }
    }
    
    if err := writer.Close(); err != nil {
        return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
    }
    
    return &buf, writer.FormDataContentType(), nil
}

// addStructFields adds struct fields to the multipart writer
func (e *Executor) addStructFields(writer *multipart.Writer, params any) error {
    v := reflect.ValueOf(params)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    
    if v.Kind() != reflect.Struct {
        return fmt.Errorf("params must be a struct")
    }
    
    t := v.Type()
    
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)
        
        // Get JSON tag
        jsonTag := fieldType.Tag.Get("json")
        if jsonTag == "" || jsonTag == "-" {
            continue
        }
        
        // Parse tag
        tagParts := strings.Split(jsonTag, ",")
        fieldName := tagParts[0]
        omitempty := len(tagParts) > 1 && strings.Contains(jsonTag, "omitempty")
        
        // Skip zero values if omitempty
        if omitempty && field.IsZero() {
            continue
        }
        
        // Skip InputFile fields (handled separately)
        if _, ok := field.Interface().(tg.InputFile); ok {
            // If it's not an upload, add the value as a field
            inputFile := field.Interface().(tg.InputFile)
            if !inputFile.IsUpload() {
                if err := writer.WriteField(fieldName, inputFile.GetValue()); err != nil {
                    return err
                }
            }
            continue
        }
        
        // Convert value to string
        var strValue string
        switch fv := field.Interface().(type) {
        case string:
            strValue = fv
        case int, int64, float64, bool:
            strValue = fmt.Sprint(fv)
        default:
            // JSON encode complex types
            jsonBytes, err := json.Marshal(fv)
            if err != nil {
                return fmt.Errorf("failed to marshal field %s: %w", fieldName, err)
            }
            strValue = string(jsonBytes)
        }
        
        if err := writer.WriteField(fieldName, strValue); err != nil {
            return fmt.Errorf("failed to write field %s: %w", fieldName, err)
        }
    }
    
    return nil
}
```

### 3.3 Smart Request Helper

**File:** `sender/request_helper.go` (new)

```go
package sender

import (
    "context"
    
    "github.com/prilive-com/galigo/tg"
)

// executeRequest automatically chooses JSON or multipart based on InputFile fields
func (c *Client) executeRequest(ctx context.Context, method string, req any, result any) error {
    // Check if any field requires upload
    files := extractUploads(req)
    
    if len(files) > 0 {
        return c.executor.CallMultipart(ctx, method, req, files, result)
    }
    
    return c.executor.Call(ctx, method, req, result)
}

// extractUploads finds all InputFile fields that require upload
func extractUploads(req any) []FilePart {
    var files []FilePart
    
    v := reflect.ValueOf(req)
    if v.Kind() == reflect.Ptr {
        v = v.Elem()
    }
    
    if v.Kind() != reflect.Struct {
        return files
    }
    
    t := v.Type()
    
    for i := 0; i < v.NumField(); i++ {
        field := v.Field(i)
        fieldType := t.Field(i)
        
        // Get field name from JSON tag
        jsonTag := fieldType.Tag.Get("json")
        if jsonTag == "" || jsonTag == "-" {
            continue
        }
        tagParts := strings.Split(jsonTag, ",")
        fieldName := tagParts[0]
        
        // Check for InputFile
        if inputFile, ok := field.Interface().(tg.InputFile); ok {
            if inputFile.IsUpload() {
                upload := inputFile.GetUpload()
                files = append(files, FilePart{
                    FieldName: fieldName,
                    FileName:  upload.Name,
                    Reader:    upload.Reader,
                })
            }
        }
    }
    
    return files
}
```

### Definition of Done (PR3)

- [ ] `tg.FileID()`, `tg.FileURL()`, `tg.FileFromReader()` work correctly
- [ ] `tg.FileFromPath()` validates against allowed directories
- [ ] Multipart uploads stream file content (no full buffering)
- [ ] `sendPhoto` works with file_id, URL, and upload
- [ ] Tests: upload, file_id, URL, path security
- [ ] Existing tests still pass

---

## PR4: Core Methods (getMe, getFile, Download)

**Goal:** Essential bot identity and file download methods.  
**Estimated Time:** 3-4 hours  
**Breaking Changes:** None (additive)

### 4.1 Implementation

**File:** `sender/methods_core.go` (new)

```go
package sender

import (
    "context"
    "fmt"
    "io"
    "net/http"
    
    "github.com/prilive-com/galigo/tg"
)

// GetMe returns basic information about the bot.
// Use this method to test your bot's authentication token.
//
// https://core.telegram.org/bots/api#getme
func (c *Client) GetMe(ctx context.Context) (*tg.User, error) {
    var user tg.User
    if err := c.executor.Call(ctx, "getMe", nil, &user); err != nil {
        return nil, err
    }
    return &user, nil
}

// LogOut logs out from the cloud Bot API server.
// You must log out the bot before moving it from one local server to another.
//
// https://core.telegram.org/bots/api#logout
func (c *Client) LogOut(ctx context.Context) error {
    return c.executor.Call(ctx, "logOut", nil, nil)
}

// Close closes the bot instance before moving it from one local server to another.
// You need to delete the webhook before calling this method.
//
// https://core.telegram.org/bots/api#close
func (c *Client) Close(ctx context.Context) error {
    return c.executor.Call(ctx, "close", nil, nil)
}

// GetFile retrieves basic information about a file and prepares it for download.
// The file can then be downloaded using DownloadFile or FileDownloadURL.
//
// Note: The download link is valid for at least 1 hour.
// Maximum file size: 20 MB.
//
// https://core.telegram.org/bots/api#getfile
func (c *Client) GetFile(ctx context.Context, fileID string) (*tg.File, error) {
    if fileID == "" {
        return nil, tg.NewValidationError("file_id", "cannot be empty")
    }
    
    params := map[string]string{"file_id": fileID}
    
    var file tg.File
    if err := c.executor.Call(ctx, "getFile", params, &file); err != nil {
        return nil, err
    }
    return &file, nil
}

// FileDownloadURL returns the URL to download a file.
// Call GetFile first to obtain the file_path.
//
// URL format: https://api.telegram.org/file/bot<token>/<file_path>
func (c *Client) FileDownloadURL(filePath string) string {
    return fmt.Sprintf("%s/file/bot%s/%s",
        c.config.BaseURL,
        c.config.Token.Value(),
        filePath,
    )
}

// DownloadFile downloads a file to the provided writer.
// This is a convenience method that combines GetFile and HTTP download.
//
// The maximum download size is 20 MB (Telegram limit).
func (c *Client) DownloadFile(ctx context.Context, fileID string, w io.Writer) error {
    // Get file info
    file, err := c.GetFile(ctx, fileID)
    if err != nil {
        return fmt.Errorf("failed to get file info: %w", err)
    }
    
    if file.FilePath == "" {
        return fmt.Errorf("file path is empty")
    }
    
    return c.DownloadFilePath(ctx, file.FilePath, w)
}

// DownloadFilePath downloads a file by its path to the provided writer.
// Use this if you already have the file_path from a previous GetFile call.
func (c *Client) DownloadFilePath(ctx context.Context, filePath string, w io.Writer) error {
    downloadURL := c.FileDownloadURL(filePath)
    
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
    if err != nil {
        return fmt.Errorf("failed to create download request: %w", err)
    }
    
    resp, err := c.executor.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("download request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("download failed with status: %d %s", resp.StatusCode, resp.Status)
    }
    
    // Telegram file download limit
    const maxDownloadSize = 20 << 20 // 20 MB
    
    limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)
    n, err := io.Copy(w, limitedReader)
    if err != nil {
        return fmt.Errorf("failed to download file: %w", err)
    }
    
    if n > maxDownloadSize {
        return fmt.Errorf("file exceeds maximum download size of 20 MB")
    }
    
    return nil
}
```

### 4.2 Tests

**File:** `sender/methods_core_test.go`

```go
func TestClient_GetMe(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.Path, "/getMe")
        
        json.NewEncoder(w).Encode(tg.APIResponse[tg.User]{
            OK: true,
            Result: tg.User{
                ID:        123456789,
                IsBot:     true,
                FirstName: "Test Bot",
                Username:  "test_bot",
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    user, err := client.GetMe(context.Background())
    require.NoError(t, err)
    assert.Equal(t, int64(123456789), user.ID)
    assert.True(t, user.IsBot)
}

func TestClient_GetFile(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(tg.APIResponse[tg.File]{
            OK: true,
            Result: tg.File{
                FileID:       "AgACAgIAAxkB...",
                FileUniqueID: "AQADAgAT...",
                FileSize:     1024,
                FilePath:     "photos/file_0.jpg",
            },
        })
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    file, err := client.GetFile(context.Background(), "AgACAgIAAxkB...")
    require.NoError(t, err)
    assert.Equal(t, "photos/file_0.jpg", file.FilePath)
}

func TestClient_DownloadFile(t *testing.T) {
    fileContent := []byte("test file content")
    
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if strings.Contains(r.URL.Path, "/getFile") {
            json.NewEncoder(w).Encode(tg.APIResponse[tg.File]{
                OK: true,
                Result: tg.File{
                    FileID:   "test_file_id",
                    FilePath: "test/file.txt",
                },
            })
            return
        }
        
        if strings.Contains(r.URL.Path, "/file/") {
            w.Write(fileContent)
            return
        }
    }))
    defer server.Close()
    
    client := newTestClient(t, server.URL)
    
    var buf bytes.Buffer
    err := client.DownloadFile(context.Background(), "test_file_id", &buf)
    require.NoError(t, err)
    assert.Equal(t, fileContent, buf.Bytes())
}
```

### Definition of Done (PR4)

- [ ] `GetMe()` returns bot user info
- [ ] `LogOut()` and `Close()` work
- [ ] `GetFile()` returns file info with path
- [ ] `DownloadFile()` downloads to io.Writer
- [ ] `FileDownloadURL()` returns correct URL
- [ ] Download respects 20MB limit
- [ ] All tests pass

---

## PR5: Media Senders

**Goal:** All core media sending methods.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

### Methods to Implement

| Method | Supports Upload | Notes |
|--------|-----------------|-------|
| `sendDocument` | Yes | Most common |
| `sendVideo` | Yes | + streaming, spoiler |
| `sendAudio` | Yes | + performer, title |
| `sendVoice` | Yes | OGG/OPUS only |
| `sendAnimation` | Yes | GIFs |
| `sendVideoNote` | Yes | Round videos |
| `sendSticker` | Yes | + emoji |

### Common Pattern

Each method follows the same pattern:

```go
// SendDocumentRequest contains parameters for sendDocument
type SendDocumentRequest struct {
    ChatID                      tg.ChatID           `json:"chat_id"`
    Document                    tg.InputFile        `json:"document"`
    MessageThreadID             int                 `json:"message_thread_id,omitempty"`
    BusinessConnectionID        string              `json:"business_connection_id,omitempty"`
    Thumbnail                   tg.InputFile        `json:"thumbnail,omitempty"`
    Caption                     string              `json:"caption,omitempty"`
    ParseMode                   tg.ParseMode        `json:"parse_mode,omitempty"`
    CaptionEntities             []tg.MessageEntity  `json:"caption_entities,omitempty"`
    DisableContentTypeDetection bool                `json:"disable_content_type_detection,omitempty"`
    DisableNotification         bool                `json:"disable_notification,omitempty"`
    ProtectContent              bool                `json:"protect_content,omitempty"`
    MessageEffectID             string              `json:"message_effect_id,omitempty"`
    ReplyParameters             *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup                 any                 `json:"reply_markup,omitempty"`
}

func (c *Client) SendDocument(ctx context.Context, req SendDocumentRequest) (*tg.Message, error) {
    if err := validateSendDocument(req); err != nil {
        return nil, err
    }
    
    var msg tg.Message
    if err := c.executeRequest(ctx, "sendDocument", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}

func validateSendDocument(req SendDocumentRequest) error {
    if req.ChatID == nil {
        return tg.NewValidationError("chat_id", "is required")
    }
    if req.Document == (tg.InputFile{}) {
        return tg.NewValidationError("document", "is required")
    }
    if len(req.Caption) > 1024 {
        return tg.NewValidationError("caption", "must be at most 1024 characters")
    }
    return nil
}
```

### Definition of Done (PR5)

- [ ] All 7 media senders implemented
- [ ] Each supports file_id, URL, and upload
- [ ] Validation for required fields and limits
- [ ] Tests for JSON mode (file_id)
- [ ] Tests for multipart mode (upload)
- [ ] Documentation with examples

---

## PR6: sendMediaGroup, sendChatAction, editMessageMedia

**Goal:** Album sending, typing indicator, media editing.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** None (additive)

### 6.1 sendMediaGroup

This is the most complex method due to:
- Multiple InputMedia items
- Each can be file_id, URL, or upload
- Uploads use `attach://` references

```go
// SendMediaGroupRequest contains parameters for sendMediaGroup
type SendMediaGroupRequest struct {
    ChatID               tg.ChatID        `json:"chat_id"`
    Media                []tg.InputMedia  `json:"media"`
    MessageThreadID      int              `json:"message_thread_id,omitempty"`
    BusinessConnectionID string           `json:"business_connection_id,omitempty"`
    DisableNotification  bool             `json:"disable_notification,omitempty"`
    ProtectContent       bool             `json:"protect_content,omitempty"`
    MessageEffectID      string           `json:"message_effect_id,omitempty"`
    ReplyParameters      *tg.ReplyParameters `json:"reply_parameters,omitempty"`
}

func (c *Client) SendMediaGroup(ctx context.Context, req SendMediaGroupRequest) ([]tg.Message, error) {
    if err := validateSendMediaGroup(req); err != nil {
        return nil, err
    }
    
    // Check if any media requires upload
    hasUploads := false
    for _, m := range req.Media {
        if m.HasUpload() {
            hasUploads = true
            break
        }
    }
    
    var messages []tg.Message
    
    if hasUploads {
        // Build multipart with attach:// references
        files, mediaJSON := prepareMediaGroupUploads(req.Media)
        
        // Replace media with JSON-serializable version
        params := map[string]any{
            "chat_id": req.ChatID,
            "media":   mediaJSON,
        }
        // Add other params...
        
        if err := c.executor.CallMultipart(ctx, "sendMediaGroup", params, files, &messages); err != nil {
            return nil, err
        }
    } else {
        if err := c.executor.Call(ctx, "sendMediaGroup", req, &messages); err != nil {
            return nil, err
        }
    }
    
    return messages, nil
}
```

### 6.2 sendChatAction

```go
type ChatAction string

const (
    ActionTyping          ChatAction = "typing"
    ActionUploadPhoto     ChatAction = "upload_photo"
    ActionRecordVideo     ChatAction = "record_video"
    ActionUploadVideo     ChatAction = "upload_video"
    ActionRecordVoice     ChatAction = "record_voice"
    ActionUploadVoice     ChatAction = "upload_voice"
    ActionUploadDocument  ChatAction = "upload_document"
    ActionChooseSticker   ChatAction = "choose_sticker"
    ActionFindLocation    ChatAction = "find_location"
    ActionRecordVideoNote ChatAction = "record_video_note"
    ActionUploadVideoNote ChatAction = "upload_video_note"
)

type SendChatActionRequest struct {
    ChatID               tg.ChatID  `json:"chat_id"`
    Action               ChatAction `json:"action"`
    MessageThreadID      int        `json:"message_thread_id,omitempty"`
    BusinessConnectionID string     `json:"business_connection_id,omitempty"`
}

func (c *Client) SendChatAction(ctx context.Context, chatID tg.ChatID, action ChatAction, opts ...ChatActionOption) error {
    req := SendChatActionRequest{
        ChatID: chatID,
        Action: action,
    }
    for _, opt := range opts {
        opt(&req)
    }
    return c.executor.Call(ctx, "sendChatAction", req, nil)
}
```

### 6.3 editMessageMedia

```go
type EditMessageMediaRequest struct {
    ChatID               tg.ChatID       `json:"chat_id,omitempty"`
    MessageID            int             `json:"message_id,omitempty"`
    InlineMessageID      string          `json:"inline_message_id,omitempty"`
    Media                tg.InputMedia   `json:"media"`
    BusinessConnectionID string          `json:"business_connection_id,omitempty"`
    ReplyMarkup          any             `json:"reply_markup,omitempty"`
}

func (c *Client) EditMessageMedia(ctx context.Context, req EditMessageMediaRequest) (*tg.Message, error) {
    // Validate: either (chat_id + message_id) or inline_message_id
    if req.InlineMessageID == "" {
        if req.ChatID == nil {
            return nil, tg.NewValidationError("chat_id", "required when inline_message_id is not set")
        }
        if req.MessageID == 0 {
            return nil, tg.NewValidationError("message_id", "required when inline_message_id is not set")
        }
    }
    
    var msg tg.Message
    if err := c.executeRequest(ctx, "editMessageMedia", req, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}
```

### Definition of Done (PR6)

- [ ] `sendMediaGroup` with 2-10 items
- [ ] Mixed uploads work (some file_id, some upload)
- [ ] `attach://` references generated correctly
- [ ] `sendChatAction` with all action types
- [ ] `sendChatAction` supports `message_thread_id`
- [ ] `editMessageMedia` works for chat and inline messages
- [ ] Tests for all scenarios

---

## PR7: sendMessageDraft (Bot API 9.3)

**Goal:** Support streaming message generation (AI bots).  
**Estimated Time:** 2-3 hours  
**Breaking Changes:** None (additive)

```go
// SendMessageDraftRequest contains parameters for sendMessageDraft
// This method is used to stream partial messages while generating content.
// Added in Bot API 9.3.
type SendMessageDraftRequest struct {
    BusinessConnectionID string             `json:"business_connection_id"`
    ChatID               tg.ChatID          `json:"chat_id"`
    DraftMessageID       int                `json:"draft_message_id,omitempty"`
    Text                 string             `json:"text"`
    ParseMode            tg.ParseMode       `json:"parse_mode,omitempty"`
    Entities             []tg.MessageEntity `json:"entities,omitempty"`
    LinkPreviewOptions   *tg.LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

type SendMessageDraftResult struct {
    DraftMessageID int `json:"draft_message_id"`
}

// SendMessageDraft sends or updates a draft message.
// Use this for streaming content generation (like ChatGPT-style streaming).
//
// Flow:
// 1. First call: omit draft_message_id → returns new draft_message_id
// 2. Subsequent calls: include draft_message_id → updates existing draft
// 3. When complete: call sendMessage with the final content
//
// https://core.telegram.org/bots/api#sendmessagedraft
func (c *Client) SendMessageDraft(ctx context.Context, req SendMessageDraftRequest) (*SendMessageDraftResult, error) {
    if req.BusinessConnectionID == "" {
        return nil, tg.NewValidationError("business_connection_id", "is required")
    }
    if req.ChatID == nil {
        return nil, tg.NewValidationError("chat_id", "is required")
    }
    
    var result SendMessageDraftResult
    if err := c.executor.Call(ctx, "sendMessageDraft", req, &result); err != nil {
        return nil, err
    }
    return &result, nil
}
```

### Definition of Done (PR7)

- [ ] `sendMessageDraft` implemented with all parameters
- [ ] Documentation explains streaming workflow
- [ ] Tests for create and update flows

---

## PR8: Modern Parameters Sweep

**Goal:** Add Bot API 9.x parameters to all relevant methods.  
**Estimated Time:** 4-6 hours  
**Breaking Changes:** None (additive fields)

### Parameters to Add

| Parameter | Added | Methods Affected |
|-----------|-------|------------------|
| `message_thread_id` | 6.3 | All send/edit |
| `business_connection_id` | 7.2 | All send/edit |
| `message_effect_id` | 7.9 | send/copy/forward |
| `show_caption_above_media` | 7.9 | Photo/video/animation |
| `allow_paid_broadcast` | 8.1 | sendMessage |

### Shared Options Types

```go
// MessageOptions contains common options for sending messages
type MessageOptions struct {
    MessageThreadID      int    `json:"message_thread_id,omitempty"`
    BusinessConnectionID string `json:"business_connection_id,omitempty"`
    DisableNotification  bool   `json:"disable_notification,omitempty"`
    ProtectContent       bool   `json:"protect_content,omitempty"`
    MessageEffectID      string `json:"message_effect_id,omitempty"`
    AllowPaidBroadcast   bool   `json:"allow_paid_broadcast,omitempty"`
}

// ReplyOptions contains reply-related options
type ReplyOptions struct {
    ReplyParameters *tg.ReplyParameters `json:"reply_parameters,omitempty"`
    ReplyMarkup     any                 `json:"reply_markup,omitempty"`
}
```

### Definition of Done (PR8)

- [ ] All Tier-1 methods support modern parameters
- [ ] Parameters correctly serialized
- [ ] Tests verify parameter presence in requests
- [ ] Documentation updated

---

## PR9: ChatID & int64 Hardening

**Goal:** Type-safe ChatID, no int32 overflow risks.  
**Estimated Time:** 6-8 hours  
**Breaking Changes:** YES - ChatID type change

### 9.1 New ChatID Type

```go
// ChatID represents a Telegram chat identifier.
// It can be either a numeric ID (int64) or a username (string like "@username").
type ChatID struct {
    id       *int64
    username *string
}

// ChatIDFromInt64 creates a ChatID from a numeric ID
func ChatIDFromInt64(id int64) ChatID {
    return ChatID{id: &id}
}

// ChatIDFromUsername creates a ChatID from a username
// The username should include the @ prefix
func ChatIDFromUsername(username string) ChatID {
    if !strings.HasPrefix(username, "@") {
        username = "@" + username
    }
    return ChatID{username: &username}
}

// Int64 returns the numeric ID, or 0 if this is a username
func (c ChatID) Int64() int64 {
    if c.id != nil {
        return *c.id
    }
    return 0
}

// Username returns the username, or empty if this is a numeric ID
func (c ChatID) Username() string {
    if c.username != nil {
        return *c.username
    }
    return ""
}

// IsNumeric returns true if this is a numeric ID
func (c ChatID) IsNumeric() bool {
    return c.id != nil
}

// IsUsername returns true if this is a username
func (c ChatID) IsUsername() bool {
    return c.username != nil
}

// IsZero returns true if this ChatID is not set
func (c ChatID) IsZero() bool {
    return c.id == nil && c.username == nil
}

// MarshalJSON implements json.Marshaler
func (c ChatID) MarshalJSON() ([]byte, error) {
    if c.id != nil {
        return json.Marshal(*c.id)
    }
    if c.username != nil {
        return json.Marshal(*c.username)
    }
    return json.Marshal(nil)
}

// UnmarshalJSON implements json.Unmarshaler
func (c *ChatID) UnmarshalJSON(data []byte) error {
    // Try int64 first
    var id int64
    if err := json.Unmarshal(data, &id); err == nil {
        c.id = &id
        return nil
    }
    
    // Try string
    var username string
    if err := json.Unmarshal(data, &username); err == nil {
        c.username = &username
        return nil
    }
    
    return fmt.Errorf("invalid ChatID: %s", string(data))
}

// String implements fmt.Stringer
func (c ChatID) String() string {
    if c.id != nil {
        return strconv.FormatInt(*c.id, 10)
    }
    if c.username != nil {
        return *c.username
    }
    return ""
}
```

### 9.2 Per-Chat Rate Limiter Fix

```go
// getChatLimiter returns the rate limiter for a chat
// Now correctly handles username-based chat IDs
func (c *Client) getChatLimiter(chatID tg.ChatID) *rate.Limiter {
    key := chatID.String() // Works for both numeric and username
    
    if limiter, ok := c.chatLimiters.Load(key); ok {
        return limiter.(*rate.Limiter)
    }
    
    limiter := rate.NewLimiter(rate.Limit(c.config.PerChatRPS), c.config.PerChatBurst)
    actual, _ := c.chatLimiters.LoadOrStore(key, limiter)
    return actual.(*rate.Limiter)
}
```

### 9.3 Migration Guide

```go
// BEFORE (v1.x):
bot.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: 123456789,  // any type
    Text:   "Hello",
})

// AFTER (v2.0):
bot.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: tg.ChatIDFromInt64(123456789),  // explicit constructor
    Text:   "Hello",
})

// Or with username:
bot.SendMessage(ctx, sender.SendMessageRequest{
    ChatID: tg.ChatIDFromUsername("@mychannel"),
    Text:   "Hello",
})
```

### Definition of Done (PR9)

- [ ] `ChatID` type with int64 and username support
- [ ] JSON marshaling works correctly
- [ ] Per-chat limiter uses correct keys
- [ ] All existing tests updated
- [ ] Migration guide documented

---

## PR10: Facade, Docs & Release

**Goal:** Complete galigo.Bot facade, update docs, release v2.0.0.  
**Estimated Time:** 4-6 hours  
**Breaking Changes:** Documented in CHANGELOG

### 10.1 Bot Facade Updates

```go
// Add to bot.go

// GetMe returns basic information about the bot
func (b *Bot) GetMe(ctx context.Context) (*tg.User, error) {
    return b.sender.GetMe(ctx)
}

// GetFile retrieves file info for downloading
func (b *Bot) GetFile(ctx context.Context, fileID string) (*tg.File, error) {
    return b.sender.GetFile(ctx, fileID)
}

// DownloadFile downloads a file to the writer
func (b *Bot) DownloadFile(ctx context.Context, fileID string, w io.Writer) error {
    return b.sender.DownloadFile(ctx, fileID, w)
}

// SendDocument sends a document
func (b *Bot) SendDocument(ctx context.Context, chatID tg.ChatID, doc tg.InputFile, opts ...sender.DocumentOption) (*tg.Message, error) {
    req := sender.SendDocumentRequest{
        ChatID:   chatID,
        Document: doc,
    }
    for _, opt := range opts {
        opt(&req)
    }
    return b.sender.SendDocument(ctx, req)
}

// ... etc for all new methods
```

### 10.2 CHANGELOG.md

```markdown
# Changelog

## [2.0.0] - 2026-XX-XX

### Breaking Changes

- `ChatID` is now a struct type instead of `any`. Use `tg.ChatIDFromInt64()` or `tg.ChatIDFromUsername()`.
- Error types consolidated into `tg.APIError`. Use `errors.As()` for type assertions.
- `sender.APIError` deprecated in favor of `tg.APIError`.

### Added

- Generic request executor with proper retry handling
- `retry_after` parsed from JSON response parameters (not HTTP headers)
- File upload support via `tg.InputFile` types
- `sendDocument`, `sendVideo`, `sendAudio`, `sendVoice`, `sendAnimation`, `sendVideoNote`, `sendSticker`
- `sendMediaGroup` with mixed uploads
- `sendChatAction` with topic support
- `editMessageMedia`
- `sendMessageDraft` (Bot API 9.3)
- `getMe`, `getFile`, `DownloadFile`
- Support for `message_thread_id`, `business_connection_id`, `message_effect_id`
- `ChatID` type with proper int64 and username support

### Fixed

- Response size limit boundary condition
- Per-chat rate limiter now works correctly with usernames
- Multipart uploads properly stream file content

### Security

- File path uploads now validate against allowed directories
```

### 10.3 Version Strategy

| Change Type | Version |
|-------------|---------|
| ChatID breaking change | v2.0.0 |
| If backwards-compatible shim provided | v1.1.0 (deprecated `any` ChatID) |

### Definition of Done (PR10)

- [ ] All facade methods added
- [ ] CHANGELOG.md complete
- [ ] README.md updated with new examples
- [ ] GoDoc complete for all exports
- [ ] `go doc ./...` produces clean output
- [ ] Tag v2.0.0 created

---

## Summary: Complete PR Checklist

| PR | Title | Est. Hours | Breaking |
|----|-------|------------|----------|
| PR0 | CI & Dependencies | 2-4 | No |
| PR1 | Response & Error Model | 4-6 | Minor |
| PR2 | JSON Executor | 6-8 | No |
| PR3 | InputFile & Multipart | 8-10 | No |
| PR4 | getMe, getFile, Download | 3-4 | No |
| PR5 | Media Senders | 6-8 | No |
| PR6 | MediaGroup, ChatAction, EditMedia | 6-8 | No |
| PR7 | sendMessageDraft | 2-3 | No |
| PR8 | Modern Parameters | 4-6 | No |
| PR9 | ChatID Hardening | 6-8 | **YES** |
| PR10 | Facade & Release | 4-6 | Documented |
| **TOTAL** | | **52-71 hours** | |

**Recommended Timeline:** 4-5 weeks with 2-3 PRs per week.

---

## References

- [Telegram Bot API 9.3](https://core.telegram.org/bots/api)
- [Bot API Changelog](https://core.telegram.org/bots/api-changelog)
- [Go Toolchains](https://go.dev/doc/toolchain)
- [pyTelegramBotAPI 429 handling](https://github.com/eternnoir/pyTelegramBotAPI/issues/253)

---

*Consolidated Plan v2.0 - January 2026*
*Combines analyses from multiple independent reviews*