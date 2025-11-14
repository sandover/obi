package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const readyFetchLimit = "200"

type readyIssue struct {
	ID          string `json:"id"`
	IssueType   string `json:"issue_type"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func fetchReadyIssues() ([]readyIssue, error) {
	cmd := exec.Command("bd", "ready", "--json", "-n", readyFetchLimit)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("bd ready: %s: %s", err, detail)
		}
		return nil, fmt.Errorf("bd ready: %w", err)
	}
	return parseReadyIssues(stdout.Bytes())
}

func parseReadyIssues(data []byte) ([]readyIssue, error) {
	var issues []readyIssue
	if err := json.Unmarshal(data, &issues); err != nil {
		return nil, fmt.Errorf("parse bd ready output: %w", err)
	}
	return issues, nil
}

func issueBelongsToEpic(issueID, epicID string) bool {
	if epicID == "" || epicID == "issues" {
		return false
	}
	return parentEpicID(issueID) == epicID
}

func parentEpicID(issueID string) string {
	idx := strings.Index(issueID, ".")
	if idx == -1 {
		return ""
	}
	return issueID[:idx]
}
