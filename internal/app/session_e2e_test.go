package app

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

func TestExecuteSessionWithFakeCodexSuccess(t *testing.T) {
	t.Setenv("OBI_PIPE_LAUNCHER", "1")
	fake := buildFakeCodexBinary(t)
	t.Setenv("FAKE_CODEX_SCENARIO", "success")

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "results.log")
	plan, cfg := newTestPlan(logPath, fake, tempDir)
	cfg.Codex.Model = "fake-model"
	plan.Codex.Model = "fake-model"

	opts := goOptions{noTUI: true}

	outcome, err := executeSession(plan, opts, cfg, logPath, false, false)
	if err != nil {
		t.Fatalf("executeSession (success): %v", err)
	}
	if outcome.Status != footer.StatusSuccess {
		t.Fatalf("expected success outcome, got %s", outcome.Status)
	}

	entries := readLedger(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(entries))
	}
	if entries[0].Status != footer.StatusSuccess {
		t.Fatalf("ledger status mismatch: %s", entries[0].Status)
	}
	if entries[0].TranscriptPath == "" {
		t.Fatalf("expected transcript path to be recorded")
	}
	if _, err := os.Stat(entries[0].TranscriptPath); err != nil {
		t.Fatalf("transcript path missing: %v", err)
	}
	if entries[0].CodexBinary != fake {
		t.Fatalf("expected codex_binary %q, got %q", fake, entries[0].CodexBinary)
	}
	if entries[0].CodexModel != "fake-model" {
		t.Fatalf("expected codex_model fake-model, got %q", entries[0].CodexModel)
	}
	if entries[0].RepoRoot != tempDir {
		t.Fatalf("expected repo root %q, got %q", tempDir, entries[0].RepoRoot)
	}
}

func TestExecuteSessionWithFakeCodexNeedsHelp(t *testing.T) {
	t.Setenv("OBI_PIPE_LAUNCHER", "1")
	fake := buildFakeCodexBinary(t)
	t.Setenv("FAKE_CODEX_SCENARIO", "needs_help")

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "results.log")
	plan, cfg := newTestPlan(logPath, fake, tempDir)
	opts := goOptions{noTUI: true}

	if _, err := executeSession(plan, opts, cfg, logPath, false, false); err == nil {
		t.Fatalf("expected executeSession to fail for needs_help scenario")
	}

	entries := readLedger(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(entries))
	}
	if entries[0].Status != footer.StatusFailure {
		t.Fatalf("expected ledger status needs_help, got %s", entries[0].Status)
	}
	if entries[0].Escalation != "sandbox approval required" {
		t.Fatalf("unexpected escalation reason: %q", entries[0].Escalation)
	}
}

func TestExecuteSessionWithFakeCodexMalformedReport(t *testing.T) {
	t.Setenv("OBI_PIPE_LAUNCHER", "1")
	fake := buildFakeCodexBinary(t)
	t.Setenv("FAKE_CODEX_SCENARIO", "malformed")

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "results.log")
	plan, cfg := newTestPlan(logPath, fake, tempDir)
	opts := goOptions{noTUI: true}

	if _, err := executeSession(plan, opts, cfg, logPath, false, false); err == nil {
		t.Fatalf("expected executeSession to fail for malformed scenario")
	}
	if entries := readLedgerAllowMissing(t, logPath); len(entries) != 0 {
		t.Fatalf("expected no ledger entries for malformed run, got %d", len(entries))
	}
}

func TestExecuteSessionWithFakeCodexRedactionApplied(t *testing.T) {
	t.Setenv("OBI_PIPE_LAUNCHER", "1")
	fake := buildFakeCodexBinary(t)
	t.Setenv("FAKE_CODEX_SCENARIO", "long_logs")
	t.Setenv("OBI_REDACT", "SECRET_TOKEN")

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "results.log")
	plan, cfg := newTestPlan(logPath, fake, tempDir)
	opts := goOptions{noTUI: true}

	outcome, err := executeSession(plan, opts, cfg, logPath, false, false)
	if err != nil {
		t.Fatalf("executeSession (redaction): %v", err)
	}
	if outcome.Status != footer.StatusSuccess {
		t.Fatalf("expected success outcome, got %s", outcome.Status)
	}

	entries := readLedger(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(entries))
	}
	if strings.Contains(entries[0].CommitDetails, "SECRET_TOKEN") {
		t.Fatalf("ledger details leaked secret: %s", entries[0].CommitDetails)
	}
	if !strings.Contains(entries[0].CommitDetails, "[REDACTED]") {
		t.Fatalf("expected redaction placeholder in ledger details, got %s", entries[0].CommitDetails)
	}
	data, err := os.ReadFile(entries[0].TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if strings.Contains(string(data), "SECRET_TOKEN") {
		t.Fatalf("transcript leaked secret: %s", string(data))
	}
	if !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("expected transcript to contain redaction marker")
	}
}

func buildFakeCodexBinary(t *testing.T) string {
	t.Helper()
	outDir := t.TempDir()
	binary := filepath.Join(outDir, "fakecodex")

	cmd := exec.Command("go", "build", "-o", binary, "./internal/fakecodex/cmd/fakecodex")
	cmd.Dir = projectRoot(t)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake codex: %v\n%s", err, output)
	}
	return binary
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func newTestPlan(logPath, binary, tempDir string) (sessionPlan, *config.Config) {
	cfg := &config.Config{
		BasePrompt: "Base instructions",
		ResultsLog: logPath,
		Codex: config.CodexConfig{
			Binary: binary,
		},
	}
	plan := sessionPlan{
		EpicKey:    "fake-epic",
		EpicName:   "Fake Epic",
		EpicID:     "automatic-octo-barnacle-d4c",
		Alias:      "fake-epic",
		EpicPrompt: "Complete fake task",
		BasePrompt: cfg.BasePrompt,
		Codex:      cfg.Codex,
		RepoRoot:   tempDir,
	}
	return plan, cfg
}

func readLedger(t *testing.T, path string) []ledgerEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer f.Close()
	var entries []ledgerEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry ledgerEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode ledger entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ledger: %v", err)
	}
	return entries
}

func readLedgerAllowMissing(t *testing.T, path string) []ledgerEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("open ledger: %v", err)
	}
	defer f.Close()
	var entries []ledgerEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry ledgerEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode ledger entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ledger: %v", err)
	}
	return entries
}
