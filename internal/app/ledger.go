package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

const ledgerSchemaVersion = "obi.v2"

var (
	errLedgerNotFound = errors.New("results log not found")
	ledgerUpgradeOnce sync.Map
)

type ledgerEntry struct {
	SchemaVersion  string    `json:"schema_version"`
	RunID          string    `json:"run_id"`
	SessionID      string    `json:"session_id"`
	RepoRoot       string    `json:"repo_root"`
	EpicID         string    `json:"epic_id"`
	EpicKey        string    `json:"epic_key"`
	EpicName       string    `json:"epic_name"`
	Alias          string    `json:"alias"`
	BeadID         string    `json:"bead_id,omitempty"`
	Status         string    `json:"status"`
	CommitSummary  string    `json:"commit_summary"`
	CommitDetails  string    `json:"commit_details"`
	Escalation     string    `json:"escalation,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	CompletedAt    time.Time `json:"completed_at"`
	DurationMs     int64     `json:"duration_ms"`
	ExitCode       int       `json:"exit_code"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	CodexBinary    string    `json:"codex_binary,omitempty"`
	CodexModel     string    `json:"codex_model,omitempty"`
	CodexSandbox   string    `json:"codex_sandbox,omitempty"`
	CodexApproval  string    `json:"codex_approval,omitempty"`
	CodexExtraArgs []string  `json:"codex_extra_args,omitempty"`
	ConfigDigest   string    `json:"config_digest,omitempty"`
	PromptHash     string    `json:"prompt_hash,omitempty"`
	Redacted       bool      `json:"redacted,omitempty"`
	OperatorEvents []operatorLedgerEvent `json:"operator_events,omitempty"`
}

const ledgerScanMaxBytes = 8 * 1024 * 1024

func appendLedgerEntry(path string, entry ledgerEntry) error {
	if path == "" {
		return fmt.Errorf("empty results log path")
	}
	if err := ensureLedgerSchema(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("ensure log dir: %w", err)
	}

	entry.SchemaVersion = ledgerSchemaVersion
	entry.CommitSummary = strings.TrimSpace(entry.CommitSummary)
	entry.CommitDetails = strings.TrimSpace(entry.CommitDetails)
	entry.Escalation = strings.TrimSpace(entry.Escalation)
	entry.DurationMs = durationMillis(entry.StartedAt, entry.CompletedAt)

	record, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal ledger entry: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(record, '\n')); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	return nil
}

func durationMillis(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func detectBeadID(plan sessionPlan, texts ...string) string {
	root := strings.ToLower(strings.TrimSpace(plan.EpicID))
	if root == "" {
		return ""
	}
	if idx := strings.LastIndex(root, "-"); idx > 0 {
		root = root[:idx]
	}
	if root == "" {
		root = strings.ToLower(plan.EpicID)
	}
	pattern := regexp.MustCompile(regexp.QuoteMeta(root) + `-[a-z0-9][a-z0-9\.-]*`)
	for _, text := range texts {
		if text == "" {
			continue
		}
		if match := pattern.FindString(strings.ToLower(text)); match != "" {
			return match
		}
	}
	return ""
}

type operatorLedgerEvent struct {
	Kind    string    `json:"kind"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

func completedBeadsFromLedger(path, epicID string) ([]string, error) {
	entries, err := ledgerEntriesForEpic(path, epicID)
	if err != nil {
		if errors.Is(err, errLedgerNotFound) {
			return nil, fmt.Errorf("results log %s not found; run at least once before using --resume", path)
		}
		return nil, err
	}

	var completed []string
	seen := map[string]struct{}{}
	for _, entry := range entries {
		status := strings.ToLower(strings.TrimSpace(entry.Status))
		switch status {
		case "":
			return nil, fmt.Errorf("ledger entry for session %s is missing a status; cannot resume safely", entry.SessionID)
		case footer.StatusSuccess:
			bead := strings.TrimSpace(entry.BeadID)
			if bead == "" {
				return nil, fmt.Errorf("ledger entry for session %s is missing bead_id; rerun without --resume or repair the ledger", entry.SessionID)
			}
			key := strings.ToLower(bead)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			completed = append(completed, bead)
		case footer.StatusFailure:
			bead := strings.TrimSpace(entry.BeadID)
			if bead == "" {
				bead = "unknown bead"
			}
			return nil, fmt.Errorf("session %s ended with status=%s for %s; resolve it before resuming", entry.SessionID, status, bead)
		default:
			return nil, fmt.Errorf("ledger entry for session %s has unknown status %q", entry.SessionID, entry.Status)
		}
	}
	return completed, nil
}

func ledgerEntriesForEpic(path, epicID string) ([]ledgerEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errLedgerNotFound, path)
		}
		return nil, fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), ledgerScanMaxBytes)

	var entries []ledgerEntry
	lowerEpic := strings.ToLower(strings.TrimSpace(epicID))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry ledgerEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("parse ledger entry: %w", err)
		}
		if lowerEpic == "" || strings.EqualFold(strings.TrimSpace(entry.EpicID), lowerEpic) {
			entries = append(entries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan ledger: %w", err)
	}
	return entries, nil
}

func ensureLedgerSchema(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, ok := ledgerUpgradeOnce.Load(path); ok {
		return nil
	}
	needsUpgrade, err := ledgerNeedsUpgrade(path)
	if err != nil {
		return err
	}
	if needsUpgrade {
		if err := upgradeLedgerFile(path); err != nil {
			return err
		}
	}
	ledgerUpgradeOnce.Store(path, struct{}{})
	return nil
}

func ledgerNeedsUpgrade(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ledgerUpgradeOnce.Store(path, struct{}{})
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("results log path %s is a directory", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(line, `"schema_version":"`+ledgerSchemaVersion+`"`) {
			return false, nil
		}
		return true, nil
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func upgradeLedgerFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	var upgraded []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return fmt.Errorf("parse ledger entry during upgrade: %w", err)
		}
		payload["schema_version"] = ledgerSchemaVersion
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode upgraded entry: %w", err)
		}
		upgraded = append(upgraded, string(encoded))
	}

	temp := path + ".upgrade"
	content := strings.Join(upgraded, "\n")
	if len(upgraded) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(temp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write upgraded ledger: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("replace upgraded ledger: %w", err)
	}
	return nil
}
