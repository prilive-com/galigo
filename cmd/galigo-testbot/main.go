package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/cleanup"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/evidence"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/registry"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites"
	"github.com/prilive-com/galigo/receiver"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

var (
	runSuite        = flag.String("run", "", "Run a test suite: smoke, messages, forward, actions, core, media, keyboards, all")
	skipInteractive = flag.Bool("skip-interactive", false, "Skip interactive scenarios")
	showStatus      = flag.Bool("status", false, "Show method coverage status")
)

func main() {
	flag.Parse()

	// Load .env file if present (doesn't override existing env vars)
	_ = config.LoadDotEnv(".env")
	_ = config.LoadDotEnv("cmd/galigo-testbot/.env") // Also check in subdir

	// Setup logging
	logLevel := slog.LevelInfo
	if os.Getenv("TESTBOT_LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	// Handle --status flag early (doesn't need config)
	if *showStatus {
		showCoverageStatus(logger)
		return
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("galigo-testbot starting",
		"mode", cfg.Mode,
		"chat_id", cfg.ChatID,
		"admins", cfg.Admins)

	// Create sender client
	senderClient, err := sender.New(
		cfg.Token,
		sender.WithLogger(logger),
		sender.WithRetries(3),
	)
	if err != nil {
		logger.Error("failed to create sender", "error", err)
		os.Exit(1)
	}
	defer senderClient.Close()

	if *runSuite != "" {
		runSuiteCommand(cfg, senderClient, logger, *runSuite, *skipInteractive)
		return
	}

	// Interactive mode - listen for commands
	runInteractive(cfg, senderClient, logger)
}

func showCoverageStatus(logger *slog.Logger) {
	// Combine all scenarios from all phases (including interactive)
	scenarios := append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
	scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
	scenarios = append(scenarios, suites.AllChatAdminScenarios()...)
	scenarios = append(scenarios, suites.AllStickerScenarios()...)
	scenarios = append(scenarios, suites.AllStarsScenarios()...)
	scenarios = append(scenarios, suites.AllGiftScenarios()...)
	scenarios = append(scenarios, suites.AllChecklistScenarios()...)
	scenarios = append(scenarios, suites.AllInteractiveScenarios()...)
	scenarios = append(scenarios, suites.AllWebhookScenarios()...)
	scenarios = append(scenarios, suites.AllExtrasScenarios()...)
	scenarios = append(scenarios, suites.AllBotConfigScenarios()...)
	scenarios = append(scenarios, suites.AllAPI94Scenarios()...)

	// Convert to Coverer interface
	coverers := make([]registry.Coverer, len(scenarios))
	for i, s := range scenarios {
		coverers[i] = s
	}

	report := registry.CheckCoverage(coverers)

	fmt.Println("Method Coverage Status (All Phases + Tier 2 + Interactive + Webhook + Bot Config)")
	fmt.Println("================================================================================")
	fmt.Printf("Covered: %d methods\n", len(report.Covered))
	for _, m := range report.Covered {
		fmt.Printf("  + %s\n", m)
	}
	fmt.Printf("\nMissing: %d methods\n", len(report.Missing))
	for _, m := range report.Missing {
		fmt.Printf("  - %s\n", m)
	}
}

func runSuiteCommand(cfg *config.Config, senderClient *sender.Client, logger *slog.Logger, suite string, skipInteractive bool) {
	adapter := engine.NewSenderAdapter(senderClient).WithToken(tg.SecretToken(cfg.Token))
	rt := engine.NewRuntime(adapter, cfg.ChatID, cfg.Admins[0])
	runner := engine.NewRunner(rt, engine.RunnerConfig{
		BaseDelay:     cfg.SendInterval,
		Jitter:        cfg.JitterInterval,
		MaxMessages:   cfg.MaxMessagesPerRun,
		RetryOn429:    cfg.RetryOn429,
		Max429Retries: cfg.Max429Retries,
	}, logger)

	var scenarios []engine.Scenario

	switch strings.ToLower(suite) {
	case "smoke":
		scenarios = []engine.Scenario{suites.S0_Smoke()}
	case "identity":
		scenarios = []engine.Scenario{suites.S1_Identity()}
	case "messages":
		scenarios = []engine.Scenario{suites.S2_MessageLifecycle()}
	case "forward":
		scenarios = []engine.Scenario{suites.S4_ForwardCopy()}
	case "actions":
		scenarios = []engine.Scenario{suites.S5_ChatAction()}
	case "formatted", "formatting", "parse-mode":
		scenarios = []engine.Scenario{suites.S3_FormattedMessages()}
	case "link-preview":
		scenarios = []engine.Scenario{suites.S37_LinkPreview()}
	case "core":
		scenarios = suites.AllPhaseAScenarios()
	// Phase B: Media
	case "media":
		scenarios = suites.AllPhaseBScenarios()
	case "media-uploads":
		scenarios = []engine.Scenario{suites.S6_MediaUploads()}
	case "media-groups":
		scenarios = []engine.Scenario{suites.S7_MediaGroups()}
	case "edit-media":
		scenarios = []engine.Scenario{suites.S8_EditMedia()}
	case "get-file":
		scenarios = []engine.Scenario{suites.S9_GetFile()}
	case "edit-message-media":
		scenarios = []engine.Scenario{suites.S11_EditMessageMedia()}
	case "multipart-parsemode":
		scenarios = []engine.Scenario{suites.S36_MultipartParseMode()}
	// Phase C: Keyboards
	case "keyboards":
		scenarios = suites.AllPhaseCScenarios()
	case "inline-keyboard":
		scenarios = []engine.Scenario{suites.S10_InlineKeyboard()}
	// Tier 2: Chat Administration
	case "chat-admin":
		scenarios = suites.AllChatAdminScenarios()
	case "chat-info":
		scenarios = []engine.Scenario{suites.S15_ChatInfo()}
	case "chat-settings":
		scenarios = []engine.Scenario{suites.S16_ChatSettings()}
	case "pin-messages":
		scenarios = []engine.Scenario{suites.S17_PinMessages()}
	case "polls":
		scenarios = []engine.Scenario{suites.S18_Polls()}
	case "forum-stickers":
		scenarios = []engine.Scenario{suites.S19_ForumStickers()}
	// Extended: Stickers
	case "stickers":
		scenarios = suites.AllStickerScenarios()
	case "sticker-lifecycle":
		scenarios = []engine.Scenario{suites.S20_StickerLifecycle()}
	// Extended: Stars & Payments
	case "stars":
		scenarios = suites.AllStarsScenarios()
	case "star-balance":
		scenarios = []engine.Scenario{suites.S21_Stars()}
	case "invoice":
		scenarios = []engine.Scenario{suites.S22_Invoice()}
	// Extended: Gifts
	case "gifts":
		scenarios = suites.AllGiftScenarios()
	// Extended: Checklists
	case "checklists":
		scenarios = suites.AllChecklistScenarios()
	// Interactive (opt-in, excluded from "all")
	case "interactive":
		runInteractiveSuite(cfg, senderClient, logger)
		return
	case "callback":
		runInteractiveSuite(cfg, senderClient, logger)
		return
	// Webhook management (opt-in, excluded from "all")
	case "webhook":
		scenarios = suites.AllWebhookScenarios()
	case "webhook-lifecycle":
		scenarios = []engine.Scenario{suites.S13_WebhookLifecycle()}
	case "get-updates":
		scenarios = []engine.Scenario{suites.S14_GetUpdates()}
	// Extras (S25-S32)
	case "extras":
		scenarios = suites.AllExtrasScenarios()
	case "geo":
		scenarios = []engine.Scenario{suites.S25_GeoLocation()}
	case "venue":
		scenarios = []engine.Scenario{suites.S26_GeoVenue()}
	case "contact-dice":
		scenarios = []engine.Scenario{suites.S27_ContactAndDice()}
	case "bulk":
		scenarios = []engine.Scenario{suites.S28_BulkOps()}
	case "reactions":
		scenarios = []engine.Scenario{suites.S29_Reactions()}
	case "user-info":
		scenarios = []engine.Scenario{suites.S30_UserInfo()}
	case "chat-photo":
		scenarios = []engine.Scenario{suites.S31_ChatPhotoLifecycle()}
	case "chat-permissions":
		scenarios = []engine.Scenario{suites.S32_ChatPermissionsLifecycle()}
	// Bot Identity (S33-S35)
	case "bot-config":
		scenarios = suites.AllBotConfigScenarios()
	case "bot-commands":
		scenarios = []engine.Scenario{suites.S33_BotCommands()}
	case "bot-profile":
		scenarios = []engine.Scenario{suites.S34_BotProfile()}
	case "bot-admin-defaults":
		scenarios = []engine.Scenario{suites.S35_BotAdminDefaults()}
	// Bot API 9.4 (S38-S42)
	case "api94", "api-94", "9.4":
		scenarios = suites.AllAPI94Scenarios()
	case "styled-buttons":
		scenarios = []engine.Scenario{suites.S38_StyledButtons()}
	case "profile-audios":
		scenarios = []engine.Scenario{suites.S39_ProfileAudios()}
	case "chat-info-94":
		scenarios = []engine.Scenario{suites.S40_ChatInfo94()}
	case "video-qualities":
		scenarios = []engine.Scenario{suites.S42_VideoQualities()}
	case "all":
		scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
		scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
		scenarios = append(scenarios, suites.AllChatAdminScenarios()...)
		scenarios = append(scenarios, suites.AllStickerScenarios()...)
		scenarios = append(scenarios, suites.AllStarsScenarios()...)
		scenarios = append(scenarios, suites.AllGiftScenarios()...)
		scenarios = append(scenarios, suites.AllExtrasScenarios()...)
		scenarios = append(scenarios, suites.AllBotConfigScenarios()...)
		scenarios = append(scenarios, suites.AllAPI94Scenarios()...)
		// Checklists require Telegram Premium — opt-in via --run checklists
	default:
		logger.Error("unknown suite", "suite", suite)
		fmt.Println("Available suites: smoke, identity, messages, forward, actions, core, media, media-uploads, media-groups, edit-media, get-file, edit-message-media, keyboards, inline-keyboard, chat-admin, chat-info, chat-settings, pin-messages, polls, forum-stickers, stickers, sticker-lifecycle, stars, star-balance, invoice, gifts, checklists, interactive, callback, webhook, webhook-lifecycle, get-updates, extras, geo, venue, contact-dice, bulk, reactions, user-info, chat-photo, chat-permissions, bot-config, bot-commands, bot-profile, bot-admin-defaults, api94, styled-buttons, profile-audios, chat-info-94, video-qualities, all")
		os.Exit(1)
	}

	report := evidence.NewReport()

	ctx := context.Background()
	for _, scenario := range scenarios {
		logger.Info("running scenario", "name", scenario.Name())
		result := runner.Run(ctx, scenario)
		report.AddScenario(result)

		if !result.Success {
			logger.Error("scenario failed", "name", scenario.Name(), "error", result.Error)
		}
	}

	report.Finalize()

	// Save report
	filename, err := report.Save(cfg.StorageDir)
	if err != nil {
		logger.Error("failed to save report", "error", err)
	} else {
		logger.Info("report saved", "filename", filename)
	}

	// Print summary
	fmt.Println("\n" + report.FormatSummary())

	if !report.Success {
		os.Exit(1)
	}
}

// runInteractiveSuite runs interactive scenarios that require user interaction.
// It starts a polling loop to receive callback queries from Telegram.
func runInteractiveSuite(cfg *config.Config, senderClient *sender.Client, logger *slog.Logger) {
	logger.Info("starting interactive test suite (requires user interaction)")

	// Start polling to receive callback queries
	updates := make(chan tg.Update, 100)
	receiverCfg := receiver.DefaultConfig()
	receiverCfg.Mode = receiver.ModeLongPolling
	receiverCfg.PollingTimeout = 30

	pollingClient := receiver.NewPollingClient(
		tg.SecretToken(cfg.Token),
		updates,
		logger,
		receiverCfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := pollingClient.Start(ctx); err != nil {
		logger.Error("failed to start polling for interactive suite", "error", err)
		os.Exit(1)
	}
	defer pollingClient.Stop()

	// Create runtime with callback channel
	adapter := engine.NewSenderAdapter(senderClient).WithToken(tg.SecretToken(cfg.Token))
	callbackChan := make(chan *tg.CallbackQuery, 10)
	rt := engine.NewRuntime(adapter, cfg.ChatID, cfg.Admins[0])
	rt.CallbackChan = callbackChan
	runner := engine.NewRunner(rt, engine.RunnerConfig{
		BaseDelay:     cfg.SendInterval,
		Jitter:        cfg.JitterInterval,
		MaxMessages:   cfg.MaxMessagesPerRun,
		RetryOn429:    cfg.RetryOn429,
		Max429Retries: cfg.Max429Retries,
	}, logger)

	// Forward callback queries from polling to the runtime channel
	go func() {
		for update := range updates {
			if update.CallbackQuery != nil {
				select {
				case callbackChan <- update.CallbackQuery:
					logger.Debug("forwarded callback query", "id", update.CallbackQuery.ID, "data", update.CallbackQuery.Data)
				default:
					logger.Warn("callback channel full, dropping query")
				}
			}
		}
	}()

	scenarios := suites.AllInteractiveScenarios()

	fmt.Printf("Running %d interactive scenario(s)...\n", len(scenarios))
	fmt.Println("Please interact with the bot in the chat when prompted.")

	report := evidence.NewReport()

	for _, scenario := range scenarios {
		logger.Info("running scenario", "name", scenario.Name())
		result := runner.Run(ctx, scenario)
		report.AddScenario(result)

		if !result.Success {
			logger.Error("scenario failed", "name", scenario.Name(), "error", result.Error)
		}
	}

	report.Finalize()

	// Save report
	filename, err := report.Save(cfg.StorageDir)
	if err != nil {
		logger.Error("failed to save report", "error", err)
	} else {
		logger.Info("report saved", "filename", filename)
	}

	fmt.Println("\n" + report.FormatSummary())

	if !report.Success {
		os.Exit(1)
	}
}

func runInteractive(cfg *config.Config, senderClient *sender.Client, logger *slog.Logger) {
	logger.Info("starting interactive mode")

	// Create receiver for updates
	updates := make(chan tg.Update, 100)
	receiverCfg := receiver.DefaultConfig()
	receiverCfg.Mode = receiver.ModeLongPolling
	receiverCfg.PollingTimeout = 30

	pollingClient := receiver.NewPollingClient(
		tg.SecretToken(cfg.Token),
		updates,
		logger,
		receiverCfg,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
		pollingClient.Stop()
	}()

	// Start polling
	if err := pollingClient.Start(ctx); err != nil {
		logger.Error("failed to start polling", "error", err)
		os.Exit(1)
	}

	adapter := engine.NewSenderAdapter(senderClient).WithToken(tg.SecretToken(cfg.Token))
	cleaner := cleanup.NewCleaner(adapter, logger)

	logger.Info("listening for commands",
		"commands", "/run, /status, /cleanup, /help")

	// Process updates
	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		msg := update.Message
		userID := int64(0)
		if msg.From != nil {
			userID = msg.From.ID
		}

		// Check admin
		if !cfg.IsAdmin(userID) {
			logger.Warn("unauthorized user", "user_id", userID)
			continue
		}

		text := msg.Text
		if !strings.HasPrefix(text, "/") {
			continue
		}

		parts := strings.Fields(text)
		command := strings.TrimPrefix(parts[0], "/")
		args := ""
		if len(parts) > 1 {
			args = strings.Join(parts[1:], " ")
		}

		handleCommand(ctx, cfg, senderClient, adapter, cleaner, logger, msg.Chat.ID, command, args, updates)
	}
}

func handleCommand(ctx context.Context, cfg *config.Config, senderClient *sender.Client,
	adapter *engine.SenderAdapter, cleaner *cleanup.Cleaner, logger *slog.Logger,
	chatID int64, command, args string, updates <-chan tg.Update) {

	switch command {
	case "run":
		handleRun(ctx, cfg, senderClient, adapter, logger, chatID, args, updates)
	case "status":
		handleStatus(ctx, adapter, chatID)
	case "cleanup":
		// Not implemented yet - would need to track messages across runs
		sendMessage(ctx, adapter, chatID, "Cleanup: No messages tracked in this session")
	case "help":
		handleHelp(ctx, adapter, chatID)
	default:
		sendMessage(ctx, adapter, chatID, "Unknown command. Use /help")
	}
}

func handleRun(ctx context.Context, cfg *config.Config, senderClient *sender.Client,
	adapter *engine.SenderAdapter, logger *slog.Logger, chatID int64, suite string, updates <-chan tg.Update) {

	if suite == "" {
		sendMessage(ctx, adapter, chatID, "Usage: /run <suite>\nSuites: smoke, identity, messages, forward, actions, core, media, media-uploads, media-groups, edit-media, get-file, edit-message-media, keyboards, inline-keyboard, interactive, webhook, get-updates, all")
		return
	}

	rt := engine.NewRuntime(adapter, chatID, cfg.Admins[0])
	runner := engine.NewRunner(rt, engine.RunnerConfig{
		BaseDelay:     cfg.SendInterval,
		Jitter:        cfg.JitterInterval,
		MaxMessages:   cfg.MaxMessagesPerRun,
		RetryOn429:    cfg.RetryOn429,
		Max429Retries: cfg.Max429Retries,
	}, logger)

	var scenarios []engine.Scenario

	switch strings.ToLower(suite) {
	case "smoke":
		scenarios = []engine.Scenario{suites.S0_Smoke()}
	case "identity":
		scenarios = []engine.Scenario{suites.S1_Identity()}
	case "messages":
		scenarios = []engine.Scenario{suites.S2_MessageLifecycle()}
	case "forward":
		scenarios = []engine.Scenario{suites.S4_ForwardCopy()}
	case "actions":
		scenarios = []engine.Scenario{suites.S5_ChatAction()}
	case "formatted", "formatting", "parse-mode":
		scenarios = []engine.Scenario{suites.S3_FormattedMessages()}
	case "link-preview":
		scenarios = []engine.Scenario{suites.S37_LinkPreview()}
	case "core":
		scenarios = suites.AllPhaseAScenarios()
	// Phase B: Media
	case "media":
		scenarios = suites.AllPhaseBScenarios()
	case "media-uploads":
		scenarios = []engine.Scenario{suites.S6_MediaUploads()}
	case "media-groups":
		scenarios = []engine.Scenario{suites.S7_MediaGroups()}
	case "edit-media":
		scenarios = []engine.Scenario{suites.S8_EditMedia()}
	case "get-file":
		scenarios = []engine.Scenario{suites.S9_GetFile()}
	case "edit-message-media":
		scenarios = []engine.Scenario{suites.S11_EditMessageMedia()}
	case "multipart-parsemode":
		scenarios = []engine.Scenario{suites.S36_MultipartParseMode()}
	// Phase C: Keyboards
	case "keyboards":
		scenarios = suites.AllPhaseCScenarios()
	case "inline-keyboard":
		scenarios = []engine.Scenario{suites.S10_InlineKeyboard()}
	// Extended: Stickers
	case "stickers":
		scenarios = suites.AllStickerScenarios()
	case "sticker-lifecycle":
		scenarios = []engine.Scenario{suites.S20_StickerLifecycle()}
	case "stars":
		scenarios = suites.AllStarsScenarios()
	case "star-balance":
		scenarios = []engine.Scenario{suites.S21_Stars()}
	case "invoice":
		scenarios = []engine.Scenario{suites.S22_Invoice()}
	case "gifts":
		scenarios = suites.AllGiftScenarios()
	case "checklists":
		scenarios = suites.AllChecklistScenarios()
	// Interactive (opt-in, requires user interaction)
	case "interactive", "callback":
		scenarios = suites.AllInteractiveScenarios()
		// Wire callback channel for interactive scenarios
		callbackChan := make(chan *tg.CallbackQuery, 10)
		rt.CallbackChan = callbackChan
		go func() {
			for update := range updates {
				if update.CallbackQuery != nil {
					select {
					case callbackChan <- update.CallbackQuery:
					default:
					}
				}
			}
		}()
	// Webhook management (opt-in)
	case "webhook":
		scenarios = suites.AllWebhookScenarios()
	case "webhook-lifecycle":
		scenarios = []engine.Scenario{suites.S13_WebhookLifecycle()}
	case "get-updates":
		scenarios = []engine.Scenario{suites.S14_GetUpdates()}
	// Tier 2
	case "chat-admin":
		scenarios = suites.AllChatAdminScenarios()
	case "chat-info":
		scenarios = []engine.Scenario{suites.S15_ChatInfo()}
	case "chat-settings":
		scenarios = []engine.Scenario{suites.S16_ChatSettings()}
	case "pin-messages":
		scenarios = []engine.Scenario{suites.S17_PinMessages()}
	case "polls":
		scenarios = []engine.Scenario{suites.S18_Polls()}
	case "forum-stickers":
		scenarios = []engine.Scenario{suites.S19_ForumStickers()}
	// Extras (S25-S32)
	case "extras":
		scenarios = suites.AllExtrasScenarios()
	case "geo":
		scenarios = []engine.Scenario{suites.S25_GeoLocation()}
	case "venue":
		scenarios = []engine.Scenario{suites.S26_GeoVenue()}
	case "contact-dice":
		scenarios = []engine.Scenario{suites.S27_ContactAndDice()}
	case "bulk":
		scenarios = []engine.Scenario{suites.S28_BulkOps()}
	case "reactions":
		scenarios = []engine.Scenario{suites.S29_Reactions()}
	case "user-info":
		scenarios = []engine.Scenario{suites.S30_UserInfo()}
	case "chat-photo":
		scenarios = []engine.Scenario{suites.S31_ChatPhotoLifecycle()}
	case "chat-permissions":
		scenarios = []engine.Scenario{suites.S32_ChatPermissionsLifecycle()}
	// Bot Identity (S33-S35)
	case "bot-config":
		scenarios = suites.AllBotConfigScenarios()
	case "bot-commands":
		scenarios = []engine.Scenario{suites.S33_BotCommands()}
	case "bot-profile":
		scenarios = []engine.Scenario{suites.S34_BotProfile()}
	case "bot-admin-defaults":
		scenarios = []engine.Scenario{suites.S35_BotAdminDefaults()}
	// Bot API 9.4 (S38-S42)
	case "api94", "api-94", "9.4":
		scenarios = suites.AllAPI94Scenarios()
	case "styled-buttons":
		scenarios = []engine.Scenario{suites.S38_StyledButtons()}
	case "profile-audios":
		scenarios = []engine.Scenario{suites.S39_ProfileAudios()}
	case "chat-info-94":
		scenarios = []engine.Scenario{suites.S40_ChatInfo94()}
	case "video-qualities":
		scenarios = []engine.Scenario{suites.S42_VideoQualities()}
	case "all":
		scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
		scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
		scenarios = append(scenarios, suites.AllChatAdminScenarios()...)
		scenarios = append(scenarios, suites.AllStickerScenarios()...)
		scenarios = append(scenarios, suites.AllStarsScenarios()...)
		scenarios = append(scenarios, suites.AllGiftScenarios()...)
		scenarios = append(scenarios, suites.AllExtrasScenarios()...)
		scenarios = append(scenarios, suites.AllBotConfigScenarios()...)
		scenarios = append(scenarios, suites.AllAPI94Scenarios()...)
		// Checklists require Telegram Premium — opt-in via --run checklists
	default:
		sendMessage(ctx, adapter, chatID, "Unknown suite: "+suite)
		return
	}

	sendMessage(ctx, adapter, chatID, fmt.Sprintf("Running %d scenario(s)...", len(scenarios)))

	report := evidence.NewReport()

	for _, scenario := range scenarios {
		result := runner.Run(ctx, scenario)
		report.AddScenario(result)
	}

	report.Finalize()

	// Save report
	filename, err := report.Save(cfg.StorageDir)
	if err != nil {
		logger.Error("failed to save report", "error", err)
	}

	// Send summary
	summary := report.FormatSummary()
	if filename != "" {
		summary += fmt.Sprintf("\nReport: %s", filename)
	}
	sendMessage(ctx, adapter, chatID, summary)
}

func handleStatus(ctx context.Context, adapter *engine.SenderAdapter, chatID int64) {
	scenarios := append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
	scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
	scenarios = append(scenarios, suites.AllChatAdminScenarios()...)
	scenarios = append(scenarios, suites.AllStickerScenarios()...)
	scenarios = append(scenarios, suites.AllStarsScenarios()...)
	scenarios = append(scenarios, suites.AllGiftScenarios()...)
	scenarios = append(scenarios, suites.AllChecklistScenarios()...)
	scenarios = append(scenarios, suites.AllInteractiveScenarios()...)
	scenarios = append(scenarios, suites.AllWebhookScenarios()...)
	scenarios = append(scenarios, suites.AllExtrasScenarios()...)
	scenarios = append(scenarios, suites.AllBotConfigScenarios()...)
	scenarios = append(scenarios, suites.AllAPI94Scenarios()...)

	coverers := make([]registry.Coverer, len(scenarios))
	for i, s := range scenarios {
		coverers[i] = s
	}

	report := registry.CheckCoverage(coverers)

	var sb strings.Builder
	sb.WriteString("Method Coverage (All Phases)\n\n")
	sb.WriteString(fmt.Sprintf("Covered: %d\n", len(report.Covered)))
	sb.WriteString(fmt.Sprintf("Missing: %d\n\n", len(report.Missing)))

	if len(report.Missing) > 0 {
		sb.WriteString("Missing methods:\n")
		for _, m := range report.Missing {
			sb.WriteString(fmt.Sprintf("  - %s\n", m))
		}
	}

	sendMessage(ctx, adapter, chatID, sb.String())
}

func handleHelp(ctx context.Context, adapter *engine.SenderAdapter, chatID int64) {
	help := `galigo-testbot Commands:

/run <suite> - Run test suite

Phase A (Core):
  smoke    - Quick sanity check
  identity - Bot identity (getMe)
  messages - Send, edit, delete
  forward  - Forward and copy
  actions  - Chat actions (typing)
  core     - All Phase A tests

Phase B (Media):
  media         - All media tests
  media-uploads - Photo, document, animation, audio, voice
  media-groups  - Albums
  edit-media    - Edit captions
  get-file      - File download info

Phase C (Keyboards):
  keyboards       - All keyboard tests
  inline-keyboard - Inline keyboard + edit markup

Extended:
  stickers          - Sticker set lifecycle
  sticker-lifecycle - Full sticker CRUD (S20)
  stars             - Star balance + transactions (S21-S22)
  star-balance      - Star balance only (S21)
  invoice           - Send invoice (S22)
  gifts             - Gift catalog (S23)
  checklists        - Checklist lifecycle (S24)

Bot API 9.4 (S38-S42):
  api94           - All 9.4 tests
  styled-buttons  - Button styling (S38)
  profile-audios  - getUserProfileAudios (S39)
  chat-info-94    - ChatFullInfo 9.4 fields (S40)
  video-qualities - Video qualities field (S42)

Interactive (opt-in, excluded from "all"):
  interactive - Callback query tests (requires user click)
  callback    - Alias for interactive

Webhook (opt-in, excluded from "all"):
  webhook           - All webhook tests (S13+S14)
  webhook-lifecycle - Set, verify, delete webhook
  get-updates       - Non-blocking getUpdates call

  all      - All non-interactive, non-webhook tests

/status - Show method coverage

/help - Show this help`

	sendMessage(ctx, adapter, chatID, help)
}

func sendMessage(ctx context.Context, adapter *engine.SenderAdapter, chatID int64, text string) {
	_, _ = adapter.SendMessage(ctx, chatID, text)
}
