package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// B-65 65h: S78 share theme backdrops (TEST-PLAN section 4). Every value is written out here.

func TestB65H_S78(t *testing.T) {
	assert.Equal(t, &engine.BaseScenario{
		ScenarioName:        "S78-ShareThemeBackdrops",
		ScenarioDescription: "Get the chat share theme backdrops (Bot API 10.4, read only)",
		CoveredMethods:      []string{"getChatShareThemeBackdrops"},
		ScenarioSteps:       []engine.Step{&engine.GetChatShareThemeBackdropsStep{}},
		ScenarioTimeout:     30 * time.Second,
	}, S78_ShareThemeBackdrops())
}

func TestB65H_API104Names(t *testing.T) {
	var names []string
	for _, sc := range AllAPI104Scenarios() {
		names = append(names, sc.Name())
	}
	assert.Equal(t, []string{"S76-Widgets", "S77-RichQuote", "S78-ShareThemeBackdrops"}, names)
}
