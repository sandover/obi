package app

import (
	"strings"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func TestFormatEpicRows(t *testing.T) {
	epics := map[string]config.EpicConfig{
		"foo": {Name: "Foo Work", ID: "automatic-octo-barnacle-foo", Alias: "foo"},
		"bar": {Name: "Bar Tasks", ID: "automatic-octo-barnacle-bar", Alias: "bar"},
	}
	readyCounts := map[string]int{
		"automatic-octo-barnacle-foo": 2,
		"automatic-octo-barnacle-bar": 0,
	}
	totalCounts := map[string]int{
		"automatic-octo-barnacle-foo": 4,
		"automatic-octo-barnacle-bar": 1,
	}
	rows := buildEpicRows(epics, readyCounts, totalCounts)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	output := formatEpicRows(rows)
	if !strings.Contains(output, "Alias") || !strings.Contains(output, "Ready/Total") || !strings.Contains(output, "Epic ID") {
		t.Fatalf("missing header columns: %s", output)
	}
	if !strings.Contains(output, "Foo Work") || !strings.Contains(output, "Bar Tasks") {
		t.Fatalf("expected both epic names in table: %s", output)
	}
	if !strings.Contains(output, "2/4") {
		t.Fatalf("expected ready/total column to show 2/4: %s", output)
	}
	if !strings.Contains(output, "0/1 (!)") {
		t.Fatalf("expected warning marker for zero-ready row: %s", output)
	}
	if idx := strings.Index(output, "Epic ID"); idx == -1 {
		t.Fatalf("epic id column missing: %s", output)
	}
}

func TestSummarizeLooseIssues(t *testing.T) {
	issues := []readyIssue{
		{ID: "automatic-octo-barnacle-foo", IssueType: "feature", Description: "foo desc"},
		{ID: "automatic-octo-barnacle-foo.1", IssueType: "task"},
		{ID: "automatic-octo-barnacle-eh2", IssueType: "epic"},
		{ID: "automatic-octo-barnacle-bar", IssueType: "bug", Title: "fallback"},
	}
	summary := summarizeLooseIssues(issues, nil)
	if summary.Err != nil {
		t.Fatalf("summarize: %v", summary.Err)
	}
	if summary.Count != 2 {
		t.Fatalf("expected 2 loose issues, got %d", summary.Count)
	}
	for _, entry := range summary.Entries {
		if entry.Description == "" {
			t.Fatalf("expected description for entry: %+v", entry)
		}
	}
}

func TestCollectZeroReadyIncludesGuardrailMessage(t *testing.T) {
	row := epicRow{
		Alias:      "foo",
		EpicID:     "automatic-octo-barnacle-foo",
		Name:       "Foo Work",
		ReadyCount: ptrInt(0),
		TotalCount: ptrInt(3),
		Warn:       true,
	}
	warnings := collectZeroReady([]epicRow{row})
	if len(warnings) != 1 {
		t.Fatalf("expected single warning, got %d", len(warnings))
	}
	want := missingReadyBeadsWarning(row.EpicID)
	if warnings[0].Message != want {
		t.Fatalf("warning message mismatch:\nwant: %s\n got: %s", want, warnings[0].Message)
	}
	if warnings[0].Total != 3 {
		t.Fatalf("expected total count 3, got %d", warnings[0].Total)
	}
}
