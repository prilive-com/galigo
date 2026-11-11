package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/prilive-com/galigo/tg"
)

// Runner executes scenarios with safety limits.
type Runner struct {
	runtime       *Runtime
	baseDelay     time.Duration
	jitter        time.Duration
	maxMessages   int
	messageCount  int
	retryOn429    bool
	max429Retries int
	logger        *slog.Logger
}

// RunnerConfig holds runner configuration.
type RunnerConfig struct {
	BaseDelay     time.Duration
	Jitter        time.Duration
	MaxMessages   int
	RetryOn429    bool
	Max429Retries int
}

// NewRunner creates a new scenario runner.
func NewRunner(rt *Runtime, cfg RunnerConfig, logger *slog.Logger) *Runner {
	return &Runner{
		runtime:       rt,
		baseDelay:     cfg.BaseDelay,
		jitter:        cfg.Jitter,
		maxMessages:   cfg.MaxMessages,
		retryOn429:    cfg.RetryOn429,
		max429Retries: cfg.Max429Retries,
		logger:        logger,
	}
}

// Run executes a scenario and returns the result.
func (r *Runner) Run(ctx context.Context, scenario Scenario) *ScenarioResult {
	result := &ScenarioResult{
		ScenarioName: scenario.Name(),
		Covers:       scenario.Covers(),
		StartTime:    time.Now(),
		Steps:        make([]StepResult, 0),
	}

	// Apply timeout
	timeout := scenario.Timeout()
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r.logger.Info("starting scenario",
		"name", scenario.Name(),
		"description", scenario.Description(),
		"covers", scenario.Covers())

	r.runtime.cleanupCarry = nil // counts of a failed CleanupStep attempt belong to that scenario only

	// A panicking step skips the code after the loop; the deferred call still cleans up before the panic leaves Run
	// (B-30). Deferred calls run while a panic unwinds, so the panic and its stack trace are not changed.
	panicking := true
	defer func() {
		if panicking {
			r.cleanupAfterStop(ctx, result)
		}
	}()

	stopped := false
	for _, step := range scenario.Steps() {
		// Check context cancellation
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
			stopped = true
			break
		}

		// Check message budget
		if r.messageCount >= r.maxMessages {
			result.Error = fmt.Sprintf("message budget exceeded (%d)", r.maxMessages)
			stopped = true
			break
		}

		// Execute step with 429 retry
		stepResult, err := r.runStepWithRetry(ctx, step)
		result.Steps = append(result.Steps, *stepResult)

		if err != nil {
			if IsSkip(err) {
				result.Success = true
				result.Skipped = true
				result.SkipReason = err.Error()
				r.logger.Info("scenario skipped", "step", step.Name(), "reason", err.Error())
				stopped = true
				break
			}
			result.Success = false
			result.Error = fmt.Sprintf("step %q failed: %v", step.Name(), err)
			stopped = true
			break
		}

		// Pace between steps: base delay + random jitter
		if err := r.pace(ctx); err != nil {
			result.Error = err.Error()
			stopped = true
			break
		}
	}

	panicking = false
	if stopped {
		r.cleanupAfterStop(ctx, result)
	}

	if result.Error == "" {
		result.Success = true
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	r.logger.Info("scenario completed",
		"name", scenario.Name(),
		"success", result.Success,
		"duration", result.Duration,
		"messages", r.messageCount)

	return result
}

// cleanupAfterStop deletes what a stopped scenario left tracked (messages, sticker sets, member tags), on a context
// that outlives the scenario's (each call bounded by deleteTracked), and records it as the step "cleanup after stop".
// One attempt per item, with no 429 wait. A failed item is named in the step and the scenario fails. An item whose
// error is permanent (a non-retryable *tg.APIError) is then dropped from tracking, so it does not fail every later
// scenario of the run; a transient failure (circuit breaker, network, 429, 500–504, rate-limit wait) stays tracked for a
// later cleanup (B-30, B-55). Nothing tracked: no call, no step.
func (r *Runner) cleanupAfterStop(parent context.Context, result *ScenarioResult) {
	rt := r.runtime
	if len(rt.CreatedMessages)+len(rt.CreatedStickerSets)+len(rt.taggedMembers) == 0 {
		return
	}

	start := time.Now()
	ev, failed := deleteTracked(context.WithoutCancel(parent), rt, true)
	step := StepResult{
		StepName: "cleanup after stop",
		Method:   "deleteMessage",
		Duration: time.Since(start),
		Success:  len(failed) == 0,
		Evidence: ev,
	}
	if len(failed) > 0 {
		step.Error = errors.Join(failed...).Error()
		result.Success = false
		if result.Error != "" {
			result.Error += "; "
		}
		result.Error += "cleanup after stop: " + step.Error
		r.logger.Warn("cleanup after stop failed", "scenario", result.ScenarioName, "evidence", ev)
	} else {
		r.logger.Info("cleanup after stop", "scenario", result.ScenarioName, "evidence", ev)
	}
	result.Steps = append(result.Steps, step)
}

// runStepWithRetry executes a step, retrying on 429 errors.
func (r *Runner) runStepWithRetry(ctx context.Context, step Step) (*StepResult, error) {
	maxAttempts := 1
	if r.retryOn429 {
		maxAttempts = 1 + r.max429Retries
	}

	var stepResult *StepResult
	var err error

	for attempt := range maxAttempts {
		stepResult, err = r.runStep(ctx, step)
		if err == nil {
			return stepResult, nil
		}

		// Check for 429 rate limit
		var apiErr *tg.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 429 {
			return stepResult, err // Not a 429, fail immediately
		}

		// 429 — wait and retry
		retryAfter := apiErr.RetryAfter
		if retryAfter == 0 {
			retryAfter = 5 * time.Second
		}
		// Add 500ms safety margin
		waitTime := retryAfter + 500*time.Millisecond

		r.logger.Warn("rate limited, retrying step",
			"step", step.Name(),
			"attempt", attempt+1,
			"max_attempts", maxAttempts,
			"retry_after", retryAfter,
			"wait", waitTime)

		select {
		case <-time.After(waitTime):
			continue
		case <-ctx.Done():
			return stepResult, ctx.Err()
		}
	}

	return stepResult, err
}

func (r *Runner) runStep(ctx context.Context, step Step) (*StepResult, error) {
	start := time.Now()

	r.logger.Debug("executing step", "step", step.Name())

	stepResult, err := step.Execute(ctx, r.runtime)
	if stepResult == nil {
		stepResult = &StepResult{StepName: step.Name()}
	}

	stepResult.Duration = time.Since(start)

	if err != nil {
		stepResult.Success = false
		stepResult.Error = err.Error()
		r.logger.Error("step failed", "step", step.Name(), "error", err, "duration", stepResult.Duration)
		return stepResult, err
	}

	stepResult.Success = true
	r.messageCount += len(stepResult.MessageIDs)

	r.logger.Info("step completed",
		"step", step.Name(),
		"duration", stepResult.Duration,
		"messages", len(stepResult.MessageIDs))

	return stepResult, nil
}

// pace waits for base delay + random jitter between steps.
func (r *Runner) pace(ctx context.Context) error {
	if r.baseDelay == 0 && r.jitter == 0 {
		return nil
	}

	delay := r.baseDelay
	if r.jitter > 0 {
		delay += time.Duration(rand.Int64N(int64(r.jitter)))
	}

	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Runtime returns the runner's runtime for access to state.
func (r *Runner) Runtime() *Runtime {
	return r.runtime
}

// MessageCount returns the number of messages sent.
func (r *Runner) MessageCount() int {
	return r.messageCount
}

// ResetMessageCount resets the message counter (for new runs).
func (r *Runner) ResetMessageCount() {
	r.messageCount = 0
}
