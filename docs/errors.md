# Error Handling Guide

galigo provides typed sentinel errors that you can check with `errors.Is()`. This guide explains what each error means and what action you should take.

## Sentinel Errors

All errors are defined in the `tg` package and re-exported in `sender` for convenience.

```go
import "github.com/prilive-com/galigo/tg"

if errors.Is(err, tg.ErrBotBlocked) {
    // Handle blocked user
}
```

## Error Reference

| Error | Meaning | Recommended Action |
|-------|---------|-------------------|
| `ErrBotBlocked` | User blocked the bot | Mark user as inactive, stop messaging them |
| `ErrBotKicked` | Bot was kicked from group | Mark group as inactive, stop messaging |
| `ErrChatNotFound` | Chat doesn't exist or bot was never a member | Remove from active roster, log for investigation |
| `ErrUserNotFound` | User doesn't exist | Remove from active roster |
| `ErrMessageNotFound` | Message to edit/delete doesn't exist | Update local state, continue (message was already deleted) |
| `ErrMessageNotModified` | Edit would result in identical content | Swallow silently (no-op), debug log only |
| `ErrTooManyRequests` | Telegram rate limit hit (429) | galigo handles retry automatically; track frequency for alerting |
| `ErrUnauthorized` | Invalid or revoked bot token (401) | **Critical alert** — rotate token immediately |
| `ErrForbidden` | Bot lacks required permissions (403) | Log, skip action, verify bot permissions in chat settings |
| `ErrNotFound` | Generic resource not found (404) | Log, investigate what resource is missing |
| `ErrCircuitOpen` | Circuit breaker is open | Service degraded — galigo will recover automatically after 30s |
| `ErrMaxRetries` | All retry attempts exhausted | Log failure, increment error counter, alert if sustained |
| `ErrRateLimited` | Local rate limiter blocked request | Request queued — wait for next available slot |

## Error Handling Patterns

### Basic Pattern

```go
msg, err := client.SendMessage(ctx, req)
if err != nil {
    switch {
    case errors.Is(err, tg.ErrBotBlocked):
        // User blocked us — mark inactive
        userRepo.SetInactive(ctx, userID)
        return nil // Not an error from our perspective

    case errors.Is(err, tg.ErrTooManyRequests):
        // galigo already retried — this means we're still rate-limited
        metrics.Increment("telegram.rate_limited")
        return err

    case errors.Is(err, tg.ErrCircuitOpen):
        // Telegram API is having issues
        metrics.Increment("telegram.circuit_open")
        return ErrServiceDegraded

    default:
        return fmt.Errorf("send message: %w", err)
    }
}
```

### Extracting APIError Details

```go
var apiErr *tg.APIError
if errors.As(err, &apiErr) {
    log.Error("telegram API error",
        "code", apiErr.Code,
        "description", apiErr.Description,
        "method", apiErr.Method,
        "retry_after", apiErr.RetryAfter,
    )
}
```

### Checking Retryability

```go
var apiErr *tg.APIError
if errors.As(err, &apiErr) && apiErr.IsRetryable() {
    // galigo already retried, but you could queue for later
    queue.Enqueue(message)
}
```

## Alert Severity Guide

| Error | Severity | Alert? |
|-------|----------|--------|
| `ErrBotBlocked` | Info | No — expected user behavior |
| `ErrBotKicked` | Info | No — expected group behavior |
| `ErrMessageNotModified` | Debug | No — benign race condition |
| `ErrTooManyRequests` | Warning | Yes, if > 100/min sustained |
| `ErrCircuitOpen` | Warning | Yes, if open > 1 minute |
| `ErrUnauthorized` | **Critical** | **Yes, immediately** — token compromised or revoked |
| `ErrMaxRetries` | Error | Yes, if > 5% of sends for 5 minutes |

## Common Mistakes

### Don't Retry 4xx Errors

```go
// WRONG — infinite loop
for {
    _, err := client.SendMessage(ctx, req)
    if err == nil {
        break
    }
    time.Sleep(time.Second)
}

// RIGHT — galigo already retries retryable errors
msg, err := client.SendMessage(ctx, req)
if err != nil {
    // If we get here, the error is not retryable
    return err
}
```

### Don't Ignore ErrBotBlocked

```go
// WRONG — keeps trying to message blocked users
_, err := client.SendMessage(ctx, req)
if err != nil {
    log.Error("failed to send", "error", err)
}

// RIGHT — stop messaging blocked users
if errors.Is(err, tg.ErrBotBlocked) {
    userRepo.SetBlocked(ctx, userID)
    return nil
}
```

### Don't Alert on Expected Errors

```go
// WRONG — alerts on normal user behavior
if err != nil {
    alerting.SendPagerDuty("telegram error", err)
}

// RIGHT — only alert on actionable errors
switch {
case errors.Is(err, tg.ErrUnauthorized):
    alerting.SendPagerDuty("TOKEN REVOKED", err)
case errors.Is(err, tg.ErrCircuitOpen):
    alerting.SendSlack("circuit breaker open", err)
case errors.Is(err, tg.ErrBotBlocked):
    // Normal — user blocked us, not an alert
}
```

## Error Wrapping

galigo errors support `errors.Is()` and `errors.As()` through the standard Go error chain:

```go
// APIError wraps a sentinel for Is() checks
err := &tg.APIError{
    Code:        403,
    Description: "Forbidden: bot was blocked by the user",
    cause:       tg.ErrBotBlocked,  // internal
}

// Both work:
errors.Is(err, tg.ErrBotBlocked)  // true
errors.As(err, &apiErr)           // true, extracts details
```
