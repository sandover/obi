package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenTranscriptWriterCreatesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "obi-results.log")

	w, path, err := openTranscriptWriter(logPath, "", "session-ABC")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if w == nil {
		t.Fatalf("expected writer")
	}
	defer w.Close()

	if !strings.Contains(path, "transcripts") {
		t.Fatalf("expected transcript directory, got %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	}
}

func TestOpenTranscriptWriterHonorsOverride(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "logs", "session.txt")

	w, path, err := openTranscriptWriter("", target, "ignored")
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	if w == nil {
		t.Fatalf("expected writer")
	}
	defer w.Close()
	if path != target {
		t.Fatalf("expected override path, got %q", path)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 perms, got %v", info.Mode())
	}
}

func TestSplitSecretsParsesList(t *testing.T) {
	raw := "token1, token2;token3\n token4"
	secrets := splitSecrets(raw)
	if len(secrets) != 4 {
		t.Fatalf("expected 4 secrets, got %d", len(secrets))
	}
	if secrets[0] != "token1" || secrets[3] != "token4" {
		t.Fatalf("unexpected secrets: %v", secrets)
	}
}
