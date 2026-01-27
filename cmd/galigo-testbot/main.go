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
	// Combine all scenarios from all phases
	scenarios := append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
	scenarios = append(scenarios, suites.AllPhaseCScenarios()...)

	// Convert to Coverer interface
	coverers := make([]registry.Coverer, len(scenarios))
	for i, s := range scenarios {
		coverers[i] = s
	}

	report := registry.CheckCoverage(coverers)

	fmt.Println("Method Coverage Status (All Phases: A, B, C)")
	fmt.Println("=============================================")
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
	adapter := engine.NewSenderAdapter(senderClient)
	rt := engine.NewRuntime(adapter, cfg.ChatID)
	runner := engine.NewRunner(rt, cfg.SendInterval, cfg.MaxMessagesPerRun, logger)

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
	// Phase C: Keyboards
	case "keyboards":
		scenarios = suites.AllPhaseCScenarios()
	case "inline-keyboard":
		scenarios = []engine.Scenario{suites.S10_InlineKeyboard()}
	case "all":
		scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
		scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
	default:
		logger.Error("unknown suite", "suite", suite)
		fmt.Println("Available suites: smoke, identity, messages, forward, actions, core, media, media-uploads, media-groups, edit-media, get-file, keyboards, inline-keyboard, all")
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

	adapter := engine.NewSenderAdapter(senderClient)
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

		handleCommand(ctx, cfg, senderClient, adapter, cleaner, logger, msg.Chat.ID, command, args)
	}
}

func handleCommand(ctx context.Context, cfg *config.Config, senderClient *sender.Client,
	adapter *engine.SenderAdapter, cleaner *cleanup.Cleaner, logger *slog.Logger,
	chatID int64, command, args string) {

	switch command {
	case "run":
		handleRun(ctx, cfg, senderClient, adapter, logger, chatID, args)
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
	adapter *engine.SenderAdapter, logger *slog.Logger, chatID int64, suite string) {

	if suite == "" {
		sendMessage(ctx, adapter, chatID, "Usage: /run <suite>\nSuites: smoke, identity, messages, forward, actions, core, media, media-uploads, media-groups, edit-media, get-file, keyboards, inline-keyboard, all")
		return
	}

	rt := engine.NewRuntime(adapter, chatID)
	runner := engine.NewRunner(rt, cfg.SendInterval, cfg.MaxMessagesPerRun, logger)

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
	// Phase C: Keyboards
	case "keyboards":
		scenarios = suites.AllPhaseCScenarios()
	case "inline-keyboard":
		scenarios = []engine.Scenario{suites.S10_InlineKeyboard()}
	case "all":
		scenarios = append(suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios()...)
		scenarios = append(scenarios, suites.AllPhaseCScenarios()...)
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

  all      - All tests

/status - Show method coverage

/help - Show this help`

	sendMessage(ctx, adapter, chatID, help)
}

func sendMessage(ctx context.Context, adapter *engine.SenderAdapter, chatID int64, text string) {
	_, _ = adapter.SendMessage(ctx, chatID, text)
}
