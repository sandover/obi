package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/codexexec"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/fenced"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/interactive"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/tui"
)

const usage = `obi – automate Codex bead execution

Usage:
  obi init                      Scaffold obi.toml (or refresh if it already exists)
  obi refresh [--config path]   Sync obi.toml with open epics
  obi list [--config path]      Show available epics and aliases
  obi go <alias> [options]      Preview and run a Codex session`

// Run is the top-level entrypoint for the obi CLI.
func Run(args []string) error {
	if len(args) == 0 {
		fmt.Println(usage)
		return nil
	}

	switch args[0] {
	case "go":
		return runGo(args[1:])
	case "refresh":
		return runRefresh(args[1:])
	case "list":
		return runList(args[1:])
	case "init":
		return runInit(args[1:])
	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

type goOptions struct {
	configPath string
	aliasInput string
	outPath    string
	resume     bool
	noTUI      bool
}

type sessionOutcome struct {
	Status string
	BeadID string
}


func runGo(args []string) error {
	opts, err := parseGoOptions(args)
	if err != nil {
		return err
	}

	resolvedPath, err := config.ResolvePath(opts.configPath)
	if err != nil {
		return err
	}

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		return err
	}

	repoRoot := repoRootForConfig(resolvedPath)
	cfgDigest := configDigest(resolvedPath)

	logPath, err := cfg.ResultsLogPath()
	if err != nil {
		return err
	}

	var plan sessionPlan

	if strings.TrimSpace(opts.aliasInput) == "" {
		if cfg.Issues == nil {
			printMissingIssuesMessage(cfg)
			return nil
		}
		plan = planFromIssues(cfg)
	} else {
		plan, err = prepareSession(cfg, opts.aliasInput)
		if err != nil {
			return err
		}
	}

	plan.RepoRoot = repoRoot
	plan.ConfigDigest = cfgDigest

	if opts.resume {
		if err := enableResume(&plan, logPath); err != nil {
			return err
		}
	}

	if plan.EpicID == "" || plan.EpicID == "issues" {
		if err := ensureReadyWork(plan); err != nil {
			return err
		}
		outcome, err := executeSession(plan, opts, cfg, logPath, cfg.ConfirmBeforeRunValue(), !cfg.ConfirmBeforeRunValue())
		if err != nil {
			return err
		}
		if outcome.Status == "" {
			return nil
		}
		return nil
	}

	return runEpicLoop(plan, opts, cfg, logPath)
}

func runEpicLoop(plan sessionPlan, opts goOptions, cfg *config.Config, logPath string) error {
	confirmFirst := cfg.ConfirmBeforeRunValue()
	autoConfirmNotice := !confirmFirst
	sessionCount := 0

	for {
		if sessionCount == 0 {
			if err := ensureReadyWork(plan); err != nil {
				return err
			}
		} else {
			hasWork, err := readyWorkAvailable(plan)
			if err != nil {
				return err
			}
			if !hasWork {
				fmt.Printf("No ready beads remain for %s (%s). All done.\n", plan.EpicName, plan.EpicID)
				if err := maybeRunSummarizer(plan, opts, cfg, logPath); err != nil {
					return err
				}
				return nil
			}
			fmt.Printf("\nReady beads remain for %s (%s); launching next session.\n\n", plan.EpicName, plan.EpicID)
		}

		fmt.Printf("=== Codex session #%d ===\n\n", sessionCount+1)

		outcome, err := executeSession(plan, opts, cfg, logPath, confirmFirst && sessionCount == 0, autoConfirmNotice && sessionCount == 0)
		if err != nil {
			return err
		}
		if outcome.Status == "" {
			return nil
		}
		if bead := strings.TrimSpace(outcome.BeadID); bead != "" {
			plan.ResumeCompletedBeads = append(plan.ResumeCompletedBeads, bead)
		}
		sessionCount++
	}
}

func executeSession(plan sessionPlan, opts goOptions, cfg *config.Config, logPath string, requireConfirmation bool, autoConfirmNotice bool) (sessionOutcome, error) {
	promptBody := buildPrompt(plan)
	sessionRunner := interactive.NewSessionRunner()
	preparedPrompt, err := sessionRunner.PreparePrompt(promptBody)
	if err != nil {
		return sessionOutcome{}, err
	}
	prompt := preparedPrompt.Text

	printPreview(plan, prompt)

	if plan.ResumeEnabled {
		printResumeSummary(plan)
		fmt.Println()
	}

	if requireConfirmation {
		ok, err := promptForConfirmation()
		if err != nil {
			return sessionOutcome{}, err
		}
		if !ok {
			fmt.Println("Run cancelled.")
			return sessionOutcome{}, nil
		}
	} else if autoConfirmNotice {
		fmt.Println("confirm_before_run=false; continuing without prompt.")
	}

	inv, err := codexexec.Build(plan.Codex, prompt)
	if err != nil {
		return sessionOutcome{}, err
	}
	fmt.Printf("\nLaunching Codex: %s %v\n", inv.Binary, inv.Args)

	transcript, transcriptPath, err := openTranscriptWriter(logPath, opts.outPath, preparedPrompt.SessionID)
	if err != nil {
		return sessionOutcome{}, err
	}
	if transcript != nil {
		defer transcript.Close()
	}

	var teeWriter io.Writer
	if transcript != nil {
		if locked := newLockedWriter(transcript); locked != nil {
			teeWriter = locked
		}
	}

	secrets := redactionSecrets()
	opLog := newOperatorLog(teeWriter)
	useTUI := !opts.noTUI
	var sessionStdout io.Writer
	if useTUI {
		sessionStdout = io.Discard
	} else {
		sessionStdout = os.Stdout
	}

	handle, err := sessionRunner.Start(context.Background(), interactive.StartOptions{
		SessionID:  preparedPrompt.SessionID,
		Prompt:     prompt,
		Invocation: inv,
		Stdout:     sessionStdout,
		Tee:        teeWriter,
		Secrets:    secrets,
	})
	if err != nil {
		return sessionOutcome{}, newExitError(err.Error())
	}

	var sessionView *sessionDisplay
	if useTUI {
		sessionView, err = startSessionTUI(handle, plan, opLog)
		if err != nil {
			return sessionOutcome{}, err
		}
	}
	defer func() {
		if sessionView != nil {
			sessionView.Stop()
		}
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	var signalWriter io.Writer = os.Stdout
	if useTUI {
		signalWriter = io.Discard
	}
	stopRelay := startSignalRelay(handle, sigCh, signalWriter)
	defer func() {
		stopRelay()
		signal.Stop(sigCh)
		close(sigCh)
	}()

	runRes, err := handle.Wait()
	if err != nil {
		return sessionOutcome{}, newExitError(err.Error())
	}

	fencedRes, err := parseFencedReport(preparedPrompt.SessionID, runRes.Output)
	if err != nil {
		return sessionOutcome{}, newExitError(fmt.Sprintf("parse fenced report: %v", err))
	}

	footerRes, err := footer.Parse(runRes.Output)
	if err != nil {
		return sessionOutcome{}, newExitError(fmt.Sprintf("parse footer: %v", err))
	}

	if !strings.EqualFold(fencedRes.Status, footerRes.Status) {
		return sessionOutcome{}, newExitError("fenced report status does not match legacy footer")
	}
	if normalizeMultiline(fencedRes.Details) != normalizeMultiline(footerRes.CommitMsg) {
		return sessionOutcome{}, newExitError("fenced report details do not match legacy footer commit body")
	}
	if normalizeWhitespace(fencedRes.Escalation) != normalizeWhitespace(footerRes.Escalation) {
		return sessionOutcome{}, newExitError("fenced report escalation does not match legacy footer")
	}

	fmt.Printf("\nCodex status: %s\n", fencedRes.Status)
	fmt.Printf("Commit summary: %s\n", fencedRes.CommitMsg)
	fmt.Printf("Details:\n%s\n", fencedRes.Details)
	if fencedRes.Escalation != "" {
		fmt.Printf("Escalation: %s\n", fencedRes.Escalation)
	}

	beadID := detectBeadID(plan, runRes.Output, fencedRes.Details, fencedRes.CommitMsg, footerRes.CommitMsg)
	if plan.BeadIDOverride != "" {
		beadID = plan.BeadIDOverride
	}

	if sessionView != nil {
		statusText := strings.TrimSpace(fencedRes.Status)
		sessionView.UpdateStatus(func(line *tui.StatusLine) {
			line.RunStatus = statusText
			line.BeadID = beadID
		})
		sessionView.Stop()
		sessionView = nil
	}

	redactedSummary, summaryRedacted := redactText(fencedRes.CommitMsg, secrets)
	redactedDetails, detailsRedacted := redactText(fencedRes.Details, secrets)
	redactedEscalation, escalationRedacted := redactText(strings.TrimSpace(fencedRes.Escalation), secrets)
	redactionsApplied := summaryRedacted || detailsRedacted || escalationRedacted

	entryPromptHash := promptHash(prompt)

	entry := ledgerEntry{
		RunID:          preparedPrompt.SessionID,
		SessionID:      preparedPrompt.SessionID,
		RepoRoot:       plan.RepoRoot,
		EpicID:         plan.EpicID,
		EpicKey:        plan.EpicKey,
		EpicName:       plan.EpicName,
		Alias:          plan.Alias,
		Status:         fencedRes.Status,
		CommitSummary:  redactedSummary,
		CommitDetails:  redactedDetails,
		Escalation:     redactedEscalation,
		StartedAt:      runRes.StartedAt,
		CompletedAt:    runRes.CompletedAt,
		ExitCode:       runRes.ExitCode,
		TranscriptPath: transcriptPath,
		BeadID:         beadID,
		CodexBinary:    inv.Binary,
		CodexModel:     plan.Codex.Model,
		CodexSandbox:   plan.Codex.Sandbox,
		CodexApproval:  plan.Codex.Approval,
		CodexExtraArgs: append([]string(nil), plan.Codex.ExtraArgs...),
		ConfigDigest:   plan.ConfigDigest,
		PromptHash:     entryPromptHash,
		Redacted:       redactionsApplied,
		OperatorEvents: opLog.ledgerEvents(secrets),
	}
	if err := appendLedgerEntry(logPath, entry); err != nil {
		return sessionOutcome{}, err
	}

	if footerRes.Status == footer.StatusFailure {
		return sessionOutcome{}, newExitError("Codex requested escalation; stopping.")
	}

	if runRes.ExitCode != 0 {
		return sessionOutcome{}, newExitError(fmt.Sprintf("codex exited with status %d", runRes.ExitCode))
	}

	return sessionOutcome{Status: fencedRes.Status, BeadID: beadID}, nil
}

func parseGoOptions(args []string) (goOptions, error) {
	fs := flag.NewFlagSet("go", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts goOptions
	fs.StringVar(&opts.configPath, "config", "", "path to obi config")
	fs.StringVar(&opts.outPath, "out", "", "tee codex stdout/stderr to this file")
	fs.BoolVar(&opts.resume, "resume", false, "skip beads already logged as success for this epic")
	fs.BoolVar(&opts.noTUI, "no-tui", false, "disable the interactive TUI (stream raw Codex output)")

	normalized, alias, err := splitAliasAndArgs(args)
	if err != nil {
		return goOptions{}, err
	}

	if err := fs.Parse(normalized); err != nil {
		return goOptions{}, fmt.Errorf("parse flags: %w", err)
	}

	opts.aliasInput = alias

	return opts, nil
}

func splitAliasAndArgs(args []string) ([]string, string, error) {
	var alias string
	var normalized []string
	iter := newArgIterator(args)
	for iter.Next() {
		arg := iter.Value()
		if arg == "--" {
			if !iter.Next() {
				break
			}
			if alias != "" {
				return nil, "", fmt.Errorf("unexpected extra arguments: %s", iter.Value())
			}
			alias = iter.Value()
			if iter.Next() {
				return nil, "", fmt.Errorf("unexpected extra arguments: %s", iter.Value())
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			normalized = append(normalized, arg)
			if consumesValue(arg) && !strings.Contains(arg, "=") {
				if !iter.Next() {
					return nil, "", fmt.Errorf("flag %s requires a value", flagName(arg))
				}
				normalized = append(normalized, iter.Value())
			}
			continue
		}
		if alias == "" {
			alias = arg
			continue
		}
		return nil, "", fmt.Errorf("unexpected extra arguments: %s", arg)
	}
	return normalized, alias, nil
}

func consumesValue(flag string) bool {
	flag = flagName(flag)
	switch flag {
	case "-o", "--out", "--config":
		return true
	default:
		return false
	}
}

func flagName(flag string) string {
	if idx := strings.Index(flag, "="); idx != -1 {
		return flag[:idx]
	}
	return flag
}

type argIterator struct {
	args []string
	i    int
}

func newArgIterator(args []string) *argIterator {
	return &argIterator{args: args, i: -1}
}

func (it *argIterator) Next() bool {
	it.i++
	return it.i < len(it.args)
}

func (it *argIterator) Value() string {
	return it.args[it.i]
}

func planFromIssues(cfg *config.Config) sessionPlan {
	return sessionPlan{
		EpicKey:    "issues-outside-epics",
		EpicName:   "Issues Outside Epics",
		Alias:      "issues",
		EpicID:     "issues",
		Tool:       "",
		EpicPrompt: cfg.Issues.Prompt,
		BasePrompt: cfg.BasePrompt,
		Codex:      cfg.Codex,
	}
}

func printMissingIssuesMessage(cfg *config.Config) {
	fmt.Println("No \"issues outside epics\" section found in obi.toml, so `obi go` needs an explicit epic alias or ID.")
	if len(cfg.Epics) == 0 {
		fmt.Println("Tip: add the section to obi.toml or run `obi refresh` after creating your first epic.")
		return
	}
	fmt.Println("Available epics:")
	for _, key := range sortedEpicKeys(cfg.Epics) {
		alias := epicAliasHandle(key, cfg.Epics[key])
		fmt.Printf("  - %s (alias: %s, id: %s)\n", cfg.Epics[key].Name, alias, cfg.Epics[key].ID)
	}
	fmt.Println("Run `obi go <alias-or-epic-id>` to work on one of these epics.")
}

func printPreview(plan sessionPlan, prompt string) {
	fmt.Println("Preparing to have Codex work on this:")
	fmt.Print(formatPreviewTable(plan))
	fmt.Println()
	fmt.Println("Prompt for Codex:")
	fmt.Println(indentPrompt(prompt))
	fmt.Println()
}

func formatPreviewTable(plan sessionPlan) string {
	const (
		aliasWidth = 18
		nameWidth  = 30
		idWidth    = 27
	)
	var b strings.Builder
	fmt.Fprintf(&b, "  %-*s  %-*s  %-*s\n",
		aliasWidth, "Alias",
		nameWidth, "Name",
		idWidth, "Epic ID",
	)
	fmt.Fprintf(&b, "  %-*s  %-*s  %-*s\n",
		aliasWidth, strings.Repeat("-", aliasWidth),
		nameWidth, strings.Repeat("-", nameWidth),
		idWidth, strings.Repeat("-", idWidth),
	)
	fmt.Fprintf(&b, "  %-*s  %-*s  %-*s\n",
		aliasWidth, plan.Alias,
		nameWidth, plan.EpicName,
		idWidth, plan.EpicID,
	)
	return b.String()
}

func indentPrompt(prompt string) string {
	lines := strings.Split(strings.TrimSpace(prompt), "\n")
	for i, line := range lines {
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n")
}

func promptForConfirmation() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Proceed? [Y/n]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		choice := strings.TrimSpace(strings.ToLower(input))
		if choice == "" || choice == "y" {
			return true, nil
		}
		if choice == "n" {
			return false, nil
		}
		fmt.Println("Please respond with Y or n.")
	}
}

func printResumeSummary(plan sessionPlan) {
	fmt.Println("Resume mode enabled.")
	if len(plan.ResumeCompletedBeads) == 0 {
		fmt.Println("  No completed beads recorded; starting fresh.")
		return
	}
	fmt.Println("Completed beads already logged for this epic (will be skipped):")
	for _, id := range plan.ResumeCompletedBeads {
		fmt.Printf("  - %s\n", id)
	}
}

func parseFencedReport(sessionID string, output string) (fenced.Result, error) {
	parser := fenced.NewParser(sessionID)
	res, done, err := parser.Feed(output)
	if err != nil {
		return fenced.Result{}, err
	}
	if done {
		return res, nil
	}
	res, done, err = parser.Finalize()
	if err != nil {
		return fenced.Result{}, err
	}
	if !done {
		return fenced.Result{}, fmt.Errorf("fenced report incomplete")
	}
	return res, nil
}

func normalizeMultiline(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(s)
}
