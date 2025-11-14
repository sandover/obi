package tui

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/interactive"
)

func TestShellRunRendersLayout(t *testing.T) {
	buf := &bytes.Buffer{}
	term := &fakeTerminal{width: 80, height: 20}
	shell := NewShell(
		WithIO(os.Stdin, buf),
		WithHeader("Test Session"),
		WithFooterHints([]string{"Ctrl+C soft-stop"}),
		withTerminal(term),
	)

	ctx := context.Background()
	events := make(chan interactive.SessionEvent, 4)
	events <- interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: "hello world\n"}
	events <- interactive.SessionEvent{Type: interactive.EventStateChange, State: interactive.StateRunning}
	events <- makeExitEvent(0, nil)
	close(events)

	if err := shell.Run(ctx, events); err != nil {
		t.Fatalf("shell run: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Test Session") {
		t.Fatalf("expected header in output, got %q", output)
	}
	if !strings.Contains(output, "hello world") {
		t.Fatalf("expected log output, got %q", output)
	}
	if !strings.Contains(output, "\x1b[?25h") {
		t.Fatalf("expected cursor restore sequence, got %q", output)
	}
	if term.restoreCount != 1 {
		t.Fatalf("expected terminal restore to be called, got %d", term.restoreCount)
	}
}

func TestShellHandleEventUpdatesPane(t *testing.T) {
	shell := NewShell(WithIO(os.Stdin, io.Discard), withTerminal(&fakeTerminal{width: 80, height: 10}))
	shell.HandleEvent(interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: "line one\n"})
	shell.HandleEvent(interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: "line two\n"})
	if lines := shell.pane.visible(10); len(lines) < 2 {
		t.Fatalf("expected 2 log lines, got %v", lines)
	}
}

func TestShellTogglePauseFreezesView(t *testing.T) {
	buf := &bytes.Buffer{}
	term := &fakeTerminal{width: 60, height: 12}
	shell := NewShell(WithIO(os.Stdin, buf), withTerminal(term))
	shell.fd = 0

	shell.HandleEvent(interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: "first line\n"})
	if err := shell.render(); err != nil {
		t.Fatalf("render before pause: %v", err)
	}

	if !shell.TogglePause() {
		t.Fatalf("expected toggle to enable pause")
	}
	shell.HandleEvent(interactive.SessionEvent{Type: interactive.EventLogChunk, Chunk: "second line\n"})

	buf.Reset()
	if err := shell.render(); err != nil {
		t.Fatalf("render while paused: %v", err)
	}
	output := buf.String()
	if strings.Contains(output, "second line") {
		t.Fatalf("expected paused view to hide new lines, got %q", output)
	}
	if !strings.Contains(output, "PAUSED") {
		t.Fatalf("expected paused indicator in header, got %q", output)
	}

	if shell.TogglePause() {
		t.Fatalf("expected toggle to resume")
	}
	buf.Reset()
	if err := shell.render(); err != nil {
		t.Fatalf("render after resume: %v", err)
	}
	output = buf.String()
	if !strings.Contains(output, "second line") {
		t.Fatalf("expected resumed render to include buffered lines, got %q", output)
	}
}

func TestShellRenderShowsHintInput(t *testing.T) {
	buf := &bytes.Buffer{}
	term := &fakeTerminal{width: 60, height: 12}
	shell := NewShell(WithIO(os.Stdin, buf), withTerminal(term))
	shell.fd = 0

	shell.SetHintInput(true, "Need a plan")
	if err := shell.render(); err != nil {
		t.Fatalf("render with hint: %v", err)
	}
	if !strings.Contains(buf.String(), "Need a plan") {
		t.Fatalf("expected hint text to appear, got %q", buf.String())
	}
}

func TestShellToggleHelpOverlay(t *testing.T) {
	buf := &bytes.Buffer{}
	term := &fakeTerminal{width: 80, height: 20}
	shell := NewShell(WithIO(os.Stdin, buf), withTerminal(term))
	shell.fd = 0

	shell.ToggleHelp()
	if err := shell.render(); err != nil {
		t.Fatalf("render with help: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Help:") || !strings.Contains(output, "Toggle this overlay") {
		t.Fatalf("expected help overlay content, got %q", output)
	}
}

func TestShellRenderIncludesStatusMetadata(t *testing.T) {
	buf := &bytes.Buffer{}
	term := &fakeTerminal{width: 120, height: 20}
	shell := NewShell(WithIO(os.Stdin, buf), withTerminal(term))
	shell.fd = 0

	shell.UpdateStatus(func(line *StatusLine) {
		line.EpicAlias = "obi-epic"
		line.EpicID = "automatic-octo-barnacle-d4c"
		line.BeadID = "automatic-octo-barnacle-d4c.9"
		line.BeadTitle = "status plumbing"
		line.RunStatus = "running"
		line.Tokens = TokenUsage{Used: 12, HasUsed: true}
	})

	if err := shell.render(); err != nil {
		t.Fatalf("render with metadata: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Epic: obi-epic (automatic-octo-barnacle-d4c)") {
		t.Fatalf("expected epic info in header, got %q", output)
	}
	if !strings.Contains(output, "Bead: automatic-octo-barnacle-d4c.9 - status plumbing") {
		t.Fatalf("expected bead info in header, got %q", output)
	}
	if !strings.Contains(output, "Status: running") {
		t.Fatalf("expected status label, got %q", output)
	}
	if !strings.Contains(output, "Tokens: 12/--") {
		t.Fatalf("expected token placeholders, got %q", output)
	}
}

type fakeTerminal struct {
	width        int
	height       int
	restoreCount int
}

func (f *fakeTerminal) makeRaw(int) (*termState, error) {
	return &termState{}, nil
}

func (f *fakeTerminal) restore(int, *termState) error {
	f.restoreCount++
	return nil
}

func (f *fakeTerminal) getSize(int) (int, int, error) {
	return f.width, f.height, nil
}

func makeExitEvent(code int, err error) interactive.SessionEvent {
	return interactive.SessionEvent{Type: interactive.EventExit, ExitCode: code, Error: err}
}
