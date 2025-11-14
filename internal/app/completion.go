package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func runCompletion(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("obi completion requires a format (e.g., 'zsh')")
	}

	format := args[0]
	rest := args[1:]

	switch format {
	case "zsh":
		return runCompletionZsh(rest)
	default:
		return fmt.Errorf("unknown completion format %q", format)
	}
}

func runCompletionZsh(args []string) error {
	fs := flag.NewFlagSet("completion zsh", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	fs.StringVar(&configPath, "config", "", "path to obi.toml (defaults to nearest)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	resolved, err := config.ResolvePath(configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(resolved)
	if err != nil {
		return err
	}

	script := buildZshCompletionScript(cfg)
	fmt.Println(script)
	return nil
}

func buildZshCompletionScript(cfg *config.Config) string {
	handles := aliasHandles(cfg)
	var sb strings.Builder
	sb.WriteString("#compdef obi\n\n")
	sb.WriteString("_obi() {\n")
	sb.WriteString("  local -a _obi_subcommands\n")
	sb.WriteString("  _obi_subcommands=(\n")
	sb.WriteString("    'go:prepare or execute a Codex session'\n")
	sb.WriteString("    'init:scaffold or refresh obi.toml'\n")
	sb.WriteString("    'refresh:sync obi.toml with bead epics'\n")
	sb.WriteString("    'list:show available epics'\n")
	sb.WriteString("    'completion:generate shell completions'\n")
	sb.WriteString("  )\n")
	sb.WriteString("  local -a _obi_aliases\n")
	sb.WriteString("  _obi_aliases=(\n")
	for _, handle := range handles {
		sb.WriteString(fmt.Sprintf("    %s\n", zshQuote(handle)))
	}
	sb.WriteString("  )\n")
	sb.WriteString("  local state\n")
	sb.WriteString("  _arguments -C \\\n")
	sb.WriteString("    '1:command:->cmd' \\\n")
	sb.WriteString("    '2:alias:->alias' \\\n")
	sb.WriteString("    '*::arg:->args'\n")
	sb.WriteString("  case $state in\n")
	sb.WriteString("    cmd)\n")
	sb.WriteString("      _describe 'command' _obi_subcommands\n")
	sb.WriteString("      return\n")
	sb.WriteString("      ;;\n")
	sb.WriteString("    alias)\n")
	sb.WriteString("      if [[ $words[2] == go ]]; then\n")
	sb.WriteString("        _describe 'alias' _obi_aliases\n")
	sb.WriteString("        return\n")
	sb.WriteString("      fi\n")
	sb.WriteString("      ;;\n")
	sb.WriteString("  esac\n")
	sb.WriteString("}\n\n")
	sb.WriteString("_obi \"$@\"\n")
	return sb.String()
}

func zshQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
