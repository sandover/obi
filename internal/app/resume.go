package app

import (
	"fmt"
	"strings"
)

func enableResume(plan *sessionPlan, logPath string) error {
	if plan.EpicID == "" || plan.EpicID == "issues" {
		return fmt.Errorf("--resume requires targeting a specific epic, but plan id is %q", plan.EpicID)
	}
	if strings.TrimSpace(logPath) == "" {
		return fmt.Errorf("results log path required for --resume")
	}
	completed, err := completedBeadsFromLedger(logPath, plan.EpicID)
	if err != nil {
		return err
	}
	plan.ResumeEnabled = true
	plan.ResumeCompletedBeads = completed
	return nil
}
