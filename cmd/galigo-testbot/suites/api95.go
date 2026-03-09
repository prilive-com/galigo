package suites

import (
	"time"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
)

// ==================== Bot API 9.5 Scenarios ====================

// S44_DateTimeEntity tests sending messages with date_time formatting.
func S44_DateTimeEntity() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S44-DateTimeEntity",
		ScenarioDescription: "Send message with date_time entity (9.5)",
		CoveredMethods:      []string{"sendMessage"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendDateTimeMessageStep{},
			&engine.CleanupStep{},
		},
	}
}

// S45_MemberTags tests setting, verifying, and removing member tags.
func S45_MemberTags() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S45-MemberTags",
		ScenarioDescription: "Set, verify, and remove member tags (9.5)",
		CoveredMethods:      []string{"setChatMemberTag", "getChatMember"},
		ScenarioTimeout:     60 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SetChatMemberTagStep{Tag: "galigo-test"},
			&engine.VerifyChatMemberTagStep{ExpectedTag: "galigo-test"},
			&engine.SetChatMemberTagStep{Tag: ""},
			&engine.VerifyChatMemberTagStep{ExpectedTag: ""},
		},
	}
}

// S46_MessageStreaming tests sendMessageDraft.
func S46_MessageStreaming() engine.Scenario {
	return &engine.BaseScenario{
		ScenarioName:        "S46-MessageStreaming",
		ScenarioDescription: "Send streaming draft message (9.5)",
		CoveredMethods:      []string{"sendMessageDraft"},
		ScenarioTimeout:     30 * time.Second,
		ScenarioSteps: []engine.Step{
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading"},
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading..."},
			&engine.SendMessageDraftStep{DraftID: 1, Text: "Loading complete!"},
		},
	}
}

// AllAPI95Scenarios returns all Bot API 9.5 test scenarios.
func AllAPI95Scenarios() []engine.Scenario {
	return []engine.Scenario{
		S44_DateTimeEntity(),
		S45_MemberTags(),
		S46_MessageStreaming(),
	}
}
