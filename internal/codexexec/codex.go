package codexexec

import (
	"errors"
	"fmt"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

// Invocation describes a codex CLI invocation.
type Invocation struct {
	Binary string
	Args   []string
}

// Build produces command-line args for codex exec based on config + prompt.
func Build(cfg config.CodexConfig, prompt string) (Invocation, error) {
	bin := cfg.Binary
	if bin == "" {
		bin = "codex"
	}
	if prompt == "" {
		return Invocation{}, errors.New("empty prompt")
	}

	args := []string{"exec"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Sandbox != "" {
		args = append(args, "--sandbox", cfg.Sandbox)
	}
	if cfg.Approval != "" {
		args = append(args, "--ask-for-approval", cfg.Approval)
	}
	if len(cfg.ExtraArgs) > 0 {
		args = append(args, cfg.ExtraArgs...)
	}

	args = append(args, prompt)

	return Invocation{Binary: bin, Args: args}, nil
}

func (inv Invocation) String() string {
	return fmt.Sprintf("%s %v", inv.Binary, inv.Args)
}
