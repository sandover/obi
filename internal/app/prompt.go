package app

import (
	"fmt"
	"strings"
)

const (
	epicCompletionTemplate = `Epic completion contract for %s (%s):
- Use "bd ready --json" and pick a bead whose ID starts with "%s."
- If "bd ready --json" returns no beads whose IDs start with "%s," run "bd show %s --json" to confirm the epic exists and emit STATUS: needs_help with ESCALATION describing which bead IDs you did find so humans can rename them.
- Claim it before coding: bd update <id> --status in_progress --json.
- When done and tests pass, close it via bd close <id> --reason "Completed" --json (or bd update <id> --status completed --json).
- Only emit STATUS: success after the bead is closed. Otherwise emit STATUS: needs_help with ESCALATION explaining the blocker.`

	issuesCompletionContract = `Loose-issue contract:
- Use "bd ready --json" and pick a bead that is not part of any epic.
- Claim it before coding: bd update <id> --status in_progress --json.
- When done and tests pass, close it via bd close <id> --reason "Completed" --json (or bd update <id> --status completed --json).
- Only emit STATUS: success after the bead is closed. Otherwise emit STATUS: needs_help with ESCALATION explaining the blocker.`
)

// buildPrompt merges base prompt text, epic-specific prompt, and metadata.
func buildPrompt(plan sessionPlan) string {
	if plan.Mode == sessionModeSummary {
		return buildSummaryPrompt(plan)
	}

	var sections []string

	if trimmed := strings.TrimSpace(plan.BasePrompt); trimmed != "" {
		sections = append(sections, trimmed)
	}
	if trimmed := strings.TrimSpace(plan.EpicPrompt); trimmed != "" {
		sections = append(sections, trimmed)
	}

	metaLines := []string{fmt.Sprintf("Epic ID: %s", plan.EpicID)}
	if plan.Tool != "" {
		metaLines = append(metaLines, fmt.Sprintf("Tool: %s", plan.Tool))
	}

	sections = append(sections, strings.Join(metaLines, "\n"))

	if instructions := resumeInstructions(plan); instructions != "" {
		sections = append(sections, instructions)
	}

	sections = append(sections, completionContract(plan))

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func completionContract(plan sessionPlan) string {
	if plan.EpicID == "" || plan.EpicID == "issues" {
		return issuesCompletionContract
	}
	name := strings.TrimSpace(plan.EpicName)
	if name == "" {
		name = plan.EpicID
	}
	return fmt.Sprintf(epicCompletionTemplate, name, plan.EpicID, plan.EpicID, plan.EpicID, plan.EpicID)
}

func resumeInstructions(plan sessionPlan) string {
	if !plan.ResumeEnabled {
		return ""
	}
	if len(plan.ResumeCompletedBeads) == 0 {
		return "Resume mode: continue working through ready beads for this epic."
	}
	lines := []string{
		"Resume mode is active – skip the beads already finished during this run:",
	}
	for _, id := range plan.ResumeCompletedBeads {
		lines = append(lines, fmt.Sprintf("- %s", id))
	}
	return strings.Join(lines, "\n")
}

func buildSummaryPrompt(plan sessionPlan) string {
	var sections []string

	if intro := strings.TrimSpace(plan.SummaryPrompt); intro != "" {
		sections = append(sections, intro)
	}

	metaLines := []string{fmt.Sprintf("Epic ID: %s", plan.EpicID)}
	if plan.SummaryIncluded > 0 {
		if plan.SummaryTotal > plan.SummaryIncluded {
			metaLines = append(metaLines, fmt.Sprintf("Showing the most recent %d of %d commits recorded for this epic.", plan.SummaryIncluded, plan.SummaryTotal))
		} else {
			metaLines = append(metaLines, fmt.Sprintf("Commits included: %d", plan.SummaryIncluded))
		}
	}
	sections = append(sections, strings.Join(metaLines, "\n"))

	for _, chunk := range plan.SummaryChunks {
		if text := formatSummaryChunk(chunk); text != "" {
			sections = append(sections, text)
		}
	}

	sections = append(sections, "Return a single omnibus commit message summarizing every chunk above.")

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func formatSummaryChunk(chunk summaryChunk) string {
	if len(chunk.Entries) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("Chunk %d:", chunk.Index))
	for _, entry := range chunk.Entries {
		bead := strings.TrimSpace(entry.BeadID)
		if bead == "" {
			bead = "(unidentified bead)"
		}
		summary := strings.TrimSpace(entry.CommitSummary)
		if summary == "" {
			summary = "(no commit summary captured)"
		}
		lines = append(lines, fmt.Sprintf("- %s — %s", bead, summary))
		if detail := indentMultiline(strings.TrimSpace(entry.CommitDetails), "    "); detail != "" {
			lines = append(lines, detail)
		}
	}
	return strings.Join(lines, "\n")
}

func indentMultiline(text, prefix string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
