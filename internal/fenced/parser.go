package fenced

import (
	"fmt"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

// Result captures the structured data inside a fenced Obi report.
type Result struct {
	SessionID  string
	Status     string
	CommitMsg  string
	Details    string
	Escalation string
}

type parserState int

const (
	stateSeeking parserState = iota
	stateInBody
	stateFinished
)

// Parser consumes streaming Codex output and detects fenced Obi reports.
type Parser struct {
	expectedID string
	state      parserState
	hold       string

	result            Result
	done              bool
	collectingDetails bool
	details           strings.Builder
}

// NewParser constructs a parser expecting the provided session UUID.
func NewParser(sessionID string) *Parser {
	return &Parser{expectedID: strings.TrimSpace(sessionID)}
}

// Feed ingests streamed chunks; it returns a parsed result once the fence closes.
func (p *Parser) Feed(chunk string) (Result, bool, error) {
	if chunk == "" || p.done {
		if p.done {
			return p.result, true, nil
		}
		return Result{}, false, nil
	}
	chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
	chunk = strings.ReplaceAll(chunk, "\r", "\n")
	p.hold += chunk

	for {
		idx := strings.IndexByte(p.hold, '\n')
		if idx == -1 {
			break
		}
		line := p.hold[:idx]
		p.hold = p.hold[idx+1:]
		if err := p.handleLine(line); err != nil {
			return Result{}, false, err
		}
		if p.done {
			return p.result, true, nil
		}
	}

	return Result{}, false, nil
}

// Finalize flushes any buffered text when the Codex stream ends.
func (p *Parser) Finalize() (Result, bool, error) {
	if p.done {
		return p.result, true, nil
	}
	if p.hold != "" {
		if err := p.handleLine(p.hold); err != nil {
			return Result{}, false, err
		}
		p.hold = ""
	}
	if p.done {
		return p.result, true, nil
	}
	switch p.state {
	case stateSeeking:
		return Result{}, false, fmt.Errorf("fenced report not found")
	case stateInBody:
		return Result{}, false, fmt.Errorf("fenced report did not close before stream ended")
	default:
		return Result{}, false, fmt.Errorf("unexpected parser state")
	}
}

func (p *Parser) handleLine(line string) error {
	trimmed := strings.TrimSpace(line)

	switch p.state {
	case stateSeeking:
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "```obi:") {
			sessionID := strings.TrimSpace(trimmed[len("```obi:"):])
			if sessionID == "" {
				return fmt.Errorf("fence missing session id")
			}
			if p.expectedID != "" && sessionID != p.expectedID {
				return fmt.Errorf("fence session id %s does not match expected %s", sessionID, p.expectedID)
			}
			p.state = stateInBody
			p.result = Result{SessionID: sessionID}
		}
		return nil

	case stateInBody:
		if trimmed == "```" {
			if p.collectingDetails {
				if err := p.finishDetails(); err != nil {
					return err
				}
				p.collectingDetails = false
			}
			if err := p.validateResult(); err != nil {
				return err
			}
			p.done = true
			p.state = stateFinished
			return nil
		}

		if p.collectingDetails {
			consumed, err := p.consumeDetailLine(line)
			if err != nil {
				return err
			}
			if consumed {
				return nil
			}
			trimmed = strings.TrimSpace(line)
		}

		if trimmed == "" {
			return nil
		}
		return p.processField(trimmed)
	default:
		return nil
	}
}

func (p *Parser) processField(line string) error {
	idx := strings.Index(line, ":")
	if idx == -1 {
		return fmt.Errorf("malformed line inside fenced report: %q", line)
	}
	key := strings.ToLower(strings.TrimSpace(line[:idx]))
	value := strings.TrimSpace(line[idx+1:])

	switch key {
	case "status":
		if value == "" {
			return fmt.Errorf("status field is empty")
		}
		lower := strings.ToLower(value)
		if lower != footer.StatusSuccess && lower != footer.StatusFailure {
			return fmt.Errorf("invalid status %q", value)
		}
		p.result.Status = lower
	case "commit_msg":
		if value == "" {
			return fmt.Errorf("commit_msg field is empty")
		}
		p.result.CommitMsg = value
	case "details":
		if p.result.Details != "" || p.collectingDetails {
			return fmt.Errorf("details field specified multiple times")
		}
		if value == "" {
			return fmt.Errorf("details field is empty")
		}
		if value == "|" {
			p.collectingDetails = true
			p.details.Reset()
			return nil
		}
		p.result.Details = value
	case "escalation":
		p.result.Escalation = value
	default:
		return fmt.Errorf("unknown field %q in fenced report", key)
	}
	return nil
}

// consumeDetailLine appends block content; it returns false when the caller should
// reprocess the same line (typically because a new field began).
func (p *Parser) consumeDetailLine(line string) (bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		p.details.WriteByte('\n')
		return true, nil
	}
	if trimmed == "```" {
		if err := p.finishDetails(); err != nil {
			return false, err
		}
		p.collectingDetails = false
		if err := p.validateResult(); err != nil {
			return false, err
		}
		p.done = true
		p.state = stateFinished
		return true, nil
	}
	if !startsIndented(line) && isFieldLine(trimmed) {
		if err := p.finishDetails(); err != nil {
			return false, err
		}
		p.collectingDetails = false
		return false, nil
	}

	p.details.WriteString(stripIndent(line))
	p.details.WriteByte('\n')
	return true, nil
}

func (p *Parser) finishDetails() error {
	text := strings.TrimRight(p.details.String(), "\n")
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("details block must include content")
	}
	p.result.Details = text
	return nil
}

func (p *Parser) validateResult() error {
	if p.result.SessionID == "" {
		return fmt.Errorf("fenced report missing session id")
	}
	if p.result.Status == "" {
		return fmt.Errorf("fenced report missing status line")
	}
	if p.result.CommitMsg == "" {
		return fmt.Errorf("fenced report missing commit_msg line")
	}
	if p.result.Details == "" {
		return fmt.Errorf("fenced report missing details block")
	}
	if p.result.Status == footer.StatusFailure && strings.TrimSpace(p.result.Escalation) == "" {
		return fmt.Errorf("status=%s requires escalation", footer.StatusFailure)
	}
	return nil
}

func isFieldLine(line string) bool {
	if line == "" {
		return false
	}
	if strings.HasPrefix(line, "`") {
		return false
	}
	return strings.Contains(line, ":")
}

func stripIndent(line string) string {
	if line == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(line, "\t"):
		return line[1:]
	case strings.HasPrefix(line, "  "):
		return line[2:]
	case strings.HasPrefix(line, " "):
		return line[1:]
	default:
		return line
	}
}

func startsIndented(line string) bool {
	if line == "" {
		return false
	}
	switch line[0] {
	case ' ', '\t':
		return true
	default:
		return false
	}
}
