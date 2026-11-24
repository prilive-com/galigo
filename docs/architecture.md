# How galigo Works

galigo is not a passive library — it runs background goroutines and manages internal state. This document explains the operational model you need to understand for correct usage.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         galigo.Bot                              │
│                                                                 │
│  ┌──────────────────────┐      ┌──────────────────────────┐    │
│  │receiver.PollingClient│      │     sender.Client        │    │
│  │  ┌────────────────┐  │      │  ┌────────────────────┐  │    │
│  │  │ Poll goroutine │──┼──────┼─►│ Circuit Breaker    │  │    │
│  │  └───────┬────────┘  │      │  └────────────────────┘  │    │
│  │          │           │      │  ┌────────────────────┐  │    │
│  │          ▼           │      │  │ Global Rate Limit  │  │    │
│  │  ┌────────────────┐  │      │  └────────────────────┘  │    │
│  │  │ Update Channel │  │      │  ┌────────────────────┐  │    │
│  │  │  (buffered)    │  │      │  │ Per-Chat Limiters  │  │    │
│  │  └───────┬────────┘  │      │  └────────────────────┘  │    │
│  └──────────┼───────────┘      │  ┌────────────────────┐  │    │
│             │                   │  │ Retry with Backoff │  │    │
│             ▼                   │  └────────────────────┘  │    │
│     bot.Updates() <──────      └──────────────────────────┘    │
│                                            │                    │
└────────────────────────────────────────────┼────────────────────┘
                                             │
                                             ▼
                                    Telegram Bot API
```

## Goroutine Lifecycle

galigo spawns background goroutines for:

1. **Polling loop** — Continuously fetches updates from Telegram
2. **Rate limiter timers** — Manages token bucket refills
3. **Circuit breaker state** — Tracks failure rates and recovery

### Starting the Bot

```go
bot, err := galigo.New(token, galigo.WithPolling(30, 100))
if err != nil {
    log.Fatal(err)
}

// Start MUST complete before consuming updates
if err := bot.Start(ctx); err != nil {
    log.Fatal(err)
}

// Now safe to consume
for update := range bot.Updates() {
    // ...
}
```

### Stopping the Bot

```go
// Close stops polling and waits for goroutines to finish
if err := bot.Close(); err != nil {
    log.Error("close failed", "error", err)
}
```

**Important:** Always call `Close()`. In polling mode it stops the poller and waits for it to exit. It does not
cancel a `getUpdates` request in flight: cancel the context you passed to `Start` to end that request at once (else
the wait can last up to the HTTP client timeout, by default the long-poll timeout plus 10 s). With
`DeliveryPolicyBlock` (the only policy `galigo.Bot` uses), a stop ends a wait for channel space: that update and the
rest of the fetched batch are not delivered, and the poller does not move its offset for them. The other policies can
still send or drop updates while the poller stops. Telegram confirms an update when it receives a `getUpdates` request
with a higher offset, so after a restart, unconfirmed updates that Telegram still holds can come again, also ones that
galigo already delivered or dropped (Telegram keeps updates for at most 24 hours). Updates already in the channel stay
readable: in polling mode `Close()` closes the channel, so a `for range bot.Updates()` loop reads them and then ends.
In webhook mode `Close()` does not close the channel, so such a loop does not end by itself: stop your HTTP server
first, then stop the loop.

## Channel-Based Delivery

Updates arrive via a buffered channel:

```go
updates := bot.Updates()  // <-chan tg.Update
```

### Backpressure

| Parameter | Default | Purpose |
|-----------|---------|---------|
| Channel buffer | 100 | Matches polling batch size |
| Polling limit | 100 | Updates per API request |

If your application consumes updates slower than they arrive, the channel will fill up. When full:

- **Polling mode**: the delivery policy decides (see [Delivery Policies](#delivery-policies)). With the default,
  `DeliveryPolicyBlock`, the poller waits up to `UpdateDeliveryTimeout` (default 5 s; 0 = no limit) for space. If the
  time runs out, it drops the update (reason `delivery_timeout`) and moves its local offset past it; the next
  `getUpdates` request confirms the drop to Telegram.
- **Webhook mode**: the same policies apply. When the handler drops an update, it answers 200, and Telegram does not
  send it again. With `DeliveryPolicyBlock`, the handler waits for space until the timeout or until the request ends.
  If the request ends first (for example, Telegram stops waiting), the update is not delivered and no drop is reported;
  Telegram may send it again.

### Recommended Consumer Pattern

```go
// Bounded worker pool — prevents goroutine explosion
workers := make(chan struct{}, 10)  // Max 10 concurrent handlers

for update := range bot.Updates() {
    workers <- struct{}{}  // Acquire slot
    go func(u tg.Update) {
        defer func() { <-workers }()  // Release slot
        handleUpdate(ctx, u)
    }(update)
}
```

## Delivery Policies

galigo supports three delivery policies when the update channel is full:

| Policy | Behavior |
|--------|----------|
| `DeliveryPolicyBlock` (default) | Wait for space up to `UpdateDeliveryTimeout` (default 5 s; 0 = no limit), then drop the update |
| `DeliveryPolicyDropNewest` | Drop incoming update, continue |
| `DeliveryPolicyDropOldest` | Drop oldest update in buffer, accept new |

These settings are in `receiver.Config`. `galigo.Bot` has no option for them: it always uses `DeliveryPolicyBlock`,
the 5 s default and no drop callback. To choose them, use the `receiver` package directly:

```go
cfg := receiver.DefaultConfig()
cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropOldest
cfg.OnUpdateDropped = func(updateID int, reason string) {
    metrics.Increment("updates_dropped", "reason", reason)
}
```

The reasons are listed in the doc of `Config.OnUpdateDropped`. An update that does not decode (for example, a field
with a type galigo does not expect) can be dropped instead of failing the request. The drop needs a usable
`update_id`: exactly one member whose name is `update_id` in any letter case, that member written `update_id`, and
an integer from 1 to the largest `int`, not math.MaxInt64. Webhook: such an update is dropped with the reason
`webhook_decode_error` and the handler answers 200; any other body that does not decode gets 400, as before. Long
polling: such an update is dropped with the reason `decode_error` only when the rest of the response decodes too:
`ok` is true, the other envelope fields (`error_code`, `description`) decode as usual, there is exactly one `result`
member (the same letter-case rule as `update_id`) and it is an array, every update in it that does not decode has a
usable `update_id`, and no update that decodes has the id math.MaxInt64. Otherwise the whole `getUpdates` call fails
as before: it counts as a polling error, and the client retries until `PollingMaxErrors` is reached (0 = no
limit). These rules apply only after a decode error: an update that decodes is delivered as before, also one with no
`update_id` (for example `{}`). The `Warn` record of a drop holds only the `update_id`, with no error text and no
value from the update.

## Single-Consumer Constraint (Polling Mode)

**Critical:** Only ONE active consumer per bot token when using long polling.

Running multiple replicas with the same token causes:
- Update loss (each replica gets different updates)
- Telegram 409 Conflict errors
- Unpredictable behavior

### Solutions for Multi-Replica Deployments

| Solution | Complexity | When to Use |
|----------|------------|-------------|
| Single replica | Low | Development, low-traffic bots |
| Leader election | Medium | Kubernetes with Lease API |
| Webhook mode | Medium | Production horizontal scaling |

### Leader Election Example (Kubernetes)

```go
import "k8s.io/client-go/tools/leaderelection"

leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
    Lock:          lock,
    LeaseDuration: 15 * time.Second,
    RenewDeadline: 10 * time.Second,
    RetryPeriod:   2 * time.Second,
    Callbacks: leaderelection.LeaderCallbacks{
        OnStartedLeading: func(ctx context.Context) {
            // Only the leader starts polling
            bot.Start(ctx)
            for update := range bot.Updates() {
                handleUpdate(ctx, update)
            }
        },
        OnStoppedLeading: func() {
            bot.Close()
        },
    },
})
```

## Polling vs Webhook

| Aspect | Polling | Webhook |
|--------|---------|---------|
| Setup | Simple — no ingress needed | Requires TLS, public endpoint |
| Scaling | Single consumer only | Horizontal scaling |
| Latency | Up to 30s (poll timeout) | Near-instant (push) |
| Firewall | Outbound only | Inbound required |

### When to Use Polling

- Development and testing
- Single-instance deployments
- Firewalled environments (outbound-only)
- Low-latency not critical

### When to Use Webhook

- Production horizontal scaling
- Low-latency requirements
- Multiple replicas

## Graceful Shutdown

```go
ctx, cancel := signal.NotifyContext(context.Background(),
    syscall.SIGINT, syscall.SIGTERM)
defer cancel()

bot, _ := galigo.New(token, galigo.WithPolling(30, 100))
bot.Start(ctx)

done := make(chan struct{})
go func() {
    defer close(done)
    for update := range bot.Updates() {
        // A context the signal does not cancel, so updates left in the channel are still handled
        handleUpdate(context.Background(), update)
    }
}()

<-ctx.Done() // the signal cancels ctx, which also ends the getUpdates request in flight

// Close takes no context: it stops the poller and closes the updates channel
if err := bot.Close(); err != nil {
    log.Error("shutdown error", "error", err)
}
<-done // the loop has read the updates left in the channel
```

## Thread Safety Summary

All galigo types are safe for concurrent use:

- Call `SendMessage()` from multiple goroutines simultaneously
- The update channel can be read by one consumer (but you can fan-out with a worker pool)
- Rate limiters and circuit breaker handle synchronization internally
