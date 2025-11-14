package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath string
	fs.StringVar(&configPath, "config", "", "path to obi.toml (defaults to nearest)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	resolved, err := config.ResolvePath(configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(resolved)
	if err != nil {
		return err
	}

	readyIssues, readyErr := fetchReadyIssues()
	loose := summarizeLooseIssues(readyIssues, readyErr)
	printLooseIssuesBlock(cfg, loose)

	var readyCounts map[string]int
	if readyErr == nil {
		readyCounts = summarizeReadyCounts(readyIssues)
	}

	openIssues, openErr := fetchOpenIssues()
	var totalCounts map[string]int
	if openErr == nil {
		totalCounts = summarizeOpenCounts(openIssues)
	}

	repoPath := repoRootForConfig(resolved)
	fmt.Printf("Epics in %s:\n", repoPath)
	rows := buildEpicRows(cfg.Epics, readyCounts, totalCounts)
	fmt.Print(formatEpicRows(rows))

	if readyErr != nil {
		fmt.Printf("\nReady counts unavailable: %s\n", readyErr)
	}
	if openErr != nil {
		fmt.Printf("\nOpen-counts unavailable: %s\n", openErr)
	}

	warnings := collectZeroReady(rows)
	if len(warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warn := range warnings {
			fmt.Printf("  - %s (%s): %s\n", warn.Alias, warn.EpicID, warn.Message)
			if warn.Total > 0 {
				fmt.Printf("      %d open issues currently match this epic.\n", warn.Total)
			}
		}
	}
	return nil
}

func printLooseIssuesBlock(cfg *config.Config, summary looseSummary) {
	label := `Standalone issues (do these by running plain "obi go")`
	if summary.Err != nil {
		fmt.Printf("%s: unavailable (%s)\n", label, summary.Err)
		fmt.Println()
		return
	}
	fmt.Printf("%s: %d\n", label, summary.Count)
	if summary.Count == 0 {
		fmt.Println()
		return
	}
	fmt.Println("  Task ID                      Description")
	fmt.Println("  ---------------------------  -------------------------")
	for _, entry := range summary.Entries {
		lines := wrapText(entry.Description, 25)
		fmt.Printf("  %-27s  %s\n", entry.ID, lines[0])
		for _, extra := range lines[1:] {
			fmt.Printf("  %-27s  %s\n", "", extra)
		}
	}
	fmt.Println()
}

func truncatePrompt(prompt string) string {
	if len(prompt) <= 80 {
		return prompt
	}
	return prompt[:77] + "..."
}

type epicRow struct {
	Alias      string
	Name       string
	EpicID     string
	ReadyCount *int
	TotalCount *int
	Warn       bool
}

func buildEpicRows(epics map[string]config.EpicConfig, readyCounts, totalCounts map[string]int) []epicRow {
	if len(epics) == 0 {
		return nil
	}
	rows := make([]epicRow, 0, len(epics))
	for _, key := range sortedEpicKeys(epics) {
		epic := epics[key]
		row := epicRow{
			Alias:  epicAliasHandle(key, epic),
			Name:   epic.Name,
			EpicID: epic.ID,
		}
		if readyCounts != nil {
			val := readyCounts[epic.ID]
			row.ReadyCount = ptrInt(val)
		}
		if totalCounts != nil {
			val := totalCounts[epic.ID]
			row.TotalCount = ptrInt(val)
		}
		if row.ReadyCount != nil && row.TotalCount != nil && *row.TotalCount > 0 && *row.ReadyCount == 0 {
			row.Warn = true
		}
		rows = append(rows, row)
	}
	return rows
}

func formatEpicRows(rows []epicRow) string {
	if len(rows) == 0 {
		return "  (none yet)\n"
	}
	aliasWidth := len("Alias")
	readyWidth := len("Ready/Total")
	nameWidth := len("Name")
	idWidth := len("Epic ID")
	readyTexts := make([]string, len(rows))
	for i, row := range rows {
		readyTexts[i] = readyTotalText(row)
		if len(row.Alias) > aliasWidth {
			aliasWidth = len(row.Alias)
		}
		if len(readyTexts[i]) > readyWidth {
			readyWidth = len(readyTexts[i])
		}
		if len(row.Name) > nameWidth {
			nameWidth = len(row.Name)
		}
		if len(row.EpicID) > idWidth {
			idWidth = len(row.EpicID)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %-*s  %-*s  %-*s  %-*s\n", aliasWidth, "Alias", readyWidth, "Ready/Total", nameWidth, "Name", idWidth, "Epic ID")
	fmt.Fprintf(&b, "  %-*s  %-*s  %-*s  %-*s\n",
		aliasWidth, strings.Repeat("-", aliasWidth),
		readyWidth, strings.Repeat("-", readyWidth),
		nameWidth, strings.Repeat("-", nameWidth),
		idWidth, strings.Repeat("-", idWidth),
	)
	for i, row := range rows {
		fmt.Fprintf(&b, "  %-*s  %-*s  %-*s  %-*s\n",
			aliasWidth, row.Alias,
			readyWidth, readyTexts[i],
			nameWidth, row.Name,
			idWidth, row.EpicID,
		)
	}
	return b.String()
}

func readyTotalText(row epicRow) string {
	ready := "?"
	total := "?"
	if row.ReadyCount != nil {
		ready = strconv.Itoa(*row.ReadyCount)
	}
	if row.TotalCount != nil {
		total = strconv.Itoa(*row.TotalCount)
	}
	text := fmt.Sprintf("%s/%s", ready, total)
	if row.Warn {
		text += " (!)"
	}
	return text
}

type zeroReadyWarning struct {
	Alias   string
	EpicID  string
	Total   int
	Message string
}

func collectZeroReady(rows []epicRow) []zeroReadyWarning {
	var warnings []zeroReadyWarning
	for _, row := range rows {
		if !row.Warn {
			continue
		}
		total := 0
		if row.TotalCount != nil {
			total = *row.TotalCount
		}
		warnings = append(warnings, zeroReadyWarning{
			Alias:   row.Alias,
			EpicID:  row.EpicID,
			Total:   total,
			Message: missingReadyBeadsWarning(row.EpicID),
		})
	}
	return warnings
}

func ptrInt(v int) *int {
	n := new(int)
	*n = v
	return n
}

func repoRootForConfig(configPath string) string {
	dir := filepath.Dir(configPath)
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		root := strings.TrimSpace(stdout.String())
		if root != "" {
			return root
		}
	}
	return dir
}

type looseSummary struct {
	Count   int
	Entries []readyIssue
	Err     error
}

func summarizeLooseIssues(issues []readyIssue, err error) looseSummary {
	if err != nil {
		return looseSummary{Err: err}
	}
	var entries []readyIssue
	for _, issue := range issues {
		if !isLooseIssue(issue) {
			continue
		}
		if issue.Description == "" {
			issue.Description = issue.Title
		}
		entries = append(entries, issue)
	}
	return looseSummary{Count: len(entries), Entries: entries}
}

func isLooseIssue(issue readyIssue) bool {
	if strings.EqualFold(issue.IssueType, "epic") {
		return false
	}
	return !strings.Contains(issue.ID, ".")
}

type listIssue struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	IssueType string `json:"issue_type"`
}

func fetchOpenIssues() ([]listIssue, error) {
	cmd := exec.Command("bd", "list", "--json", "--all")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("bd list: %s: %s", err, detail)
		}
		return nil, fmt.Errorf("bd list: %w", err)
	}
	var issues []listIssue
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parse bd list output: %w", err)
	}
	return issues, nil
}

func summarizeReadyCounts(issues []readyIssue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		if strings.EqualFold(issue.IssueType, "epic") {
			continue
		}
		epicID := parentEpicID(issue.ID)
		if epicID == "" {
			continue
		}
		counts[epicID]++
	}
	return counts
}

func summarizeOpenCounts(issues []listIssue) map[string]int {
	counts := make(map[string]int)
	for _, issue := range issues {
		if strings.EqualFold(issue.IssueType, "epic") {
			continue
		}
		if strings.EqualFold(issue.Status, "closed") {
			continue
		}
		epicID := parentEpicID(issue.ID)
		if epicID == "" {
			continue
		}
		counts[epicID]++
	}
	return counts
}
