package registry

import "slices"

// MethodCategory groups methods by functional area.
type MethodCategory string

const (
	CategoryMessaging MethodCategory = "messaging"
	CategoryChatAdmin MethodCategory = "chat-admin"
	CategoryExtended  MethodCategory = "extended"
	CategoryLegacy    MethodCategory = "legacy"
)

// Method represents a galigo API method.
type Method struct {
	Name     string
	Category MethodCategory
	Notes    string // e.g., "requires webhook infra"
}

// AllMethods is the complete list of galigo API methods.
var AllMethods = []Method{
	// === Messaging Methods ===
	// Core
	{Name: "getMe", Category: CategoryMessaging},
	{Name: "sendMessage", Category: CategoryMessaging},
	{Name: "editMessageText", Category: CategoryMessaging},
	{Name: "deleteMessage", Category: CategoryMessaging},

	// Callbacks
	{Name: "answerCallbackQuery", Category: CategoryMessaging},
	{Name: "editMessageReplyMarkup", Category: CategoryMessaging},

	// Forward/Copy
	{Name: "forwardMessage", Category: CategoryMessaging},
	{Name: "copyMessage", Category: CategoryMessaging},

	// Chat action
	{Name: "sendChatAction", Category: CategoryMessaging},

	// Media uploads (multipart)
	{Name: "sendPhoto", Category: CategoryMessaging},
	{Name: "sendDocument", Category: CategoryMessaging},
	{Name: "sendVideo", Category: CategoryMessaging},
	{Name: "sendAudio", Category: CategoryMessaging},
	{Name: "sendAnimation", Category: CategoryMessaging},
	{Name: "sendVoice", Category: CategoryMessaging},
	{Name: "sendVideoNote", Category: CategoryMessaging},
	{Name: "sendSticker", Category: CategoryMessaging},

	// Albums
	{Name: "sendMediaGroup", Category: CategoryMessaging},

	// Media edit
	{Name: "editMessageMedia", Category: CategoryMessaging},
	{Name: "editMessageCaption", Category: CategoryMessaging},

	// Files
	{Name: "getFile", Category: CategoryMessaging},

	// Location & Contact
	{Name: "sendLocation", Category: CategoryMessaging},
	{Name: "sendVenue", Category: CategoryMessaging},
	{Name: "sendContact", Category: CategoryMessaging},
	{Name: "sendDice", Category: CategoryMessaging},

	// Reactions
	{Name: "setMessageReaction", Category: CategoryMessaging},

	// Bulk operations
	{Name: "forwardMessages", Category: CategoryMessaging},
	{Name: "copyMessages", Category: CategoryMessaging},
	{Name: "deleteMessages", Category: CategoryMessaging},

	// User info
	{Name: "getUserProfilePhotos", Category: CategoryMessaging},

	// === Chat Administration Methods ===
	// Chat info
	{Name: "getChat", Category: CategoryChatAdmin},
	{Name: "getChatAdministrators", Category: CategoryChatAdmin},
	{Name: "getChatMemberCount", Category: CategoryChatAdmin},
	{Name: "getChatMember", Category: CategoryChatAdmin},

	// Chat settings
	{Name: "setChatTitle", Category: CategoryChatAdmin},
	{Name: "setChatDescription", Category: CategoryChatAdmin},
	{Name: "setChatPhoto", Category: CategoryChatAdmin},
	{Name: "deleteChatPhoto", Category: CategoryChatAdmin},
	{Name: "setChatPermissions", Category: CategoryChatAdmin},

	// Boosts
	{Name: "getUserChatBoosts", Category: CategoryChatAdmin},

	// Pin messages
	{Name: "pinChatMessage", Category: CategoryChatAdmin},
	{Name: "unpinChatMessage", Category: CategoryChatAdmin},
	{Name: "unpinAllChatMessages", Category: CategoryChatAdmin},

	// Polls
	{Name: "sendPoll", Category: CategoryChatAdmin},
	{Name: "stopPoll", Category: CategoryChatAdmin},

	// Forum
	{Name: "getForumTopicIconStickers", Category: CategoryChatAdmin},

	// === Extended: Stickers ===
	{Name: "createNewStickerSet", Category: CategoryExtended},
	{Name: "getStickerSet", Category: CategoryExtended},
	{Name: "addStickerToSet", Category: CategoryExtended},
	{Name: "setStickerPositionInSet", Category: CategoryExtended},
	{Name: "setStickerEmojiList", Category: CategoryExtended},
	{Name: "setStickerSetTitle", Category: CategoryExtended},
	{Name: "deleteStickerFromSet", Category: CategoryExtended},
	{Name: "deleteStickerSet", Category: CategoryExtended},

	// === Extended: Stars & Payments ===
	{Name: "getMyStarBalance", Category: CategoryExtended},
	{Name: "getStarTransactions", Category: CategoryExtended},
	{Name: "sendInvoice", Category: CategoryExtended},

	// === Extended: Gifts ===
	{Name: "getAvailableGifts", Category: CategoryExtended},

	// === Extended: Checklists ===
	{Name: "sendChecklist", Category: CategoryExtended},
	{Name: "editChecklist", Category: CategoryExtended},

	// === Legacy Methods ===
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

// MessagingMethods returns only messaging methods.
func MessagingMethods() []Method {
	var methods []Method
	for _, m := range AllMethods {
		if m.Category == CategoryMessaging {
			methods = append(methods, m)
		}
	}
	return methods
}

// ChatAdminMethods returns only chat administration methods.
func ChatAdminMethods() []Method {
	var methods []Method
	for _, m := range AllMethods {
		if m.Category == CategoryChatAdmin {
			methods = append(methods, m)
		}
	}
	return methods
}

// ExtendedMethods returns only extended methods (stickers, stars, gifts, checklists).
func ExtendedMethods() []Method {
	var methods []Method
	for _, m := range AllMethods {
		if m.Category == CategoryExtended {
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
