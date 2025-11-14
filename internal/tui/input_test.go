package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestInputRouterPassThrough(t *testing.T) {
	session := &fakeSessionControls{}
	router := NewInputRouter(session, nil)
	if err := router.HandleBytes([]byte("abc")); err != nil {
		t.Fatalf("handle bytes: %v", err)
	}
	if got := session.joinWrites(); got != "abc" {
		t.Fatalf("expected writes to reach session, got %q", got)
	}
}

func TestInputRouterHandlesHotkeys(t *testing.T) {
	session := &fakeSessionControls{}
	shell := &fakeShellBindings{}
	router := NewInputRouter(session, shell)

	if err := router.HandleBytes([]byte("p")); err != nil {
		t.Fatalf("toggle pause: %v", err)
	}
	if !shell.paused {
		t.Fatalf("expected pause toggle to flip state")
	}

	if err := router.HandleBytes([]byte("?")); err != nil {
		t.Fatalf("toggle help: %v", err)
	}
	if !shell.helpVisible {
		t.Fatalf("expected help overlay to toggle on")
	}

	if err := router.HandleBytes([]byte("s")); err != nil {
		t.Fatalf("soft stop: %v", err)
	}
	if len(session.softStops) != 1 {
		t.Fatalf("expected one soft stop request, got %v", session.softStops)
	}

	if err := router.HandleBytes([]byte("q")); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if session.abortCount != 1 {
		t.Fatalf("expected abort to be invoked once, got %d", session.abortCount)
	}

	if err := router.HandleBytes([]byte{'x'}); err != nil {
		t.Fatalf("passthrough byte: %v", err)
	}
	if got := session.joinWrites(); !strings.Contains(got, "x") {
		t.Fatalf("expected passthrough byte to reach session, got %q", got)
	}
}

func TestInputRouterHintLifecycle(t *testing.T) {
	session := &fakeSessionControls{}
	shell := &fakeShellBindings{}
	hints := &fakeHintSubmitter{}
	router := NewInputRouter(session, shell, WithHintSubmitter(hints))

	sequence := []byte{'h', 'f', 'o', 'o', 0x7f, 'o', 'd', '\r'}
	if err := router.HandleBytes(sequence); err != nil {
		t.Fatalf("handle hint sequence: %v", err)
	}

	if shell.hintActive {
		t.Fatalf("expected hint mode to exit after submit")
	}
	if len(hints.submissions) != 1 || hints.submissions[0] != "food" {
		t.Fatalf("unexpected hint submissions: %v", hints.submissions)
	}
}

func TestInputRouterHintCancel(t *testing.T) {
	session := &fakeSessionControls{}
	shell := &fakeShellBindings{}
	hints := &fakeHintSubmitter{}
	router := NewInputRouter(session, shell, WithHintSubmitter(hints))

	if err := router.HandleBytes([]byte{'h', 'a', 'b', 0x1b}); err != nil {
		t.Fatalf("handle cancel: %v", err)
	}
	if shell.hintActive {
		t.Fatalf("expected hint mode to be inactive")
	}
	if len(hints.submissions) != 0 {
		t.Fatalf("expected no submissions on cancel, got %v", hints.submissions)
	}
}

func TestInputRouterHintSubmissionErrorPropagates(t *testing.T) {
	session := &fakeSessionControls{}
	shell := &fakeShellBindings{}
	hints := &fakeHintSubmitter{err: errors.New("fail")}
	router := NewInputRouter(session, shell, WithHintSubmitter(hints))

	if err := router.HandleBytes([]byte{'h', 'o', 'k', '\r'}); err == nil {
		t.Fatalf("expected submission error")
	}
}

// --- fakes ---

type fakeSessionControls struct {
	writes     []string
	softStops  []string
	abortCount int
}

func (f *fakeSessionControls) WriteInput(b []byte) (int, error) {
	f.writes = append(f.writes, string(b))
	return len(b), nil
}

func (f *fakeSessionControls) SoftStop(reason string) error {
	f.softStops = append(f.softStops, reason)
	return nil
}

func (f *fakeSessionControls) Abort() error {
	f.abortCount++
	return nil
}

func (f *fakeSessionControls) joinWrites() string {
	var out string
	for _, w := range f.writes {
		out += w
	}
	return out
}

type fakeShellBindings struct {
	paused      bool
	helpVisible bool
	hintActive  bool
	hintText    string
}

func (f *fakeShellBindings) TogglePause() bool {
	f.paused = !f.paused
	return f.paused
}

func (f *fakeShellBindings) SetHintInput(active bool, text string) {
	f.hintActive = active
	f.hintText = text
}

func (f *fakeShellBindings) ToggleHelp() bool {
	f.helpVisible = !f.helpVisible
	return f.helpVisible
}

type fakeHintSubmitter struct {
	submissions []string
	err         error
}

func (f *fakeHintSubmitter) SubmitHint(text string) error {
	f.submissions = append(f.submissions, text)
	return f.err
}
