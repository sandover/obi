package main

import (
	"fmt"
	"io"
	"os"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/fakecodex"
)

const envScenario = "FAKE_CODEX_SCENARIO"

func main() {
	name := os.Getenv(envScenario)
	if name == "" {
		name = "success"
	}
	scenario := fakecodex.Lookup(name)

	prompt := readPrompt()
	ctx := fakecodex.Context{
		SessionID: fakecodex.ExtractSessionID(prompt),
		Prompt:    prompt,
	}
	if err := scenario.Run(ctx, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "fakecodex: %v\n", err)
		os.Exit(1)
	}
	os.Exit(scenario.ExitCode)
}

func readPrompt() string {
	if len(os.Args) > 1 {
		return os.Args[len(os.Args)-1]
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}
