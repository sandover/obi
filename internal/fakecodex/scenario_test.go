package fakecodex

import (
	"bytes"
	"strings"
	"testing"
)

func TestExtractSessionID(t *testing.T) {
	prompt := "Some instructions\n```obi:abc-123-def\nstatus: success\n```\n"
	if got := ExtractSessionID(prompt); got != "abc-123-def" {
		t.Fatalf("expected abc-123-def, got %q", got)
	}
	if got := ExtractSessionID("no fence here"); got != "" {
		t.Fatalf("expected empty match, got %q", got)
	}
}

func TestScenarioRunAppliesPlaceholders(t *testing.T) {
	scenario := Scenario{
		Name: "test",
		Steps: []Step{
			{Stream: "stdout", Text: "hello {{SESSION_ID}}\n"},
			{Stream: "stderr", Text: "bye {{SESSION_ID}}\n", Repeat: 2},
		},
	}
	var out bytes.Buffer
	var err bytes.Buffer
	ctx := Context{SessionID: "abc"}
	if errRun := scenario.Run(ctx, &out, &err); errRun != nil {
		t.Fatalf("run scenario: %v", errRun)
	}
	if !strings.Contains(out.String(), "hello abc") {
		t.Fatalf("stdout missing session id: %q", out.String())
	}
	if got := strings.Count(err.String(), "bye abc"); got != 2 {
		t.Fatalf("expected repeated stderr, got %d entries", got)
	}
}

func TestLookupUnknownFallsBackToSuccess(t *testing.T) {
	fallback := Lookup("does-not-exist")
	if fallback.Name != "success" {
		t.Fatalf("expected fallback scenario to be success, got %s", fallback.Name)
	}
}
