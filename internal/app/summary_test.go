package app

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

func TestChunkSummaryEntries(t *testing.T) {
	entries := []summaryEntry{
		{BeadID: "a", CommitSummary: "A"},
		{BeadID: "b", CommitSummary: "B"},
		{BeadID: "c", CommitSummary: "C"},
	}

	chunks := chunkSummaryEntries(entries, 2)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Index != 1 || len(chunks[0].Entries) != 2 {
		t.Fatalf("unexpected first chunk: %+v", chunks[0])
	}
	if chunks[1].Index != 2 || len(chunks[1].Entries) != 1 {
		t.Fatalf("unexpected second chunk: %+v", chunks[1])
	}
}

func TestLoadSummaryEntriesTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	base := ledgerEntry{
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusSuccess,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
	}

	for i := 0; i < 3; i++ {
		entry := base
		entry.BeadID = fmt.Sprintf("automatic-octo-barnacle-d4c.%d", i+1)
		entry.CommitSummary = fmt.Sprintf("summary-%d", i+1)
		entry.CommitDetails = fmt.Sprintf("details-%d", i+1)
		entry.CompletedAt = base.CompletedAt.Add(time.Duration(i) * time.Minute)
		if err := appendLedgerEntry(path, entry); err != nil {
			t.Fatalf("append entry: %v", err)
		}
	}

	entries, total, err := loadSummaryEntries(path, "automatic-octo-barnacle-d4c", 2)
	if err != nil {
		t.Fatalf("load summary entries: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after truncation, got %d", len(entries))
	}
	if entries[0].BeadID != "automatic-octo-barnacle-d4c.2" || entries[1].BeadID != "automatic-octo-barnacle-d4c.3" {
		t.Fatalf("unexpected entries order: %+v", entries)
	}
}

func TestLoadSummaryEntriesFailsOnNeedsHelp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.log")

	entry := ledgerEntry{
		EpicID:        "automatic-octo-barnacle-d4c",
		EpicKey:       "automatic",
		EpicName:      "Demo",
		Alias:         "demo",
		Status:        footer.StatusFailure,
		CommitSummary: "summary",
		CommitDetails: "details",
		StartedAt:     time.Unix(0, 0),
		CompletedAt:   time.Unix(0, 0),
		BeadID:        "automatic-octo-barnacle-d4c.9",
	}
	if err := appendLedgerEntry(path, entry); err != nil {
		t.Fatalf("append entry: %v", err)
	}

	if _, _, err := loadSummaryEntries(path, "automatic-octo-barnacle-d4c", 5); err == nil {
		t.Fatalf("expected error when ledger has needs_help entry")
	}
}
