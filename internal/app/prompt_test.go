package app

import (
	"strings"
	"testing"
)

func TestBuildPromptIncludesAllSections(t *testing.T) {
	plan := sessionPlan{
		BasePrompt: "Base text",
		EpicPrompt: "Epic text",
		EpicID:     "automatic-octo-barnacle-xyz",
		Tool:       "orderscope",
	}

	got := buildPrompt(plan)
	parts := []string{
		"Base text",
		"Epic text",
		"Epic ID: automatic-octo-barnacle-xyz",
		"Tool: orderscope",
		"Epic completion contract",
	}
	for _, part := range parts {
		if !strings.Contains(got, part) {
			t.Fatalf("expected prompt to include %q, got %q", part, got)
		}
	}
}

func TestBuildPromptHandlesMissingSections(t *testing.T) {
	plan := sessionPlan{
		EpicID: "automatic-octo-barnacle-xyz",
	}

	got := buildPrompt(plan)
	if strings.Count(got, "Epic ID") != 1 {
		t.Fatalf("expected single epic line, got %q", got)
	}
}

func TestCompletionContractForIssues(t *testing.T) {
	plan := sessionPlan{
		EpicID: "issues",
	}
	got := buildPrompt(plan)
	if !strings.Contains(got, "Loose-issue contract") {
		t.Fatalf("expected loose issue contract, got %q", got)
	}
}

func TestBuildPromptIncludesResumeSection(t *testing.T) {
	plan := sessionPlan{
		EpicID:               "automatic-octo-barnacle-d4c",
		ResumeEnabled:        true,
		ResumeCompletedBeads: []string{"automatic-octo-barnacle-d4c.1"},
	}
	got := buildPrompt(plan)
	if !strings.Contains(got, "Resume mode") {
		t.Fatalf("expected resume instructions, got %q", got)
	}
	if !strings.Contains(got, "automatic-octo-barnacle-d4c.1") {
		t.Fatalf("expected bead id in resume block, got %q", got)
	}
}

func TestBuildSummaryPrompt(t *testing.T) {
	plan := sessionPlan{
		EpicID:          "automatic-octo-barnacle-d4c",
		Mode:            sessionModeSummary,
		SummaryPrompt:   "Summarize these commits.",
		SummaryIncluded: 2,
		SummaryTotal:    3,
		SummaryChunks: []summaryChunk{
			{
				Index: 1,
				Entries: []summaryEntry{
					{
						BeadID:        "automatic-octo-barnacle-d4c.1",
						CommitSummary: "Add API",
						CommitDetails: "Implemented endpoint",
					},
				},
			},
		},
	}

	got := buildPrompt(plan)
	for _, expected := range []string{
		"Summarize these commits.",
		"Chunk 1",
		"automatic-octo-barnacle-d4c.1",
		"Add API",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected summary prompt to include %q, got %q", expected, got)
		}
	}
}
