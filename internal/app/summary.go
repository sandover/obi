package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/footer"
)

type summaryEntry struct {
	BeadID        string
	CommitSummary string
	CommitDetails string
	CompletedAt   time.Time
}

type summaryChunk struct {
	Index   int
	Entries []summaryEntry
}

func maybeRunSummarizer(plan sessionPlan, opts goOptions, cfg *config.Config, logPath string) error {
	summaryCfg := cfg.SummaryConfigValue()
	if summaryCfg.MaxCommits <= 0 || strings.TrimSpace(summaryCfg.Prompt) == "" {
		fmt.Println("Omnibus summarizer disabled via config; skipping.")
		return nil
	}

	entries, total, err := loadSummaryEntries(logPath, plan.EpicID, summaryCfg.MaxCommits)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No completed beads found in the ledger; skipping omnibus summary.")
		return nil
	}

	chunks := chunkSummaryEntries(entries, summaryCfg.ChunkSize)

	summaryPlan := plan
	summaryPlan.Mode = sessionModeSummary
	summaryPlan.BasePrompt = ""
	summaryPlan.EpicPrompt = ""
	summaryPlan.SummaryPrompt = summaryCfg.Prompt
	summaryPlan.SummaryChunks = chunks
	summaryPlan.SummaryIncluded = len(entries)
	summaryPlan.SummaryTotal = total
	summaryPlan.ResumeEnabled = false
	summaryPlan.ResumeCompletedBeads = nil
	summaryPlan.BeadIDOverride = fmt.Sprintf("%s.omnibus-summary", plan.EpicID)
	summaryPlan.EpicName = fmt.Sprintf("%s – Omnibus Summary", plan.EpicName)

	fmt.Printf("Launching omnibus summarizer with %d commit(s) (%d total recorded).\n\n", len(entries), total)
	outcome, err := executeSession(summaryPlan, opts, cfg, logPath, false, false)
	if err != nil {
		return err
	}
	if outcome.Status == "" {
		fmt.Println("Summarizer cancelled by operator.")
		return nil
	}
	fmt.Println("Omnibus summary recorded.")
	return nil
}

func loadSummaryEntries(path, epicID string, maxCommits int) ([]summaryEntry, int, error) {
	rawEntries, err := ledgerEntriesForEpic(path, epicID)
	if err != nil {
		if errors.Is(err, errLedgerNotFound) {
			return nil, 0, fmt.Errorf("results log %s not found; cannot produce omnibus summary", path)
		}
		return nil, 0, err
	}

	var filtered []summaryEntry
	for _, entry := range rawEntries {
		status := strings.ToLower(strings.TrimSpace(entry.Status))
		switch status {
		case "":
			continue
		case footer.StatusSuccess:
			summary := strings.TrimSpace(entry.CommitSummary)
			details := strings.TrimSpace(entry.CommitDetails)
			if details == "" {
				details = summary
			}
			if summary == "" && details == "" {
				continue
			}
			filtered = append(filtered, summaryEntry{
				BeadID:        strings.TrimSpace(entry.BeadID),
				CommitSummary: summary,
				CommitDetails: details,
				CompletedAt:   entry.CompletedAt,
			})
		case footer.StatusFailure:
			return nil, 0, fmt.Errorf("session %s ended with status=%s; resolve blockers before running the omnibus summary", entry.SessionID, status)
		default:
			continue
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CompletedAt.Equal(filtered[j].CompletedAt) {
			return i < j
		}
		return filtered[i].CompletedAt.Before(filtered[j].CompletedAt)
	})

	total := len(filtered)
	if maxCommits > 0 && total > maxCommits {
		filtered = filtered[total-maxCommits:]
	}
	return filtered, total, nil
}

func chunkSummaryEntries(entries []summaryEntry, chunkSize int) []summaryChunk {
	if chunkSize <= 0 {
		chunkSize = 5
	}
	var chunks []summaryChunk
	for i := 0; i < len(entries); i += chunkSize {
		end := i + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunks = append(chunks, summaryChunk{
			Index:   len(chunks) + 1,
			Entries: entries[i:end],
		})
	}
	return chunks
}
