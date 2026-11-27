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

<!-- go-check: func(ctx context.Context, token string) -->
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
    _ = update // handle the update
}
```

### Stopping the Bot

<!-- go-check: func(bot *galigo.Bot) -->
```go
// Close stops polling and waits for goroutines to finish
if err := bot.Close(); err != nil {
    slog.Error("close failed", "error", err)
}
```

**Important:** Always call `Close()` when you are done: `Stop()` only stops the poller, and `Close()` also closes the
updates channel (polling mode) and the sender. `Stop()` and `Close()` may run at the same time: each waits until the
poller has exited. Do not call them while `Start` is running. To restart, call `Stop()`, wait until every `Stop()` call
has returned, then `Start`; `Close()` is final: after it, make a new bot. Never call them from the poller's own
goroutine (for example from a log handler), and do not wait there for code that calls them: they wait for that
goroutine and never return. In polling mode `Close()` stops the poller and waits for it to exit. It does not cancel a
`getUpdates` request in flight: cancel the context you passed to `Start` to cancel that request. The default HTTP
client timeout is the long-poll timeout plus 10 s; it limits the request, not `Close()`. While the poller stops, it can
still send an update when the channel has space, with every policy, and the other policies can also drop updates then.
Such a send happens before `Stop()` and `Close()` return, so `Close()` never closes the channel under it.
With `DeliveryPolicyBlock` (the only policy `galigo.Bot` uses), when a stop or a cancelled `Start` context ends a wait
for channel space, the poller does not deliver or drop that update or the rest of the batch, and does not move its
offset for them (a delivery timeout that ends first still drops the update). Telegram confirms an update only when it
receives a `getUpdates` request with a higher offset, so after a restart, unconfirmed updates that Telegram still holds
can come again, also ones that galigo already delivered or dropped (Telegram keeps updates for at most 24 hours).
Updates already in the channel stay readable: in polling mode `Close()` closes the channel, so a
`for range bot.Updates()` loop reads them and then ends. In webhook mode, stop your HTTP server first, then call
`Close()`. It does not close the channel, so a `for range` loop does not end by itself: stop it with your own signal or
time limit.

## Channel-Based Delivery

Updates arrive via a buffered channel:

<!-- go-check: func(bot *galigo.Bot) -->
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
  time runs out, it drops the update (reason `delivery_timeout`) and moves its local offset past it. Telegram confirms
  the update only when it receives a `getUpdates` request with a higher offset.
- **Webhook mode**: the same policies apply. When the handler drops an update, it writes 200; if Telegram receives
  that response, it does not send the update again. With `DeliveryPolicyBlock`, the handler waits for space until the
  timeout or until the request ends. With `DeliveryPolicyDropOldest` and an unbuffered channel, the handler waits until
  a reader takes the update or the request ends. If the end of the request ends the wait (for example, Telegram stops
  waiting), the update is not delivered and no drop is reported; Telegram may send it again.

### Recommended Consumer Pattern

<!-- go-check: func(ctx context.Context, bot *galigo.Bot, handleUpdate func(context.Context, tg.Update)) -->
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
| `DeliveryPolicyDropOldest` | Drop oldest update in buffer, accept new; if the consumer empties the buffer first, deliver and drop nothing (with both free space and older updates, the oldest may still be dropped). Unbuffered channel: drop nothing, wait for a reader like Block with no time limit |

These settings are in `receiver.Config`. `galigo.Bot` has no option for them: it always uses `DeliveryPolicyBlock`,
the 5 s default and no drop callback. To choose them, use the `receiver` package directly:

<!-- go-check: func(metrics interface{ Increment(name string, tags ...string) }) -->
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

<!-- go-check: skip imports k8s.io/client-go, which is not a galigo dependency -->
```go
import "k8s.io/client-go/tools/leaderelection"

// RunOrDie does not wait for OnStartedLeading to return, so a process that exits right after RunOrDie
// returns may exit before bot.Close has run.
electionCtx, stopElection := context.WithCancel(ctx)
defer stopElection()

leaderelection.RunOrDie(electionCtx, leaderelection.LeaderElectionConfig{
    Lock:          lock,
    LeaseDuration: 15 * time.Second,
    RenewDeadline: 10 * time.Second,
    RetryPeriod:   2 * time.Second,
    Callbacks: leaderelection.LeaderCallbacks{
        OnStartedLeading: func(ctx context.Context) {
            // This replica starts polling after it gets the lease; ctx ends when it loses the lease. client-go does not
            // promise that only one replica polls at a time. This callback owns the bot: it makes it, starts it and closes it.
            bot, err := galigo.New(token, galigo.WithPolling(30, 100))
            if err != nil {
                log.Printf("bot: %v", err)
                stopElection() // stop renewing the lease (it expires): this replica cannot poll
                return
            }
            defer bot.Close() // runs after Start has returned
            if err := bot.Start(ctx); err != nil {
                log.Printf("bot start: %v", err)
                stopElection()
                return
            }
            for {
                select {
                case <-ctx.Done(): // leadership ended
                    return
                case update := <-bot.Updates():
                    handleUpdate(ctx, update)
                }
            }
        },
        OnStoppedLeading: func() {
            // Required by client-go. OnStartedLeading closes the bot; do not use it here.
            log.Print("leadership lost")
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

<!-- go-check: func(token string, handleUpdate func(context.Context, tg.Update)) -->
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
cancel()     // a second Ctrl-C now ends the program at once

// Close takes no context: it stops the poller and closes the updates channel
if err := bot.Close(); err != nil {
    slog.Error("shutdown error", "error", err)
}

// Wait for the loop to handle the updates left in the channel, but not forever
select {
case <-done:
case <-time.After(10 * time.Second):
    slog.Warn("handlers did not finish in time")
}
```

## Thread Safety Summary

All galigo types are safe for concurrent use (for `Bot.Start`, `Stop` and `Close`, and `PollingClient.Start` and
`Stop`, see [Stopping the Bot](#stopping-the-bot)):

- Call `SendMessage()` from multiple goroutines simultaneously
- The update channel can be read by one consumer (but you can fan-out with a worker pool)
- Rate limiters and circuit breaker handle synchronization internally
