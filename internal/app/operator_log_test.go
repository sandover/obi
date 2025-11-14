package app

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestOperatorLogLedgerEventsRedactsAndMirrors(t *testing.T) {
	var buf bytes.Buffer
	log := newOperatorLog(&buf)
	log.now = func() time.Time { return time.Unix(0, 0) }

	log.record(operatorEventHint, "secret plan goes here")
	log.record(operatorEventSoftStop, "wrap up please")

	events := log.ledgerEvents([]string{"secret"})
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Kind != string(operatorEventHint) {
		t.Fatalf("unexpected event kind %q", events[0].Kind)
	}
	if !strings.Contains(events[0].Message, "[REDACTED]") {
		t.Fatalf("expected hint message to be redacted, got %q", events[0].Message)
	}
	if events[1].Kind != string(operatorEventSoftStop) {
		t.Fatalf("unexpected second event kind %q", events[1].Kind)
	}
	if !strings.Contains(buf.String(), "[obi operator hint] secret plan goes here") {
		t.Fatalf("expected mirror output for hint, got %q", buf.String())
	}
}
