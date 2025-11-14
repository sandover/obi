package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const redactionEnv = "OBI_REDACT"

func openTranscriptWriter(logPath, overridePath, sessionID string) (io.WriteCloser, string, error) {
	target := strings.TrimSpace(overridePath)
	if target != "" {
		if err := ensureTranscriptDir(filepath.Dir(target)); err != nil {
			return nil, "", err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return nil, "", fmt.Errorf("open transcript: %w", err)
		}
		return f, target, nil
	}

	if strings.TrimSpace(logPath) == "" {
		return nil, "", fmt.Errorf("transcript storage requires results log path or explicit --out target")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, "", fmt.Errorf("session id required to name transcript")
	}

	baseDir := filepath.Dir(logPath)
	transcriptDir := filepath.Join(baseDir, "transcripts")
	if err := ensureTranscriptDir(transcriptDir); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("%s.log", sanitizeFilename(sessionID))
	target = filepath.Join(transcriptDir, filename)

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, "", fmt.Errorf("open transcript: %w", err)
	}
	return f, target, nil
}

func ensureTranscriptDir(path string) error {
	if path == "" {
		path = "."
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("ensure transcript dir: %w", err)
	}
	return nil
}

func sanitizeFilename(input string) string {
	if input == "" {
		return "transcript"
	}
	var b strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func redactionSecrets() []string {
	raw := os.Getenv(redactionEnv)
	return splitSecrets(raw)
}

func splitSecrets(raw string) []string {
	var secrets []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	}) {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			secrets = append(secrets, trimmed)
		}
	}
	return secrets
}

func configDigest(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func redactText(input string, secrets []string) (string, bool) {
	if len(secrets) == 0 {
		return input, false
	}
	out := input
	var redacted bool
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		if strings.Contains(out, secret) {
			out = strings.ReplaceAll(out, secret, "[REDACTED]")
			redacted = true
		}
	}
	return out, redacted
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newLockedWriter(w io.Writer) *lockedWriter {
	if w == nil {
		return nil
	}
	return &lockedWriter{w: w}
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	if w == nil || w.w == nil {
		return 0, fmt.Errorf("writer not initialized")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}
