package tui

import (
	"fmt"
	"testing"
)

func TestLogPaneAppendAndVisible(t *testing.T) {
	pane := newLogPane(3)
	pane.append("line1\nline2")
	pane.append(" tail\nline3\n")

	lines := pane.visible(5)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[2] != "line3" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestLogPanePartialFlush(t *testing.T) {
	pane := newLogPane(2)
	pane.append("partial")
	if got := pane.visible(2); len(got) != 1 || got[0] != "partial" {
		t.Fatalf("expected partial in visible slice, got %v", got)
	}
	pane.flushPartial()
	lines := pane.visible(2)
	if len(lines) != 1 || lines[0] != "partial" {
		t.Fatalf("expected flushed line, got %v", lines)
	}
}

func TestLogPaneScrollBy(t *testing.T) {
	pane := newLogPane(10)
	for i := 0; i < 5; i++ {
		pane.append("line")
		pane.append("\n")
	}
	pane.scrollBy(2)
	lines := pane.visible(2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 visible lines, got %d", len(lines))
	}
	pane.scrollBy(-100)
	lines = pane.visible(2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 visible lines at bottom, got %d", len(lines))
	}
}

func TestLogPanePauseAndResume(t *testing.T) {
	pane := newLogPane(10)
	pane.append("one\n")
	pane.append("two\n")
	if state := pane.setPaused(true); !state {
		t.Fatalf("expected pane to be paused")
	}
	pane.append("three\n")

	lines := pane.visible(10)
	if len(lines) != 2 || lines[len(lines)-1] != "two" {
		t.Fatalf("expected paused view to ignore new lines, got %v", lines)
	}

	pane.setPaused(false)
	lines = pane.visible(10)
	if len(lines) != 3 || lines[len(lines)-1] != "three" {
		t.Fatalf("expected resumed view to include buffered lines, got %v", lines)
	}
}

func TestLogPaneTruncatesWhilePaused(t *testing.T) {
	pane := newLogPane(3)
	for i := 0; i < 3; i++ {
		pane.append(fmt.Sprintf("line%d\n", i))
	}
	pane.setPaused(true)
	for i := 3; i < 8; i++ {
		pane.append(fmt.Sprintf("line%d\n", i))
	}
	lines := pane.visible(3)
	if len(lines) != 3 {
		t.Fatalf("expected max lines preserved, got %v", lines)
	}
	if lines[0] != "line5" {
		t.Fatalf("expected older lines to be dropped once truncated, got %v", lines)
	}
}
