package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Runner executes scenarios with safety limits.
type Runner struct {
	runtime      *Runtime
	sendInterval time.Duration
	maxMessages  int
	messageCount int
	logger       *slog.Logger
}

// NewRunner creates a new scenario runner.
func NewRunner(rt *Runtime, sendInterval time.Duration, maxMessages int, logger *slog.Logger) *Runner {
	return &Runner{
		runtime:      rt,
		sendInterval: sendInterval,
		maxMessages:  maxMessages,
		logger:       logger,
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

	for _, step := range scenario.Steps() {
		// Check context cancellation
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
			break
		}

		// Check message budget
		if r.messageCount >= r.maxMessages {
			result.Error = fmt.Sprintf("message budget exceeded (%d)", r.maxMessages)
			break
		}

		// Execute step
		stepResult, err := r.runStep(ctx, step)
		result.Steps = append(result.Steps, *stepResult)

		if err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("step %q failed: %v", step.Name(), err)
			break
		}

		// Rate limiting between steps (only if more steps remain)
		select {
		case <-ctx.Done():
			result.Error = ctx.Err().Error()
		case <-time.After(r.sendInterval):
			// Continue
		}
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
