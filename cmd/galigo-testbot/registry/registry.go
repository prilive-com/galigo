package registry

import "slices"

// MethodCategory groups methods by implementation phase.
type MethodCategory string

const (
	CategoryTier1  MethodCategory = "tier1"
	CategoryLegacy MethodCategory = "legacy"
)

// Method represents a galigo API method.
type Method struct {
	Name     string
	Category MethodCategory
	Notes    string // e.g., "requires webhook infra"
}

// AllMethods is the complete list of galigo API methods.
var AllMethods = []Method{
	// === Tier 1 Methods ===
	// Core
	{Name: "getMe", Category: CategoryTier1},
	{Name: "sendMessage", Category: CategoryTier1},
	{Name: "editMessageText", Category: CategoryTier1},
	{Name: "deleteMessage", Category: CategoryTier1},

	// Callbacks
	{Name: "answerCallbackQuery", Category: CategoryTier1},
	{Name: "editMessageReplyMarkup", Category: CategoryTier1},

	// Forward/Copy
	{Name: "forwardMessage", Category: CategoryTier1},
	{Name: "copyMessage", Category: CategoryTier1},

	// Chat action
	{Name: "sendChatAction", Category: CategoryTier1},

	// Media uploads (multipart)
	{Name: "sendPhoto", Category: CategoryTier1},
	{Name: "sendDocument", Category: CategoryTier1},
	{Name: "sendVideo", Category: CategoryTier1},
	{Name: "sendAudio", Category: CategoryTier1},
	{Name: "sendAnimation", Category: CategoryTier1},
	{Name: "sendVoice", Category: CategoryTier1},
	{Name: "sendVideoNote", Category: CategoryTier1},
	{Name: "sendSticker", Category: CategoryTier1},

	// Albums
	{Name: "sendMediaGroup", Category: CategoryTier1},

	// Media edit
	{Name: "editMessageMedia", Category: CategoryTier1},
	{Name: "editMessageCaption", Category: CategoryTier1},

	// Files
	{Name: "getFile", Category: CategoryTier1},

	// === Legacy Methods (pre-Tier-1) ===
	// Webhook management
	{Name: "setWebhook", Category: CategoryLegacy, Notes: "requires webhook infra"},
	{Name: "deleteWebhook", Category: CategoryLegacy},
	{Name: "getWebhookInfo", Category: CategoryLegacy},

	// Polling
	{Name: "getUpdates", Category: CategoryLegacy, Notes: "internal to receiver"},
}

// MethodNames returns just the names.
func MethodNames() []string {
	names := make([]string, len(AllMethods))
	for i, m := range AllMethods {
		names[i] = m.Name
	}
	return names
}

// Tier1Methods returns only Tier-1 methods.
func Tier1Methods() []Method {
	var methods []Method
	for _, m := range AllMethods {
		if m.Category == CategoryTier1 {
			methods = append(methods, m)
		}
	}
	return methods
}

// LegacyMethods returns only legacy methods.
func LegacyMethods() []Method {
	var methods []Method
	for _, m := range AllMethods {
		if m.Category == CategoryLegacy {
			methods = append(methods, m)
		}
	}
	return methods
}

// CoverageReport shows which methods are covered/missing.
type CoverageReport struct {
	Covered []string
	Skipped []string // With reasons
	Missing []string
}

// Coverer is implemented by scenarios to declare method coverage.
type Coverer interface {
	Covers() []string
}

// CheckCoverage compares scenarios against method registry.
func CheckCoverage(scenarios []Coverer) *CoverageReport {
	allMethods := make(map[string]bool)
	for _, m := range AllMethods {
		allMethods[m.Name] = false
	}

	// Mark covered methods
	for _, s := range scenarios {
		for _, method := range s.Covers() {
			allMethods[method] = true
		}
	}

	report := &CoverageReport{}

	for method, isCovered := range allMethods {
		if isCovered {
			report.Covered = append(report.Covered, method)
		} else {
			report.Missing = append(report.Missing, method)
		}
	}

	slices.Sort(report.Covered)
	slices.Sort(report.Missing)

	return report
}
