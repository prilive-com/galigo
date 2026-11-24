package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/config"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/suites"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// b49Routes is the expected selection of every label suiteScenarios accepts (B-49): the group function or the
// scenario constructors each label names. It checks the selector's choice; the contents of a group are the suites'
// own tests' job. TestB49_SuiteLabelsComplete keeps it in step with the switch.
var b49Routes = []struct {
	label string
	want  func() []engine.Scenario
}{
	{"smoke", func() []engine.Scenario { return []engine.Scenario{suites.S0_Smoke()} }},
	{"identity", func() []engine.Scenario { return []engine.Scenario{suites.S1_Identity()} }},
	{"messages", func() []engine.Scenario { return []engine.Scenario{suites.S2_MessageLifecycle()} }},
	{"forward", func() []engine.Scenario { return []engine.Scenario{suites.S4_ForwardCopy()} }},
	{"actions", func() []engine.Scenario { return []engine.Scenario{suites.S5_ChatAction()} }},
	{"formatted", func() []engine.Scenario { return []engine.Scenario{suites.S3_FormattedMessages()} }},
	{"formatting", func() []engine.Scenario { return []engine.Scenario{suites.S3_FormattedMessages()} }},
	{"parse-mode", func() []engine.Scenario { return []engine.Scenario{suites.S3_FormattedMessages()} }},
	{"link-preview", func() []engine.Scenario { return []engine.Scenario{suites.S37_LinkPreview()} }},
	{"core", suites.AllPhaseAScenarios},
	{"media", suites.AllPhaseBScenarios},
	{"media-uploads", func() []engine.Scenario { return []engine.Scenario{suites.S6_MediaUploads()} }},
	{"media-groups", func() []engine.Scenario { return []engine.Scenario{suites.S7_MediaGroups()} }},
	{"edit-media", func() []engine.Scenario { return []engine.Scenario{suites.S8_EditMedia()} }},
	{"get-file", func() []engine.Scenario { return []engine.Scenario{suites.S9_GetFile()} }},
	{"edit-message-media", func() []engine.Scenario { return []engine.Scenario{suites.S11_EditMessageMedia()} }},
	{"multipart-parsemode", func() []engine.Scenario { return []engine.Scenario{suites.S36_MultipartParseMode()} }},
	{"media-file-id", func() []engine.Scenario {
		return []engine.Scenario{suites.S55_MediaGroupByFileID(), suites.S56_EditMessageMediaByFileID()}
	}},
	{"live-photos", suites.AllLivePhotoScenarios},
	{"keyboards", suites.AllPhaseCScenarios},
	{"inline-keyboard", func() []engine.Scenario { return []engine.Scenario{suites.S10_InlineKeyboard()} }},
	{"chat-admin", suites.AllChatAdminScenarios},
	{"chat-info", func() []engine.Scenario { return []engine.Scenario{suites.S15_ChatInfo()} }},
	{"chat-settings", func() []engine.Scenario { return []engine.Scenario{suites.S16_ChatSettings()} }},
	{"pin-messages", func() []engine.Scenario { return []engine.Scenario{suites.S17_PinMessages()} }},
	{"polls", func() []engine.Scenario { return []engine.Scenario{suites.S18_Polls()} }},
	{"forum-stickers", func() []engine.Scenario { return []engine.Scenario{suites.S19_ForumStickers()} }},
	{"stickers", suites.AllStickerScenarios},
	{"sticker-lifecycle", func() []engine.Scenario { return []engine.Scenario{suites.S20_StickerLifecycle()} }},
	{"stars", suites.AllStarsScenarios},
	{"star-balance", func() []engine.Scenario { return []engine.Scenario{suites.S21_Stars()} }},
	{"paid-media", func() []engine.Scenario { return []engine.Scenario{suites.S70_PaidMedia()} }},
	{"suggested-posts", suites.AllSuggestedPostScenarios},
	{"ephemeral", suites.AllEphemeralScenarios},
	{"invoice", func() []engine.Scenario { return []engine.Scenario{suites.S22_Invoice()} }},
	{"gifts", suites.AllGiftScenarios},
	{"checklists", suites.AllChecklistScenarios},
	{"webhook", suites.AllWebhookScenarios},
	{"webhook-lifecycle", func() []engine.Scenario { return []engine.Scenario{suites.S13_WebhookLifecycle()} }},
	{"get-updates", func() []engine.Scenario { return []engine.Scenario{suites.S14_GetUpdates()} }},
	{"extras", suites.AllExtrasScenarios},
	{"geo", func() []engine.Scenario { return []engine.Scenario{suites.S25_GeoLocation()} }},
	{"venue", func() []engine.Scenario { return []engine.Scenario{suites.S26_GeoVenue()} }},
	{"contact-dice", func() []engine.Scenario { return []engine.Scenario{suites.S27_ContactAndDice()} }},
	{"bulk", func() []engine.Scenario { return []engine.Scenario{suites.S28_BulkOps()} }},
	{"reactions", func() []engine.Scenario { return []engine.Scenario{suites.S29_Reactions()} }},
	{"user-info", func() []engine.Scenario { return []engine.Scenario{suites.S30_UserInfo()} }},
	{"chat-photo", func() []engine.Scenario { return []engine.Scenario{suites.S31_ChatPhotoLifecycle()} }},
	{"chat-permissions", func() []engine.Scenario { return []engine.Scenario{suites.S32_ChatPermissionsLifecycle()} }},
	{"bot-config", suites.AllBotConfigScenarios},
	{"bot-commands", func() []engine.Scenario { return []engine.Scenario{suites.S33_BotCommands()} }},
	{"bot-profile", func() []engine.Scenario { return []engine.Scenario{suites.S34_BotProfile()} }},
	{"bot-admin-defaults", func() []engine.Scenario { return []engine.Scenario{suites.S35_BotAdminDefaults()} }},
	{"api94", suites.AllAPI94Scenarios},
	{"api-94", suites.AllAPI94Scenarios},
	{"9.4", suites.AllAPI94Scenarios},
	{"styled-buttons", func() []engine.Scenario { return []engine.Scenario{suites.S38_StyledButtons()} }},
	{"profile-audios", func() []engine.Scenario { return []engine.Scenario{suites.S39_ProfileAudios()} }},
	{"chat-info-94", func() []engine.Scenario { return []engine.Scenario{suites.S40_ChatInfo94()} }},
	{"video-qualities", func() []engine.Scenario { return []engine.Scenario{suites.S42_VideoQualities()} }},
	{"api95", suites.AllAPI95Scenarios},
	{"api-95", suites.AllAPI95Scenarios},
	{"9.5", suites.AllAPI95Scenarios},
	{"api96", suites.AllAPI96Scenarios},
	{"api-96", suites.AllAPI96Scenarios},
	{"9.6", suites.AllAPI96Scenarios},
	{"api100", suites.AllAPI100Scenarios},
	{"api-100", suites.AllAPI100Scenarios},
	{"10.0", suites.AllAPI100Scenarios},
	{"api101", suites.AllAPI101Scenarios},
	{"api-101", suites.AllAPI101Scenarios},
	{"10.1", suites.AllAPI101Scenarios},
	{"api102", suites.AllAPI102Scenarios},
	{"api-102", suites.AllAPI102Scenarios},
	{"10.2", suites.AllAPI102Scenarios},
	{"api103", suites.AllAPI103Scenarios},
	{"api-103", suites.AllAPI103Scenarios},
	{"10.3", suites.AllAPI103Scenarios},
	{"api104", suites.AllAPI104Scenarios}, // B-65 65f
	{"api-104", suites.AllAPI104Scenarios},
	{"10.4", suites.AllAPI104Scenarios},
	{"all", func() []engine.Scenario {
		var s []engine.Scenario
		for _, g := range [][]engine.Scenario{
			suites.AllPhaseAScenarios(), suites.AllPhaseBScenarios(), suites.AllPhaseCScenarios(),
			suites.AllChatAdminScenarios(), suites.AllStickerScenarios(), suites.AllStarsScenarios(),
			suites.AllGiftScenarios(), suites.AllExtrasScenarios(), suites.AllBotConfigScenarios(),
			suites.AllAPI94Scenarios(), suites.AllAPI95Scenarios(), suites.AllAPI96Scenarios(),
			suites.AllAPI100Scenarios(), suites.AllAPI101Scenarios(), suites.AllAPI102Scenarios(),
			suites.AllAPI103Scenarios(), suites.AllSuggestedPostScenarios(), suites.AllEphemeralScenarios(),
			suites.AllAPI104Scenarios(), // B-65 65f
		} {
			s = append(s, g...)
		}
		return s
	}},
}

// b49SuiteList is the expected text after "Available suites: " (B-126: every name the CLI and the bot accept), kept
// apart from availableSuites so a changed list is seen; docs/testing.md step 5 updates both.
const b49SuiteList = "smoke, identity, messages, forward, actions, formatted, formatting, parse-mode, link-preview, core, media, media-uploads, media-groups, edit-media, get-file, edit-message-media, multipart-parsemode, media-file-id, keyboards, inline-keyboard, chat-admin, chat-info, chat-settings, pin-messages, polls, forum-stickers, stickers, sticker-lifecycle, stars, star-balance, invoice, paid-media, suggested-posts, ephemeral, gifts, checklists, interactive, callback, webhook, webhook-lifecycle, get-updates, extras, geo, venue, contact-dice, bulk, reactions, user-info, chat-photo, chat-permissions, bot-config, bot-commands, bot-profile, bot-admin-defaults, api94, api-94, 9.4, styled-buttons, profile-audios, chat-info-94, video-qualities, api95, api-95, 9.5, api96, api-96, 9.6, api100, api-100, 10.0, api101, api-101, 10.1, api102, api-102, 10.2, api103, api-103, 10.3, api104, api-104, 10.4, live-photos, all"

func b49Names(ss []engine.Scenario) []string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = s.Name()
	}
	return names
}

// b49Unicode holds Unicode upper-case inputs per label. The selector lower-cases with strings.ToLower, like the
// baseline switch tag, so U+0130 maps to "i" and the Kelvin sign U+212A to "k" (register A-C1-01).
var b49Unicode = map[string][]string{"identity": {"\u0130dentity"}, "smoke": {"SMO\u212aE"}}

// b49Want returns the expected scenarios of the b49Routes row with this label, and fails the test when there is none
// (register O-C3-02, A-C3-01: a row's position or a renamed label must not pick or skip a row silently).
func b49Want(t *testing.T, label string) func() []engine.Scenario {
	t.Helper()
	for _, r := range b49Routes {
		if r.label == label {
			return r.want
		}
	}
	t.Fatalf("no b49Routes row %q", label)
	return nil
}

// T-route: every accepted label, in lower and upper case (and b49Unicode), selects the expected scenarios in order.
func TestB49_SuiteRoutes(t *testing.T) {
	for label := range b49Unicode {
		b49Want(t, label)
	}
	for _, r := range b49Routes {
		want := b49Names(r.want())
		require.NotEmpty(t, want, r.label)
		ins := append([]string{r.label}, b49Unicode[r.label]...)
		if up := strings.ToUpper(r.label); up != r.label {
			ins = append(ins, up)
		}
		for _, in := range ins {
			got, ok := suiteScenarios(in)
			assert.True(t, ok, in)
			assert.Equal(t, want, b49Names(got), in)
		}
	}
}

// T-routes (B-65 65d): the 10.3 labels select S74 then S75, and all holds the pair in that order; the names are
// written out here, not taken from the builders, so S75 missing from AllAPI103Scenarios fails this row.
func TestB65D_SuiteRoutesAPI103(t *testing.T) {
	pair := []string{"S74-RichAPI103", "S75-ReplyMarkupAPI103"}
	for _, in := range []string{"api103", "api-103", "10.3"} {
		got, ok := suiteScenarios(in)
		require.True(t, ok, in)
		assert.Equal(t, pair, b49Names(got), in)
	}
	got, ok := suiteScenarios("all")
	require.True(t, ok)
	names := b49Names(got)
	i := slices.Index(names, pair[0])
	require.GreaterOrEqual(t, i, 0, "all has no S74")
	require.Less(t, i+1, len(names), "all ends at S74")
	assert.Equal(t, pair, names[i:i+2])
	assert.Equal(t, 1, strings.Count(strings.Join(names, ","), pair[1]))
}

// T-refuse: names suiteScenarios does not select, including the two each caller runs itself.
func TestB49_SuiteRefused(t *testing.T) {
	for _, in := range []string{"", " smoke", "smoke ", "smoke\t", "\tsmoke", "smoke\n", "smoke\r", "smoke\u00a0", "\u00a0smoke", "\nsmoke",
		"\rsmoke", "nosuch",
		"interactive", "callback", "INTERACTIVE", "Callback"} {
		got, ok := suiteScenarios(in)
		assert.False(t, ok, "%q", in)
		assert.Nil(t, got, "%q", in)
	}
}

// T-interactive: isInteractiveSuite is true exactly for interactive and callback, in any case.
func TestB49_InteractiveSuite(t *testing.T) {
	// "\u0130nteractive" starts with U+0130, which strings.ToLower maps to "i" (the baseline ran it as interactive).
	for _, in := range []string{"interactive", "callback", "INTERACTIVE", "Callback", "InterActive", "\u0130nteractive"} {
		assert.True(t, isInteractiveSuite(in), in)
	}
	for _, in := range []string{"", " interactive", "interactive ", "callback ", "interactive\t", "\tcallback", "interactive\n",
		"interactive\r", "callback\u00a0", "\u00a0interactive", "\ninteractive", "\rcallback", "interactive2", "interactives", "call-back"} {
		assert.False(t, isInteractiveSuite(in), "%q", in)
	}
	for _, r := range b49Routes {
		assert.False(t, isInteractiveSuite(r.label), r.label)
	}
}

// T-fresh: two calls give the same names and share no backing array, for every label.
func TestB49_SuiteFresh(t *testing.T) {
	marker := &engine.BaseScenario{ScenarioName: "b49-marker"}
	for _, r := range b49Routes {
		a, _ := suiteScenarios(r.label)
		b, _ := suiteScenarios(r.label)
		require.Equal(t, b49Names(a), b49Names(b), r.label)
		for i := range a {
			a[i] = marker
		}
		assert.NotContains(t, b49Names(b), "b49-marker", r.label)
	}
}

// T-complete: the case labels of suiteScenarios are exactly the table's labels, so a new label needs a row.
func TestB49_SuiteLabelsComplete(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	require.NoError(t, err)
	var labels []string
	found := 0
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Recv != nil || fd.Name.Name != "suiteScenarios" {
			continue
		}
		found++
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				lit, ok := e.(*ast.BasicLit)
				require.True(t, ok && lit.Kind == token.STRING, "case label is not a string literal")
				s, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				labels = append(labels, s)
			}
			return true
		})
	}
	require.Equal(t, 1, found)
	var want []string
	for _, r := range b49Routes {
		want = append(want, r.label)
	}
	sort.Strings(labels)
	sort.Strings(want)
	assert.Equal(t, want, labels)
}

// T-list: every name the CLI prints for an unknown suite routes.
func TestB49_AvailableSuitesRoute(t *testing.T) {
	assert.Equal(t, b49SuiteList, availableSuites)
	for _, n := range strings.Split(availableSuites, ", ") {
		_, ok := suiteScenarios(n)
		assert.True(t, ok || isInteractiveSuite(n), n)
	}
}

func b49Discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// b49ReportNames reads the scenario names of the one report a run saved under dir.
func b49ReportNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "reports"))
	require.NoError(t, err)
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	require.Len(t, files, 1)
	data, err := os.ReadFile(filepath.Join(dir, "reports", files[0]))
	require.NoError(t, err)
	var rep struct {
		Scenarios []struct {
			Name string `json:"scenario_name"`
		} `json:"scenarios"`
	}
	require.NoError(t, json.Unmarshal(data, &rep))
	names := make([]string, len(rep.Scenarios))
	for i, s := range rep.Scenarios {
		names[i] = s.Name
	}
	return names
}

type b49Input struct {
	in   string
	want func() []engine.Scenario
}

// b49Inputs is every name a caller accepts: each b49Routes label as given and, where the label has a letter, with
// its first letter upper-cased; and interactive and callback as given and in other cases.
func b49Inputs() []b49Input {
	var rows []b49Input
	for _, in := range []string{"interactive", "callback", "Interactive", "CALLBACK", "iNTERACTIVE", "cALLBACK"} {
		rows = append(rows, b49Input{in, suites.AllInteractiveScenarios})
	}
	for _, r := range b49Routes {
		rows = append(rows, b49Input{r.label, r.want})
		if up := strings.ToUpper(r.label[:1]) + r.label[1:]; up != r.label {
			rows = append(rows, b49Input{up, r.want})
		}
	}
	return rows
}

// b49RunBot runs /run <suite> from testutil.TestChatID with a message budget of 0, so every scenario stops before its
// first step: the only requests are the bot's own messages. See b49RunBotFrom.
func b49RunBot(t *testing.T, suite string) (string, []string) {
	t.Helper()
	return b49RunBotFrom(t, testutil.TestChatID, 0, suite)
}

// b49RunBotFrom runs /run <suite> through handleCommand from the given chat against the mock server, with the given
// message budget (B-156). It returns the report folder and the message texts. The admin (1) and the config chat
// (424242) differ from the command chat, and every reply must go to the command chat. The server and the client are
// closed when it returns, so the rows do not keep listeners and connections open (register REGISTER-change.md
// O-C1-02; the t.Cleanup closes run again and do nothing).
func b49RunBotFrom(t *testing.T, chat int64, budget int, suite string) (string, []string) {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendMessage", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyMessage(w, 1)
	})
	defer server.Close()
	client := testutil.NewTestClient(t, server.BaseURL())
	defer func() { _ = client.Close() }()
	adapter := engine.NewSenderAdapter(client)
	cfg := &config.Config{Admins: []int64{1}, ChatID: 424242, StorageDir: t.TempDir(), MaxMessagesPerRun: budget}
	updates := make(chan tg.Update)
	defer close(updates)
	handleCommand(context.Background(), cfg, client, adapter, nil, b49Discard(), chat, "run", suite, updates)
	var texts []string
	for _, c := range server.Captures() {
		require.Equal(t, "/bot"+testutil.TestToken+"/sendMessage", c.Path)
		var body struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		// strict: a /run reply is exactly the JSON the sender writes for chat_id and text, so an extra field (an
		// ephemeral receiver, a topic), a key in another case, a repeated key, null or data after the object fails
		require.NoError(t, json.Unmarshal(c.Body, &body), "B156-CHAT: the /run reply %s is one JSON object", c.Body)
		want, err := json.Marshal(body)
		require.NoError(t, err)
		require.Equal(t, string(want), string(c.Body), "B156-CHAT: the /run reply has only chat_id and text, once each")
		require.Equal(t, chat, body.ChatID, "B156-CHAT: the /run reply %q goes to the chat the command came from", body.Text)
		texts = append(texts, body.Text)
	}
	return cfg.StorageDir, texts
}

// T-bot: the bot's /run refuses an empty and an unknown name with one message and no report, and for every accepted
// label (and interactive) runs exactly the selected scenarios. The refusals include near misses of both conditions:
// spaces, a second word, a suffix (the bot joins the words after /run with one space).
func TestB49_BotRun(t *testing.T) {
	dir, texts := b49RunBot(t, "")
	require.Len(t, texts, 1)
	assert.True(t, strings.HasPrefix(texts[0], "Usage: /run <suite>\n"), texts[0])
	_, err := os.Stat(filepath.Join(dir, "reports"))
	assert.True(t, os.IsNotExist(err))

	for _, in := range []string{"nosuch", "NoSuch", " smoke", "smoke ", "smoke x",
		" interactive", "interactive ", "interactive x", "callback x", "interactive2"} {
		dir, texts := b49RunBot(t, in)
		assert.Equal(t, []string{"Unknown suite: " + in}, texts, in)
		_, err := os.Stat(filepath.Join(dir, "reports"))
		assert.True(t, os.IsNotExist(err), in)
	}

	for _, r := range b49Inputs() {
		want := b49Names(r.want())
		dir, texts := b49RunBot(t, r.in)
		require.Len(t, texts, 2, r.in)
		assert.Equal(t, "Running "+strconv.Itoa(len(want))+" scenario(s)...", texts[0], r.in)
		assert.True(t, strings.HasPrefix(texts[1], "Test Run: "), r.in)
		assert.Equal(t, want, b49ReportNames(t, dir), r.in)
	}
}

// T-cli: runSuiteCommand in a child process (it exits). An unknown name logs it, prints the list and exits 1; every
// accepted label runs exactly the selected scenarios (budget 0, so the run fails and exits 1 after saving the report).
func TestB49_CLIRun(t *testing.T) {
	// Child mode needs the marker argument that run adds after "--" (flag.Args), not only the environment: an
	// inherited B49_CLI_SUITE alone must not turn the parent test into a child (register O-T3-01).
	if a := flag.Args(); len(a) == 1 && a[0] == "b49-cli-child" {
		suite, ok := os.LookupEnv("B49_CLI_SUITE")
		dir := os.Getenv("B49_CLI_DIR")
		if !ok || !filepath.IsAbs(dir) {
			fmt.Fprintln(os.Stderr, "b49: child needs B49_CLI_SUITE and an absolute B49_CLI_DIR")
			os.Exit(3)
		}
		// The child calls runSuiteCommand only: never main (it reads .env with config.LoadDotEnv) or config.Load (it
		// reads the environment), so the child's config holds only the fake values below. The nil
		// client makes a call through the sender panic. The resolver below refuses DNS lookups as a backup only (a
		// hosts-file entry is read first): wrong versions that could reach runInteractiveSuite's own poller are kept
		// out of this test by the mutation tool's checked network rule (TEST-PLAN §3).
		net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("b49: no network in the CLI child")
		}}
		cfg := &config.Config{Admins: []int64{1}, StorageDir: dir, MaxMessagesPerRun: 0}
		runSuiteCommand(cfg, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)), suite)
		os.Exit(0)
	}
	run := func(suite string) (int, string, string, string) {
		dir, err := filepath.Abs(t.TempDir()) // the child needs an absolute folder, also with a relative TMPDIR (O-C2-02)
		require.NoError(t, err)
		exe, err := os.Executable()
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestB49_CLIRun$", "--", "b49-cli-child")
		cmd.Env = append(os.Environ(), "B49_CLI_SUITE="+suite, "B49_CLI_DIR="+dir)
		cmd.Dir = dir // the child's working folder is the temp folder too (register O-C1-06)
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err = cmd.Run()
		code := 0
		if err != nil {
			var ee *exec.ExitError
			require.ErrorAs(t, err, &ee, "%q", suite)
			code = ee.ExitCode()
		}
		return code, stdout.String(), stderr.String(), dir
	}

	for in, logged := range map[string]string{"nosuch": "nosuch", "NoSuch": "NoSuch", " smoke": `" smoke"`,
		"smoke ": `"smoke "`, "smoke\t": `"smoke\t"`} {
		code, out, logs, dir := run(in)
		assert.Equal(t, 1, code, "%q", in)
		assert.Equal(t, "Available suites: "+b49SuiteList+"\n", out, "%q", in)
		assert.Contains(t, logs, `level=ERROR msg="unknown suite" suite=`+logged+"\n", "%q", in)
		_, err := os.Stat(filepath.Join(dir, "reports"))
		assert.True(t, os.IsNotExist(err), "%q", in)
	}

	rows := []b49Input{{"Api101", b49Want(t, "api101")}, {"SMOKE", b49Want(t, "smoke")}}
	for _, r := range b49Routes {
		rows = append(rows, b49Input{r.label, r.want})
	}
	for _, r := range rows {
		code, out, _, dir := run(r.in)
		assert.Equal(t, 1, code, r.in)
		assert.NotContains(t, out, "Available suites:", r.in)
		assert.Contains(t, out, "Test Run: ", r.in)
		assert.Equal(t, b49Names(r.want()), b49ReportNames(t, dir), r.in)
	}
}

// T-seam: the CLI's interactive branch cannot run offline (runInteractiveSuite builds its own poller against the
// real API), so its shape is read from main.go's syntax tree (E-0112). In runSuiteCommand the statements before it
// are only the adapter, runtime and runner declarations, with no function literal; then `if isInteractiveSuite(suite)
// { runInteractiveSuite(cfg, senderClient, logger); return }` with no else; suite is never written or declared again
// (parentheses are looked through); runInteractiveSuite is named only in that call. Bypass forms it refuses (E-0172):
// a write to suite (also `(suite) = …` or one in a closure), a shadowing suite, another or narrowed condition, a
// statement before the call that can return, the branch moved after the selector, a call through a function value.
func TestB49_CLIInteractiveSeam(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.SkipObjectResolution)
	require.NoError(t, err)
	var fd *ast.FuncDecl
	for _, d := range f.Decls {
		if x, ok := d.(*ast.FuncDecl); ok && x.Recv == nil && x.Name.Name == "runSuiteCommand" {
			require.Nil(t, fd, "two runSuiteCommand")
			fd = x
		}
	}
	require.NotNil(t, fd)
	isIdent := func(e ast.Expr, name string) bool {
		id, ok := ast.Unparen(e).(*ast.Ident)
		return ok && id.Name == name
	}
	stmts := fd.Body.List
	i := 0
	for _, want := range []string{"adapter", "rt", "runner"} {
		require.Greater(t, len(stmts), i)
		as, ok := stmts[i].(*ast.AssignStmt)
		require.True(t, ok && as.Tok == token.DEFINE && len(as.Lhs) == 1 && isIdent(as.Lhs[0], want), "statement %d is not %s :=", i, want)
		ast.Inspect(as, func(n ast.Node) bool {
			_, lit := n.(*ast.FuncLit)
			assert.False(t, lit, "function literal in the %s declaration", want)
			return true
		})
		i++
	}
	require.Greater(t, len(stmts), i)
	is, ok := stmts[i].(*ast.IfStmt)
	require.True(t, ok, "statement after the runner is not an if")
	require.Nil(t, is.Init)
	require.Nil(t, is.Else)
	cond, ok := ast.Unparen(is.Cond).(*ast.CallExpr)
	require.True(t, ok && isIdent(cond.Fun, "isInteractiveSuite") && len(cond.Args) == 1 && isIdent(cond.Args[0], "suite"),
		"condition is not isInteractiveSuite(suite)")
	require.Len(t, is.Body.List, 2)
	es, ok := is.Body.List[0].(*ast.ExprStmt)
	require.True(t, ok)
	call, ok := ast.Unparen(es.X).(*ast.CallExpr)
	require.True(t, ok && isIdent(call.Fun, "runInteractiveSuite") && len(call.Args) == 3 &&
		isIdent(call.Args[0], "cfg") && isIdent(call.Args[1], "senderClient") && isIdent(call.Args[2], "logger"))
	ret, ok := is.Body.List[1].(*ast.ReturnStmt)
	require.True(t, ok && len(ret.Results) == 0)

	refs := 0
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name == "runInteractiveSuite" {
				refs++
			}
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				assert.False(t, isIdent(l, "suite"), "suite written or declared at %v", x.Pos())
			}
		case *ast.IncDecStmt:
			assert.False(t, isIdent(x.X, "suite"))
		case *ast.UnaryExpr:
			assert.False(t, x.Op == token.AND && isIdent(x.X, "suite"), "&suite")
		case *ast.ValueSpec:
			for _, n := range x.Names {
				assert.NotEqual(t, "suite", n.Name, "suite declared again")
			}
		case *ast.FuncLit:
			for _, fl := range x.Type.Params.List {
				for _, n := range fl.Names {
					assert.NotEqual(t, "suite", n.Name, "suite shadowed by a parameter")
				}
			}
		case *ast.RangeStmt:
			assert.False(t, isIdent(x.Key, "suite") || isIdent(x.Value, "suite"), "suite shadowed by range")
		}
		return true
	})
	assert.Equal(t, 1, refs, "runInteractiveSuite named outside the branch call")
}
