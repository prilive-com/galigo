package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence"
)

// b72Step is a fake step: it counts its runs, calls do (if set), records whether the context it was given is
// cancelled after that, and returns err.
type b72Step struct {
	runs     *int
	sawCtxOK *bool // set to true when the step's context is cancelled after do
	do       func()
	err      error
}

func (s b72Step) Name() string { return "b72-step" }

func (s b72Step) Execute(ctx context.Context, _ *engine.Runtime) (*engine.StepResult, error) {
	*s.runs++
	if s.do != nil {
		s.do()
	}
	*s.sawCtxOK = ctx.Err() != nil
	if s.err != nil {
		return nil, s.err
	}
	return &engine.StepResult{Method: "b72"}, nil
}

// b72FakeScenario counts calls to Steps (the runner calls it once, after it starts a scenario) and calls onSteps first.
type b72FakeScenario struct {
	engine.BaseScenario
	admitted *int
	onSteps  func()
}

func (s *b72FakeScenario) Steps() []engine.Step {
	*s.admitted++
	if s.onSteps != nil {
		s.onSteps()
	}
	return s.BaseScenario.Steps()
}

// TestB72_RunScenarios: the run context is checked before each scenario starts, the report is finalized, and an
// interrupted run's report is marked failed after Finalize (PLAN §4, AC2). Each scenario has exactly one step (a
// scenario with no step passes on a cancelled context). Rows: a context cancelled before the first scenario; cancelled
// after the first scenario was admitted, before its step; a scenario that passes while its step cancels the context
// (the step must see its own context cancelled: the scenario runs on the run context); a scenario failing with a
// returned context error while the run context is live (E-0108: that is not an interrupt; the next scenario runs);
// the last step of the last scenario cancels (no further scenario to stop, but the run was interrupted: failed); the
// last scenario's step cancels and reports a skip (a step that turns the cancel into a skip or a success must not
// make the run pass, change review C1-01); no scenario and a cancelled context; all pass. Admissions (calls to Steps)
// show which scenarios the runner started; the summary totals and the end time show that Finalize ran.
func TestB72_RunScenarios(t *testing.T) {
	type want struct {
		name string
		ok   bool
	}
	type fix struct {
		step    b72Step
		onSteps bool // the scenario's Steps call cancels the run context
	}
	for _, c := range []struct {
		name     string
		pre      bool                      // cancel before runScenarios
		fixtures func(cancel func()) []fix // one scenario per fixture, scenario i named "B72-<i>"
		want     []want
		admitted []int
		runs     []int
		sawCtx   int // index of the step that must see its context cancelled, or -1
		success  bool
	}{
		{"cancelled before the first", true,
			func(func()) []fix { return []fix{{}, {}} },
			[]want{}, []int{0, 0}, []int{0, 0}, -1, false},
		{"cancelled after admission", false,
			func(func()) []fix { return []fix{{onSteps: true}, {}} },
			[]want{{"B72-0", false}}, []int{1, 0}, []int{0, 0}, -1, false},
		{"passes then cancel", false,
			func(cancel func()) []fix { return []fix{{step: b72Step{do: cancel}}, {}, {}} },
			[]want{{"B72-0", true}}, []int{1, 0, 0}, []int{1, 0, 0}, 0, false},
		{"returned deadline error live run", false,
			func(func()) []fix { return []fix{{step: b72Step{err: context.DeadlineExceeded}}, {}} },
			[]want{{"B72-0", false}, {"B72-1", true}}, []int{1, 1}, []int{1, 1}, -1, false},
		{"returned canceled error live run", false,
			func(func()) []fix { return []fix{{step: b72Step{err: context.Canceled}}, {}} },
			[]want{{"B72-0", false}, {"B72-1", true}}, []int{1, 1}, []int{1, 1}, -1, false},
		{"last step of the last scenario cancels", false,
			func(cancel func()) []fix { return []fix{{}, {step: b72Step{do: cancel}}} },
			[]want{{"B72-0", true}, {"B72-1", true}}, []int{1, 1}, []int{1, 1}, 1, false},
		{"last scenario cancels and skips", false,
			func(cancel func()) []fix { return []fix{{step: b72Step{do: cancel, err: engine.Skip("b72 skip")}}} },
			[]want{{"B72-0", true}}, []int{1}, []int{1}, 0, false},
		{"no scenario cancelled", true,
			func(func()) []fix { return nil },
			[]want{}, []int{}, []int{}, -1, false},
		{"all pass", false,
			func(func()) []fix { return []fix{{}, {}} },
			[]want{{"B72-0", true}, {"B72-1", true}}, []int{1, 1}, []int{1, 1}, -1, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixtures := c.fixtures(cancel)
			admitted := make([]int, len(fixtures))
			runs := make([]int, len(fixtures))
			saw := make([]bool, len(fixtures))
			scenarios := make([]engine.Scenario, len(fixtures))
			for i, f := range fixtures {
				st := f.step
				st.runs, st.sawCtxOK = &runs[i], &saw[i]
				s := &b72FakeScenario{admitted: &admitted[i]}
				s.ScenarioName = "B72-" + string(rune('0'+i))
				s.ScenarioSteps = []engine.Step{st}
				if f.onSteps {
					s.onSteps = cancel
				}
				scenarios[i] = s
			}
			logger := slog.New(slog.DiscardHandler)
			runner := engine.NewRunner(engine.NewRuntime(nil, 1, 1), engine.RunnerConfig{MaxMessages: 100}, logger)
			report := evidence.NewReport()
			if c.pre {
				cancel()
			}

			runScenarios(ctx, runner, scenarios, report, logger)

			assert.Equal(t, c.success, report.Success, "the report's success (marked failed when interrupted)")
			status := "Status: FAILED\n"
			if c.success {
				status = "Status: PASSED\n"
			}
			assert.Contains(t, report.FormatSummary(), status)
			assert.False(t, report.EndTime.IsZero(), "Finalize ran (end time set)")
			assert.Equal(t, len(c.want), report.Summary.TotalScenarios, "Finalize ran (summary totals)")
			assert.Equal(t, c.admitted, admitted, "scenarios the runner started (calls to Steps)")
			assert.Equal(t, c.runs, runs, "step runs per scenario")
			if c.sawCtx >= 0 {
				assert.True(t, saw[c.sawCtx], "the step's own context is cancelled: the scenario runs on the run context")
			}
			got := make([]want, len(report.Scenarios))
			for i, s := range report.Scenarios {
				got[i] = want{s.ScenarioName, s.Success}
			}
			require.Equal(t, c.want, got, "exactly the scenarios that started, in order")
		})
	}
}
