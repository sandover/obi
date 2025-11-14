package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

func TestAppendLedgerEntryWritesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	start := time.Unix(0, 0)
	end := start.Add(1500 * time.Millisecond)

	entry := ledgerEntry{
		RunID:          "run-42",
		SessionID:      "session-42",
		RepoRoot:       "/repo",
		EpicID:         "automatic-octo-barnacle-d4c",
		EpicKey:        "automatic_octo_barnacle_d4c",
		EpicName:       "Test Epic",
		Alias:          "alias",
		Status:         "success",
		CommitSummary:  "Do thing",
		CommitDetails:  "Did several things",
		StartedAt:      start,
		CompletedAt:    end,
		ExitCode:       0,
		TranscriptPath: "/tmp/transcript.log",
		CodexBinary:    "/usr/local/bin/codex",
		CodexModel:     "gpt-5",
		CodexSandbox:   "workspace-write",
		CodexApproval:  "on-request",
		CodexExtraArgs: []string{"--foo", "bar"},
		ConfigDigest:   "abc123",
		PromptHash:     "def456",
		Redacted:       true,
	}

	if err := appendLedgerEntry(path, entry); err != nil {
		t.Fatalf("append ledger: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	var stored ledgerEntry
	line := strings.TrimSpace(string(data))
	if err := json.Unmarshal([]byte(line), &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stored.SchemaVersion != ledgerSchemaVersion {
		t.Fatalf("expected schema %s, got %s", ledgerSchemaVersion, stored.SchemaVersion)
	}
	if stored.DurationMs != 1500 {
		t.Fatalf("expected duration 1500, got %d", stored.DurationMs)
	}
	if stored.TranscriptPath != "/tmp/transcript.log" {
		t.Fatalf("expected transcript path, got %q", stored.TranscriptPath)
	}
	if stored.RunID != "run-42" || stored.RepoRoot != "/repo" {
		t.Fatalf("expected run metadata, got %+v", stored)
	}
	if !stored.Redacted {
		t.Fatalf("expected redacted flag to be true")
	}
	if stored.CodexBinary != "/usr/local/bin/codex" {
		t.Fatalf("expected codex metadata")
	}
}

func TestAppendLedgerEntryLocksFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		RunID:         "run",
		SessionID:     "session",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		Alias:         "alias",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
		BeadID:        "automatic-octo-barnacle-d4c.1",
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- appendLedgerEntry(path, entry)
	}()
	go func() {
		errCh <- appendLedgerEntry(path, entry)
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("append ledger concurrent: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two entries, got %d", len(lines))
	}
}

func TestAppendLedgerEntryAppendsNDJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

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
		CompletedAt:   time.Unix(0, 0).Add(time.Second),
		BeadID:        "automatic-octo-barnacle-d4c.1",
	}

	for i := 0; i < 3; i++ {
		entry := base
		entry.RunID = fmt.Sprintf("run-%d", i)
		entry.BeadID = fmt.Sprintf("automatic-octo-barnacle-d4c.%d", i+1)
		if err := appendLedgerEntry(path, entry); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("line %d not valid JSON: %s", i, line)
		}
		var entry ledgerEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal line %d: %v", i, err)
		}
		if entry.RunID != fmt.Sprintf("run-%d", i) {
			t.Fatalf("expected append order preserved, got %+v", entry)
		}
	}
}

func TestDetectBeadID(t *testing.T) {
	plan := sessionPlan{EpicID: "automatic-octo-barnacle-d4c"}
	text := "Worked on bead automatic-octo-barnacle-kmn for logging."
	if got := detectBeadID(plan, text); got != "automatic-octo-barnacle-kmn" {
		t.Fatalf("expected bead id, got %q", got)
	}
	if got := detectBeadID(plan, "no match"); got != "" {
		t.Fatalf("expected empty bead id, got %q", got)
	}
}

func TestCompletedBeadsFromLedgerReturnsSuccesses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	base := ledgerEntry{
		RunID:         "run",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0).Add(time.Second),
	}

	entry1 := base
	entry1.BeadID = "automatic-octo-barnacle-d4c.1"
	if err := appendLedgerEntry(path, entry1); err != nil {
		t.Fatalf("append entry1: %v", err)
	}

	entry2 := base
	entry2.BeadID = "automatic-octo-barnacle-d4c.2"
	entry2.CompletedAt = entry2.CompletedAt.Add(time.Second)
	if err := appendLedgerEntry(path, entry2); err != nil {
		t.Fatalf("append entry2: %v", err)
	}

	// Different epic should be ignored.
	other := base
	other.EpicID = "automatic-octo-barnacle-zzz"
	other.BeadID = "automatic-octo-barnacle-zzz.1"
	if err := appendLedgerEntry(path, other); err != nil {
		t.Fatalf("append other: %v", err)
	}

	got, err := completedBeadsFromLedger(path, "automatic-octo-barnacle-d4c")
	if err != nil {
		t.Fatalf("completed beads: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %v", got)
	}
	if got[0] != "automatic-octo-barnacle-d4c.1" || got[1] != "automatic-octo-barnacle-d4c.2" {
		t.Fatalf("unexpected bead order: %v", got)
	}
}

func TestCompletedBeadsFromLedgerFailsOnNeedsHelp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		RunID:         "run",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusFailure,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0).Add(time.Second),
		BeadID:        "automatic-octo-barnacle-d4c.9",
	}
	if err := appendLedgerEntry(path, entry); err != nil {
		t.Fatalf("append entry: %v", err)
	}

	if _, err := completedBeadsFromLedger(path, "automatic-octo-barnacle-d4c"); err == nil {
		t.Fatalf("expected failure when ledger contains needs_help entry")
	}
}

func TestCompletedBeadsFromLedgerFailsWithoutBeadID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		RunID:         "run",
		RepoRoot:      "/repo",
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0).Add(time.Second),
	}
	if err := appendLedgerEntry(path, entry); err != nil {
		t.Fatalf("append entry: %v", err)
	}

	if _, err := completedBeadsFromLedger(path, "automatic-octo-barnacle-d4c"); err == nil {
		t.Fatalf("expected failure when bead id missing")
	}
}

func TestEnsureLedgerSchemaUpgradesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")
	if err := os.WriteFile(path, []byte(`{"session_id":"legacy"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := ensureLedgerSchema(path); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read upgraded: %v", err)
	}
	if !strings.Contains(string(data), `"schema_version":"`+ledgerSchemaVersion+`"`) {
		t.Fatalf("expected schema upgrade, got %s", data)
	}
}
