package fakecodex

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Context captures metadata extracted from the Codex prompt.
type Context struct {
	SessionID string
	Prompt    string
}

// Step describes a single scripted action emitted by the fake Codex.
type Step struct {
	Stream string
	Text   string
	Repeat int
	Sleep  time.Duration
}

// Scenario defines one deterministic fake Codex transcript.
type Scenario struct {
	Name     string
	Steps    []Step
	ExitCode int
}

var sessionPattern = regexp.MustCompile("```obi:([a-z0-9\\-]+)")

// ExtractSessionID returns the fenced report UUID embedded in the prompt.
func ExtractSessionID(prompt string) string {
	matches := sessionPattern.FindStringSubmatch(strings.ToLower(prompt))
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

// Run writes the scenario's scripted output to the provided streams.
func (s Scenario) Run(ctx Context, stdout, stderr io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	for _, step := range s.Steps {
		if err := executeStep(step, ctx, stdout, stderr); err != nil {
			return err
		}
	}
	return nil
}

func executeStep(step Step, ctx Context, stdout, stderr io.Writer) error {
	repeat := step.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	switch strings.ToLower(step.Stream) {
	case "stdout":
		payload := renderText(step.Text, ctx)
		return writeRepeated(stdout, payload, repeat)
	case "stderr":
		payload := renderText(step.Text, ctx)
		return writeRepeated(stderr, payload, repeat)
	case "sleep":
		target := step.Sleep
		if target <= 0 {
			target = time.Millisecond * 5
		}
		time.Sleep(target)
		return nil
	default:
		return fmt.Errorf("fake codex: unknown step stream %q", step.Stream)
	}
}

func writeRepeated(dst io.Writer, text string, repeat int) error {
	for i := 0; i < repeat; i++ {
		if _, err := io.Copy(dst, bytes.NewBufferString(text)); err != nil {
			return err
		}
	}
	return nil
}

func renderText(body string, ctx Context) string {
	replacer := strings.NewReplacer(
		"{{SESSION_ID}}", ctx.SessionID,
	)
	return replacer.Replace(body)
}
