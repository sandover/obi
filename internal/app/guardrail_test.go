package app

import (
	"strings"
	"testing"
)

func TestHasReadyIssueForPlanMatchesEpic(t *testing.T) {
	plan := sessionPlan{EpicID: "automatic-octo-barnacle-d4c"}
	ready := []readyIssue{
		{ID: "automatic-octo-barnacle-d4c.1", IssueType: "task"},
	}
	ok, err := hasReadyIssueForPlan(plan, ready)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ready work to be available")
	}
}

func TestHasReadyIssueForPlanBlocksResumeWhenAllSkipped(t *testing.T) {
	plan := sessionPlan{
		EpicID:               "automatic-octo-barnacle-d4c",
		ResumeEnabled:        true,
		ResumeCompletedBeads: []string{"automatic-octo-barnacle-d4c.1"},
	}
	ready := []readyIssue{
		{ID: "automatic-octo-barnacle-d4c.1", IssueType: "task"},
	}
	ok, err := hasReadyIssueForPlan(plan, ready)
	if err == nil {
		t.Fatalf("expected resume guardrail error")
	}
	if ok {
		t.Fatalf("expected no ready work when all beads skipped")
	}
}

func TestHasReadyIssueForIssuesPlanAlwaysTrue(t *testing.T) {
	plan := sessionPlan{EpicID: "issues"}
	ok, err := hasReadyIssueForPlan(plan, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected issues plan to always allow work")
	}
}

func TestMissingReadyBeadsWarningIncludesPrefixAndCommand(t *testing.T) {
	epicID := "automatic-octo-barnacle-d4c"
	msg := missingReadyBeadsWarning(epicID)
	if !strings.Contains(msg, epicID) {
		t.Fatalf("missing epic id in warning: %s", msg)
	}
	if !strings.Contains(msg, "bd ready --json -n "+readyFetchLimit) {
		t.Fatalf("warning missing bd ready reference: %s", msg)
	}
	if !strings.Contains(msg, epicID+".<suffix>") {
		t.Fatalf("warning missing rename guidance: %s", msg)
	}
}

func TestIssueBelongsToEpicDetectsParent(t *testing.T) {
	if !issueBelongsToEpic("automatic-octo-barnacle-d4c.1", "automatic-octo-barnacle-d4c") {
		t.Fatalf("expected sub-issue to match epic prefix")
	}
	if issueBelongsToEpic("automatic-octo-barnacle-j4s.1", "automatic-octo-barnacle-d4c") {
		t.Fatalf("did not expect unrelated epic match")
	}
}

func TestParentEpicIDHandlesMissingSuffix(t *testing.T) {
	if got := parentEpicID("automatic-octo-barnacle-d4c.5"); got != "automatic-octo-barnacle-d4c" {
		t.Fatalf("unexpected parent id: %s", got)
	}
	if got := parentEpicID("automatic-octo-barnacle-d4c"); got != "" {
		t.Fatalf("expected empty parent for single-level id, got %s", got)
	}
}
