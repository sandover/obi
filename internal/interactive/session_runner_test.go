package interactive

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/codexexec"
)

func TestPreparePromptAddsFenceAndLegacyFooter(t *testing.T) {
	runner := NewSessionRunner(
		WithUUIDGenerator(func() (string, error) { return "session-123", nil }),
	)
	prep, err := runner.PreparePrompt("Base body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}
	if prep.SessionID != "session-123" {
		t.Fatalf("unexpected session id %q", prep.SessionID)
	}
	if !strings.Contains(prep.Text, "```obi:session-123") {
		t.Fatalf("prompt missing fenced instructions: %s", prep.Text)
	}
	if !strings.Contains(prep.Text, "STATUS: success") {
		t.Fatalf("prompt missing legacy footer: %s", prep.Text)
	}
	if !strings.Contains(prep.Text, "Base body") {
		t.Fatalf("prompt missing body: %s", prep.Text)
	}
}

func TestSessionRunnerStreamsOutputAndRedactsSecrets(t *testing.T) {
	fake := &fakeLauncher{
		script: "booting\nsuper-secret token\nSTATUS: success\nCOMMIT_MSG:\ndone\n",
	}
	runner := NewSessionRunner(
		WithLauncher(fake),
		WithPreflight(func() error { return nil }),
		WithUUIDGenerator(func() (string, error) { return "session-xyz", nil }),
	)

	prep, err := runner.PreparePrompt("body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}

	var live bytes.Buffer
	var tee bytes.Buffer

	handle, err := runner.Start(context.Background(), StartOptions{
		SessionID:  prep.SessionID,
		Prompt:     prep.Text,
		Invocation: codexexec.Invocation{Binary: "codex"},
		Stdout:     &live,
		Tee:        &tee,
		Secrets:    []string{"super-secret"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	result, err := handle.Wait()
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if strings.Contains(result.Output, "super-secret") {
		t.Fatalf("result output should be redacted, got %q", result.Output)
	}
	if strings.Contains(tee.String(), "super-secret") {
		t.Fatalf("tee output should be redacted, got %q", tee.String())
	}
	if !strings.Contains(live.String(), "super-secret") {
		t.Fatalf("live stream should include secret for operator visibility")
	}

	var sawLog bool
	for evt := range handle.Events() {
		if evt.Type == EventLogChunk {
			sawLog = true
		}
	}
	if !sawLog {
		t.Fatalf("expected at least one log chunk event")
	}
}

func TestSessionRunnerSoftStopWritesMarker(t *testing.T) {
	fake := &fakeLauncher{
		script: "STATUS: success\nCOMMIT_MSG:\nok\n",
	}
	runner := NewSessionRunner(
		WithLauncher(fake),
		WithPreflight(func() error { return nil }),
		WithUUIDGenerator(func() (string, error) { return "session-soft", nil }),
	)
	prep, err := runner.PreparePrompt("body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}
	handle, err := runner.Start(context.Background(), StartOptions{
		SessionID:  prep.SessionID,
		Prompt:     prep.Text,
		Invocation: codexexec.Invocation{Binary: "codex"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := handle.SoftStop("wrap up"); err != nil {
		t.Fatalf("soft stop: %v", err)
	}
	input := fake.lastTTY.ReceivedInput()
	if !strings.Contains(input, SoftStopMarker) {
		t.Fatalf("expected soft stop marker in tty input, got %q", input)
	}
	if !strings.Contains(input, "wrap up") {
		t.Fatalf("expected reason in tty input, got %q", input)
	}
	_, _ = handle.Wait()
}

func TestSessionRunnerSubmitHintWritesMarker(t *testing.T) {
	fake := &fakeLauncher{
		script: "STATUS: success\nCOMMIT_MSG:\nok\n",
	}
	runner := NewSessionRunner(
		WithLauncher(fake),
		WithPreflight(func() error { return nil }),
		WithUUIDGenerator(func() (string, error) { return "session-hint", nil }),
	)
	prep, err := runner.PreparePrompt("body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}
	handle, err := runner.Start(context.Background(), StartOptions{
		SessionID:  prep.SessionID,
		Prompt:     prep.Text,
		Invocation: codexexec.Invocation{Binary: "codex"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := handle.SubmitHint("Need to cover tests"); err != nil {
		t.Fatalf("submit hint: %v", err)
	}
	input := fake.lastTTY.ReceivedInput()
	if !strings.Contains(input, HumanHintMarker) {
		t.Fatalf("expected hint marker in tty input, got %q", input)
	}
	if !strings.Contains(input, "Need to cover tests") {
		t.Fatalf("expected hint text in tty input, got %q", input)
	}
	_, _ = handle.Wait()
}

func TestSessionRunnerAbortUsesSignal(t *testing.T) {
	fake := &fakeLauncher{
		script: "STATUS: success\nCOMMIT_MSG:\nok\n",
	}
	runner := NewSessionRunner(
		WithLauncher(fake),
		WithPreflight(func() error { return nil }),
		WithUUIDGenerator(func() (string, error) { return "session-abort", nil }),
	)
	prep, err := runner.PreparePrompt("body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}
	handle, err := runner.Start(context.Background(), StartOptions{
		SessionID:  prep.SessionID,
		Prompt:     prep.Text,
		Invocation: codexexec.Invocation{Binary: "codex"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := handle.Abort(); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if len(fake.lastSignals) == 0 || fake.lastSignals[0] != os.Interrupt {
		t.Fatalf("expected SIGINT, got %v", fake.lastSignals)
	}
	_, _ = handle.Wait()
}

func TestSessionHandleWriteInputForwardsToTTY(t *testing.T) {
	fake := &fakeLauncher{
		script: "STATUS: success\nCOMMIT_MSG:\nok\n",
	}
	runner := NewSessionRunner(
		WithLauncher(fake),
		WithPreflight(func() error { return nil }),
		WithUUIDGenerator(func() (string, error) { return "session-write", nil }),
	)
	prep, err := runner.PreparePrompt("body")
	if err != nil {
		t.Fatalf("prepare prompt: %v", err)
	}
	handle, err := runner.Start(context.Background(), StartOptions{
		SessionID:  prep.SessionID,
		Prompt:     prep.Text,
		Invocation: codexexec.Invocation{Binary: "codex"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := handle.WriteInput([]byte("hello")); err != nil {
		t.Fatalf("write input: %v", err)
	}
	if got := fake.lastTTY.ReceivedInput(); got != "hello" {
		t.Fatalf("expected tty to receive input, got %q", got)
	}
	_, _ = handle.Wait()
}

type fakeLauncher struct {
	script      string
	waitErr     error
	lastTTY     *fakePTY
	lastSignals []os.Signal
}

func (f *fakeLauncher) Launch(_ context.Context, inv codexexec.Invocation, _ string, _ []string) (*processHandle, error) {
	if inv.Binary == "" {
		return nil, io.EOF
	}
	tty := newFakePTY(f.script)
	f.lastTTY = tty
	waitErr := f.waitErr
	if waitErr == nil {
		waitErr = exitError{code: 0}
	}
	return &processHandle{
		tty: tty,
		wait: func() error {
			return waitErr
		},
		kill: func() error {
			return tty.Close()
		},
		signal: func(sig os.Signal) error {
			f.lastSignals = append(f.lastSignals, sig)
			return nil
		},
	}, nil
}

type fakePTY struct {
	output []byte
	offset int
	input  strings.Builder
	closed bool
}

func newFakePTY(script string) *fakePTY {
	return &fakePTY{output: []byte(script)}
}

func (f *fakePTY) Read(p []byte) (int, error) {
	if f.closed {
		return 0, io.EOF
	}
	if f.offset >= len(f.output) {
		return 0, io.EOF
	}
	n := copy(p, f.output[f.offset:])
	f.offset += n
	if f.offset >= len(f.output) {
		return n, io.EOF
	}
	return n, nil
}

func (f *fakePTY) Write(p []byte) (int, error) {
	if f.closed {
		return 0, io.EOF
	}
	return f.input.WriteString(string(p))
}

func (f *fakePTY) Close() error {
	f.closed = true
	return nil
}

func (f *fakePTY) ReceivedInput() string {
	return f.input.String()
}

type exitError struct {
	code int
}

func (e exitError) Error() string {
	return "exit"
}

func (e exitError) ExitCode() int {
	return e.code
}
