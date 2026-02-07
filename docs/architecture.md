# How galigo Works

galigo is not a passive library — it runs background goroutines and manages internal state. This document explains the operational model you need to understand for correct usage.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         galigo.Bot                              │
│                                                                 │
│  ┌──────────────────────┐      ┌──────────────────────────┐    │
│  │  receiver.Polling    │      │     sender.Client        │    │
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

**Important:** Always call `Close()` to ensure graceful shutdown. The polling goroutine will finish its current request and drain pending updates.

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

- **Polling mode**: The polling goroutine blocks until space is available
- **Webhook mode**: Returns 503 Service Unavailable (Telegram will retry)

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
| `DeliveryPolicyBlock` | Block until space available (default) |
| `DeliveryPolicyDropNewest` | Drop incoming update, continue |
| `DeliveryPolicyDropOldest` | Drop oldest update in buffer, accept new |

```go
cfg := receiver.DefaultConfig()
cfg.UpdateDeliveryPolicy = receiver.DeliveryPolicyDropOldest
cfg.OnUpdateDropped = func(updateID int, reason string) {
    metrics.Increment("updates_dropped", "reason", reason)
}
```

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

// Handle updates until signal
go func() {
    for update := range bot.Updates() {
        handleUpdate(ctx, update)
    }
}()

<-ctx.Done()

// Graceful shutdown with timeout
shutdownCtx, shutdownCancel := context.WithTimeout(
    context.Background(), 10*time.Second)
defer shutdownCancel()

if err := bot.Close(); err != nil {
    log.Error("shutdown error", "error", err)
}
```

## Thread Safety Summary

All galigo types are safe for concurrent use:

- Call `SendMessage()` from multiple goroutines simultaneously
- The update channel can be read by one consumer (but you can fan-out with a worker pool)
- Rate limiters and circuit breaker handle synchronization internally
