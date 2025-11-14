package codexexec

import (
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func TestBuildDefaults(t *testing.T) {
	inv, err := Build(config.CodexConfig{}, "prompt")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if inv.Binary != "codex" {
		t.Fatalf("expected default binary codex, got %s", inv.Binary)
	}
	if len(inv.Args) == 0 || inv.Args[len(inv.Args)-1] != "prompt" {
		t.Fatalf("prompt not appended: %v", inv.Args)
	}
}

func TestBuildIncludesFlags(t *testing.T) {
	cfg := config.CodexConfig{
		Binary:    "codex-beta",
		Model:     "o3",
		Sandbox:   "workspace-write",
		Approval:  "on-request",
		ExtraArgs: []string{"--search"},
	}
	inv, err := Build(cfg, "prompt")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if inv.Binary != "codex-beta" {
		t.Fatalf("expected custom binary, got %s", inv.Binary)
	}
	want := []string{"exec", "--model", "o3", "--sandbox", "workspace-write", "--ask-for-approval", "on-request", "--search", "prompt"}
	if got := len(inv.Args); got != len(want) {
		t.Fatalf("arg length mismatch: got %d want %d", got, len(want))
	}
	for i, arg := range want {
		if inv.Args[i] != arg {
			t.Fatalf("arg %d mismatch: got %s want %s", i, inv.Args[i], arg)
		}
	}
}
