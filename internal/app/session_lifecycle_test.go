package app

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

type fakeSessionControl struct {
	softStopReasons []string
	aborts          int
	softStopErr     error
	abortErr        error
}

func (f *fakeSessionControl) SoftStop(reason string) error {
	f.softStopReasons = append(f.softStopReasons, reason)
	return f.softStopErr
}

func (f *fakeSessionControl) Abort() error {
	f.aborts++
	return f.abortErr
}

func TestSignalRelaySoftStopThenAbort(t *testing.T) {
	ctrl := &fakeSessionControl{}
	var buf bytes.Buffer
	relay := newSignalRelay(ctrl, &buf)

	relay.handleSignal(os.Interrupt)
	relay.handleSignal(os.Interrupt)

	if len(ctrl.softStopReasons) != 1 {
		t.Fatalf("expected one soft stop, got %d", len(ctrl.softStopReasons))
	}
	if ctrl.softStopReasons[0] != "Operator pressed Ctrl+C" {
		t.Fatalf("unexpected reason: %s", ctrl.softStopReasons[0])
	}
	if ctrl.aborts != 1 {
		t.Fatalf("expected abort after second ctrl+c, got %d", ctrl.aborts)
	}
	out := buf.String()
	if !strings.Contains(out, "requesting soft stop") {
		t.Fatalf("missing soft stop log: %s", out)
	}
	if !strings.Contains(out, "Second Ctrl+C") {
		t.Fatalf("missing abort log: %s", out)
	}
}

func TestSignalRelayImmediateAbortOnTerm(t *testing.T) {
	ctrl := &fakeSessionControl{}
	var buf bytes.Buffer
	relay := newSignalRelay(ctrl, &buf)

	relay.handleSignal(syscall.SIGTERM)

	if ctrl.aborts != 1 {
		t.Fatalf("expected abort on SIGTERM, got %d", ctrl.aborts)
	}
	if !strings.Contains(buf.String(), "abort") {
		t.Fatalf("expected abort log, got %s", buf.String())
	}
}

func TestSignalRelayLogsErrors(t *testing.T) {
	ctrl := &fakeSessionControl{softStopErr: fmt.Errorf("oops"), abortErr: fmt.Errorf("boom")}
	var buf bytes.Buffer
	relay := newSignalRelay(ctrl, &buf)

	relay.handleSignal(os.Interrupt)
	relay.handleSignal(os.Interrupt)

	out := buf.String()
	if !strings.Contains(out, "Soft stop failed") {
		t.Fatalf("expected soft stop error log, got %s", out)
	}
	if !strings.Contains(out, "Abort failed") {
		t.Fatalf("expected abort error log, got %s", out)
	}
}
