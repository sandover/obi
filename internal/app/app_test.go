package app

import (
	"strings"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func TestSplitAliasAndArgsHandlesFlagsWithValues(t *testing.T) {
	args := []string{"scope-engine", "--out", "log.txt", "--config", "obi.toml"}
	normalized, alias, err := splitAliasAndArgs(args)
	if err != nil {
		t.Fatalf("splitAliasAndArgs: %v", err)
	}
	if alias != "scope-engine" {
		t.Fatalf("expected alias scope-engine, got %q", alias)
	}
	want := []string{"--out", "log.txt", "--config", "obi.toml"}
	if len(normalized) != len(want) {
		t.Fatalf("unexpected normalized args: %v", normalized)
	}
	for i, val := range want {
		if normalized[i] != val {
			t.Fatalf("normalized[%d]=%q want %q", i, normalized[i], val)
		}
	}
}

func TestSplitAliasAndArgsSupportsFlagEqualsSyntax(t *testing.T) {
	args := []string{"--out=log.txt", "scope-engine"}
	normalized, alias, err := splitAliasAndArgs(args)
	if err != nil {
		t.Fatalf("splitAliasAndArgs: %v", err)
	}
	if alias != "scope-engine" {
		t.Fatalf("expected alias scope-engine, got %q", alias)
	}
	if len(normalized) != 1 || normalized[0] != "--out=log.txt" {
		t.Fatalf("unexpected normalized args: %v", normalized)
	}
}

func TestSplitAliasAndArgsRejectsMultipleTargets(t *testing.T) {
	if _, _, err := splitAliasAndArgs([]string{"one", "two"}); err == nil {
		t.Fatalf("expected error for extra positional arguments")
	}
}

func TestFormatPreviewTablePlacesEpicIDLast(t *testing.T) {
	plan := sessionPlan{
		Alias:    "obi-orchestrator",
		EpicName: "Obi v2 interactive orchestration",
		EpicID:   "automatic-octo-barnacle-d4c",
	}
	out := formatPreviewTable(plan)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("unexpected preview table:\n%s", out)
	}
	header := lines[0]
	row := lines[2]
	hAlias := strings.Index(header, "Alias")
	hName := strings.Index(header, "Name")
	hID := strings.LastIndex(header, "Epic ID")
	if hAlias == -1 || hName == -1 || hID == -1 || !(hAlias < hName && hName < hID) {
		t.Fatalf("header order incorrect: %s", header)
	}
	rAlias := strings.Index(row, plan.Alias)
	rName := strings.Index(row, plan.EpicName)
	rID := strings.LastIndex(row, plan.EpicID)
	if rAlias == -1 || rName == -1 || rID == -1 || !(rAlias < rName && rName < rID) {
		t.Fatalf("row order incorrect: %s", row)
	}
}

func TestPlanFromIssuesBuildsIssuesPlan(t *testing.T) {
	cfg := &config.Config{
		BasePrompt: "base",
		Codex:      config.CodexConfig{Binary: "codex"},
		Issues:     &config.IssuesConfig{Prompt: "issues prompt"},
	}
	plan := planFromIssues(cfg)
	if plan.EpicID != "issues" {
		t.Fatalf("expected issues plan id, got %s", plan.EpicID)
	}
	if plan.EpicName != "Issues Outside Epics" {
		t.Fatalf("unexpected plan name: %s", plan.EpicName)
	}
	if plan.EpicPrompt != "issues prompt" {
		t.Fatalf("expected issues prompt, got %s", plan.EpicPrompt)
	}
	if plan.BasePrompt != "base" {
		t.Fatalf("expected base prompt, got %s", plan.BasePrompt)
	}
	if plan.Codex.Binary != "codex" {
		t.Fatalf("expected codex config")
	}
}

func TestResumeSkipSetNormalizesEntries(t *testing.T) {
	plan := sessionPlan{
		ResumeCompletedBeads: []string{
			" automatic-octo-barnacle-d4c.1 ",
			"AUTOMATIC-OCTO-BARNACLE-D4C.2",
			"",
			"automatic-octo-barnacle-d4c.1",
		},
	}
	set := plan.resumeSkipSet()
	if len(set) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(set))
	}
	if _, ok := set["automatic-octo-barnacle-d4c.1"]; !ok {
		t.Fatalf("missing normalized bead id 1")
	}
	if _, ok := set["automatic-octo-barnacle-d4c.2"]; !ok {
		t.Fatalf("missing normalized bead id 2")
	}
}

func TestIndentPromptIndentsEachLine(t *testing.T) {
	input := "line1\nline2\nline3"
	got := indentPrompt(input)
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "    ") {
			t.Fatalf("line %d not indented: %q", i, line)
		}
	}
}

func TestParseGoOptionsRecognizesNoTUIFlag(t *testing.T) {
	opts, err := parseGoOptions([]string{"--no-tui", "obi-orchestrator"})
	if err != nil {
		t.Fatalf("parseGoOptions: %v", err)
	}
	if !opts.noTUI {
		t.Fatalf("expected no-tui flag to set option")
	}
	if opts.aliasInput != "obi-orchestrator" {
		t.Fatalf("expected alias passthrough, got %s", opts.aliasInput)
	}
}
