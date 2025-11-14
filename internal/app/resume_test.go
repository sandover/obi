package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

func TestEnableResumeLoadsCompletedBeads(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "results.log")

	base := ledgerEntry{
		RunID:         "run",
		SessionID:     "session",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
	}

	entry1 := base
	entry1.BeadID = "automatic-octo-barnacle-d4c.1"
	if err := appendLedgerEntry(logPath, entry1); err != nil {
		t.Fatalf("append entry1: %v", err)
	}
	entry2 := base
	entry2.BeadID = "automatic-octo-barnacle-d4c.2"
	if err := appendLedgerEntry(logPath, entry2); err != nil {
		t.Fatalf("append entry2: %v", err)
	}

	plan := sessionPlan{EpicID: "automatic-octo-barnacle-d4c"}
	if err := enableResume(&plan, logPath); err != nil {
		t.Fatalf("enableResume: %v", err)
	}
	if !plan.ResumeEnabled {
		t.Fatalf("expected resume flag")
	}
	if len(plan.ResumeCompletedBeads) != 2 {
		t.Fatalf("expected 2 beads, got %v", plan.ResumeCompletedBeads)
	}
	if plan.ResumeCompletedBeads[0] != "automatic-octo-barnacle-d4c.1" {
		t.Fatalf("unexpected bead order %v", plan.ResumeCompletedBeads)
	}
}

func TestEnableResumeFailsWhenNeedsHelpInLedger(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		RunID:         "run",
		SessionID:     "session",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		Status:        footer.StatusFailure,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
		BeadID:        "automatic-octo-barnacle-d4c.9",
	}
	if err := appendLedgerEntry(logPath, entry); err != nil {
		t.Fatalf("append entry: %v", err)
	}

	plan := sessionPlan{EpicID: "automatic-octo-barnacle-d4c"}
	if err := enableResume(&plan, logPath); err == nil {
		t.Fatalf("expected error when ledger contains needs_help entry")
	}
}

func TestEnableResumeFailsWhenBeadMissing(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		RunID:         "run",
		SessionID:     "session",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
	}
	if err := appendLedgerEntry(logPath, entry); err != nil {
		t.Fatalf("append entry: %v", err)
	}

	plan := sessionPlan{EpicID: "automatic-octo-barnacle-d4c"}
	if err := enableResume(&plan, logPath); err == nil {
		t.Fatalf("expected error when bead id missing")
	}
}
