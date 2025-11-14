package app

import (
	"errors"
	"fmt"
	"strings"
)

func ensureReadyWork(plan sessionPlan) error {
	hasWork, err := readyWorkAvailable(plan)
	if err != nil {
		return err
	}
	if hasWork {
		return nil
	}
	if plan.EpicID == "" || plan.EpicID == "issues" {
		return nil
	}
	return errors.New(missingReadyBeadsWarning(plan.EpicID))
}

func readyWorkAvailable(plan sessionPlan) (bool, error) {
	if plan.EpicID == "" || plan.EpicID == "issues" {
		return true, nil
	}

	readyIssues, err := fetchReadyIssues()
	if err != nil {
		return false, fmt.Errorf("preflight ready check: %w", err)
	}
	return hasReadyIssueForPlan(plan, readyIssues)
}

func hasReadyIssueForPlan(plan sessionPlan, readyIssues []readyIssue) (bool, error) {
	if plan.EpicID == "" || plan.EpicID == "issues" {
		return true, nil
	}

	skip := plan.resumeSkipSet()
	var skippedMatches int

	for _, issue := range readyIssues {
		if strings.EqualFold(issue.IssueType, "epic") {
			continue
		}
		if issueBelongsToEpic(issue.ID, plan.EpicID) {
			if skip != nil {
				if _, ok := skip[strings.ToLower(issue.ID)]; ok {
					skippedMatches++
					continue
				}
			}
			return true, nil
		}
	}

	if plan.ResumeEnabled && skip != nil && skippedMatches > 0 {
		return false, fmt.Errorf("resume requested but every ready bead for %s is already logged as completed; create new beads or rerun without --resume", plan.EpicID)
	}

	return false, nil
}

func missingReadyBeadsWarning(epicID string) string {
	return fmt.Sprintf("no ready beads with prefix %s were returned by `bd ready --json -n %s`. Rename or recreate tasks as %s.<suffix> before rerunning.", epicID, readyFetchLimit, epicID)
}
