package footer

import (
	"fmt"
	"strings"
)

const (
	// Field prefixes emitted by Codex agents.
	StatusPrefix     = "STATUS:"
	CommitPrefix     = "COMMIT_MSG:"
	EscalationPrefix = "ESCALATION:"

	// StatusSuccess indicates the bead finished successfully.
	StatusSuccess = "success"
	// StatusFailure indicates the bead needs human intervention.
	StatusFailure = "needs_help"
)

// Result captures the structured footer emitted by Codex.
type Result struct {
	Status     string
	CommitMsg  string
	Escalation string
}

// Parse scans stdout/stderr for the required footer markers.
func Parse(output string) (Result, error) {
	var res Result
	var collectingCommit bool

	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, StatusPrefix):
			res.Status = strings.TrimSpace(strings.TrimPrefix(trimmed, StatusPrefix))
			collectingCommit = false
		case strings.HasPrefix(trimmed, CommitPrefix):
			res.CommitMsg = strings.TrimSpace(strings.TrimPrefix(trimmed, CommitPrefix))
			collectingCommit = true
		case strings.HasPrefix(trimmed, EscalationPrefix):
			res.Escalation = strings.TrimSpace(strings.TrimPrefix(trimmed, EscalationPrefix))
			collectingCommit = false
		default:
			if collectingCommit {
				if res.CommitMsg != "" {
					res.CommitMsg += "\n"
				}
				res.CommitMsg += trimmed
			}
		}
	}

	if res.Status == "" {
		return Result{}, fmt.Errorf("missing %s line", StatusPrefix)
	}
	if res.CommitMsg == "" {
		return Result{}, fmt.Errorf("missing %s line", CommitPrefix)
	}
	if res.Status == StatusFailure && res.Escalation == "" {
		return Result{}, fmt.Errorf("status=%s requires %s line", StatusFailure, EscalationPrefix)
	}

	return res, nil
}
