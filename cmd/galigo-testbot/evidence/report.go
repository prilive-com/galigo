package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// Report represents a test run report.
type Report struct {
	RunID     string                   `json:"run_id"`
	StartTime time.Time                `json:"start_time"`
	EndTime   time.Time                `json:"end_time"`
	Duration  time.Duration            `json:"duration"`
	Success   bool                     `json:"success"`
	Scenarios []*engine.ScenarioResult `json:"scenarios"`
	Summary   Summary                  `json:"summary"`
}

// Summary contains aggregate statistics.
type Summary struct {
	TotalScenarios  int      `json:"total_scenarios"`
	PassedScenarios int      `json:"passed_scenarios"`
	FailedScenarios int      `json:"failed_scenarios"`
	TotalSteps      int      `json:"total_steps"`
	PassedSteps     int      `json:"passed_steps"`
	FailedSteps     int      `json:"failed_steps"`
	MethodsCovered  []string `json:"methods_covered"`
	TotalDuration   string   `json:"total_duration"`
}

// NewReport creates a new report.
func NewReport() *Report {
	return &Report{
		RunID:     time.Now().Format("20060102-150405"),
		StartTime: time.Now(),
		Scenarios: make([]*engine.ScenarioResult, 0),
	}
}

// AddScenario adds a scenario result to the report.
func (r *Report) AddScenario(result *engine.ScenarioResult) {
	r.Scenarios = append(r.Scenarios, result)
}

// Finalize completes the report with summary statistics.
func (r *Report) Finalize() {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)

	methodsMap := make(map[string]bool)
	allPassed := true

	for _, s := range r.Scenarios {
		r.Summary.TotalScenarios++
		if s.Success {
			r.Summary.PassedScenarios++
		} else {
			r.Summary.FailedScenarios++
			allPassed = false
		}

		for _, step := range s.Steps {
			r.Summary.TotalSteps++
			if step.Success {
				r.Summary.PassedSteps++
			} else {
				r.Summary.FailedSteps++
			}
		}

		for _, method := range s.Covers {
			methodsMap[method] = true
		}
	}

	r.Success = allPassed
	r.Summary.TotalDuration = r.Duration.String()

	for method := range methodsMap {
		r.Summary.MethodsCovered = append(r.Summary.MethodsCovered, method)
	}
}

// ToJSON returns the report as JSON.
func (r *Report) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// Save saves the report to a file.
func (r *Report) Save(storageDir string) (string, error) {
	if err := os.MkdirAll(filepath.Join(storageDir, "reports"), 0o755); err != nil {
		return "", fmt.Errorf("failed to create reports directory: %w", err)
	}

	filename := filepath.Join(storageDir, "reports", fmt.Sprintf("report-%s.json", r.RunID))

	data, err := r.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write report: %w", err)
	}

	return filename, nil
}

// FormatSummary returns a human-readable summary.
func (r *Report) FormatSummary() string {
	var sb strings.Builder

	status := "PASSED"
	if !r.Success {
		status = "FAILED"
	}

	sb.WriteString(fmt.Sprintf("Test Run: %s\n", r.RunID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))
	sb.WriteString(fmt.Sprintf("Duration: %s\n\n", r.Duration.Round(time.Millisecond)))

	sb.WriteString(fmt.Sprintf("Scenarios: %d/%d passed\n",
		r.Summary.PassedScenarios, r.Summary.TotalScenarios))
	sb.WriteString(fmt.Sprintf("Steps: %d/%d passed\n",
		r.Summary.PassedSteps, r.Summary.TotalSteps))
	sb.WriteString(fmt.Sprintf("Methods covered: %d\n\n",
		len(r.Summary.MethodsCovered)))

	// List failed scenarios
	for _, s := range r.Scenarios {
		if !s.Success {
			sb.WriteString(fmt.Sprintf("FAILED: %s - %s\n", s.ScenarioName, s.Error))
		}
	}

	return sb.String()
}
