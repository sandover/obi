package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/codexexec"
	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

type bdEpic struct {
	Epic struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	} `json:"epic"`
	EligibleForClose bool `json:"eligible_for_close"`
	TotalChildren    int  `json:"total_children"`
	ClosedChildren   int  `json:"closed_children"`
}

func runInit(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("obi init takes no arguments")
	}

	path, err := defaultConfigPath()
	if err != nil {
		return err
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr == nil {
		fmt.Printf("%s already exists; running 'obi refresh' instead.\n", path)
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	_, err = refreshAtPath(path, false)
	if err != nil {
		return err
	}
	action := "Created"
	if exists {
		action = "Updated"
	}
	fmt.Printf("%s %s\n", action, path)
	return nil
}

func defaultConfigPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "obi.toml"), nil
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

type refreshOptions struct {
	configPath string
	silent     bool
}

func runRefresh(args []string) error {
	opts, err := parseRefreshOptions(args)
	if err != nil {
		return err
	}

	path, err := determineRefreshPath(opts.configPath)
	if err != nil {
		return err
	}

	summary, err := refreshAtPath(path, opts.silent)
	if err != nil {
		return err
	}
	if !opts.silent {
		fmt.Printf("Done! %d epics → %s (kept %d, added %d, removed %d).\n",
			summary.total, filepath.Base(path), summary.kept, summary.added, summary.removed)
	}
	return nil
}

func parseRefreshOptions(args []string) (refreshOptions, error) {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts refreshOptions
	fs.StringVar(&opts.configPath, "config", "", "path to obi.toml (defaults to nearest or ./obi.toml)")
	fs.BoolVar(&opts.silent, "silent", false, "suppress summary output")

	if err := fs.Parse(args); err != nil {
		return refreshOptions{}, fmt.Errorf("parse flags: %w", err)
	}
	return opts, nil
}

func determineRefreshPath(flagPath string) (string, error) {
	if flagPath != "" {
		return expandPath(flagPath)
	}
	path, found, err := findExistingConfigUpwards()
	if err != nil {
		return "", err
	}
	if found {
		return path, nil
	}
	return defaultConfigPath()
}

func findExistingConfigUpwards() (string, bool, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false, err
	}
	dir := wd
	for {
		candidate := filepath.Join(dir, "obi.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join(wd, "obi.toml"), false, nil
}

func refreshAtPath(path string, silent bool) (refreshSummary, error) {
	logger := refreshLogger{enabled: !silent}
	logger.Printf("Scanning bead epics via `bd epic status --json`...\n")
	epics, err := listEpics()
	if err != nil {
		return refreshSummary{}, err
	}
	active := filterActiveEpics(epics)
	if len(active) == 0 {
		return refreshSummary{}, fmt.Errorf("no open epics available to refresh")
	}
	logger.Printf("Found %d open epics (from %d total).\n", len(active), len(epics))

	existingCfg, err := loadConfigIfExists(path)
	if err != nil {
		return refreshSummary{}, err
	}
	if existingCfg == nil {
		logger.Printf("No existing obi config; a new file will be created at %s\n", path)
	} else {
		logger.Printf("Loaded existing config at %s\n", path)
	}

	updatedCfg, summary, err := buildConfig(active, existingCfg, logger)
	if err != nil {
		return refreshSummary{}, err
	}
	logger.Printf("Writing config to %s...\n", filepath.Base(path))
	if err := writeConfigFile(path, updatedCfg); err != nil {
		return refreshSummary{}, err
	}

	return summary, nil
}

func listEpics() ([]bdEpic, error) {
	cmd := exec.Command("bd", "epic", "status", "--json")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("bd epic status: %w", err)
	}

	var epics []bdEpic
	if err := json.Unmarshal(out.Bytes(), &epics); err != nil {
		return nil, fmt.Errorf("parse bd output: %w", err)
	}

	return epics, nil
}

const defaultBasePrompt = `Your task is to select the most appropriate bead from the epic we've indicated, implement it, and when you've succeeded, end the session with a detailed multi-line commit message, suitable to help humans and AI agents thoroughly understand project history later on.

If you aren't familiar with beads, beads is not installed, or you can't find the indicated epic, stop now and report the error.

There may be other Codex instances working elsewhere in the repo—ignore their activity unless it causes conflicts; if it does, stop immediately and report the situation.`

const defaultIssuesPrompt = `Your task is to select the most appropriate bead not already covered by any epic, implement it, and when you've succeeded, end the session with a detailed multi-line commit message, suitable to help humans and AI agents thoroughly understand project history later on.

If you aren't familiar with beads or beads is not installed, stop now and report the error.

There may be other Codex instances working elsewhere in the repo—ignore their activity unless it causes conflicts; if it does, stop immediately and report the situation.`

type refreshSummary struct {
	total   int
	kept    int
	added   int
	removed int
}

type refreshLogger struct {
	enabled bool
}

func (l refreshLogger) Printf(format string, args ...interface{}) {
	if !l.enabled {
		return
	}
	fmt.Printf(format, args...)
}

func filterActiveEpics(epics []bdEpic) []bdEpic {
	var active []bdEpic
	for _, e := range epics {
		if strings.EqualFold(e.Epic.Status, "closed") {
			continue
		}
		active = append(active, e)
	}
	return active
}

func loadConfigIfExists(path string) (*config.Config, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func buildConfig(epics []bdEpic, existing *config.Config, logger refreshLogger) (*config.Config, refreshSummary, error) {
	newCfg := &config.Config{
		ResultsLog:       "./obi-results.log",
		BasePrompt:       defaultBasePrompt,
		Epics:            map[string]config.EpicConfig{},
		Codex:            config.CodexConfig{},
		ConfirmBeforeRun: boolPtr(true),
		Summary: config.SummaryConfig{
			Prompt:     config.DefaultSummaryPrompt,
			MaxCommits: config.DefaultSummaryMaxCommits,
			ChunkSize:  config.DefaultSummaryChunkSize,
		},
	}
	if existing != nil {
		newCfg.ResultsLog = fallbackString(existing.ResultsLog, "./obi-results.log")
		newCfg.BasePrompt = fallbackString(existing.BasePrompt, defaultBasePrompt)
		newCfg.Codex = existing.Codex
		if existing.ConfirmBeforeRun != nil {
			val := *existing.ConfirmBeforeRun
			newCfg.ConfirmBeforeRun = boolPtr(val)
		}
		if existing.Issues != nil {
			copy := *existing.Issues
			newCfg.Issues = &copy
		}
		newCfg.Summary = existing.Summary
		if strings.TrimSpace(newCfg.Summary.Prompt) == "" {
			newCfg.Summary.Prompt = config.DefaultSummaryPrompt
		}
		if newCfg.Summary.MaxCommits <= 0 {
			newCfg.Summary.MaxCommits = config.DefaultSummaryMaxCommits
		}
		if newCfg.Summary.ChunkSize <= 0 {
			newCfg.Summary.ChunkSize = config.DefaultSummaryChunkSize
		}
	} else {
		newCfg.Issues = &config.IssuesConfig{Prompt: defaultIssuesPrompt}
	}

	existingEpics := map[string]config.EpicConfig{}
	if existing != nil {
		for key, epic := range existing.Epics {
			existingEpics[key] = epic
		}
	}

	usedAliases := map[string]struct{}{}
	for _, epic := range existingEpics {
		if alias := strings.ToLower(strings.TrimSpace(epic.Alias)); alias != "" {
			usedAliases[alias] = struct{}{}
		}
	}

	summary := refreshSummary{}

	aliasRequests := map[string]aliasRequest{}
	for _, e := range epics {
		key := sanitizeKey(e.Epic.ID)
		if epicCfg, ok := existingEpics[key]; ok {
			if strings.TrimSpace(epicCfg.Alias) == "" {
				aliasRequests[key] = aliasRequest{Key: key, Title: e.Epic.Title, Description: e.Epic.Description}
			}
			continue
		}
		aliasRequests[key] = aliasRequest{Key: key, Title: e.Epic.Title, Description: e.Epic.Description}
	}

	if len(aliasRequests) > 0 {
		logger.Printf("Requesting aliases for %d epic(s)...\n", len(aliasRequests))
	} else {
		logger.Printf("All epics already have aliases; skipping Codex request.\n")
	}
	aliasSuggestions, err := generateAliasesBatch(newCfg.Codex, aliasRequests)
	if err != nil {
		return nil, summary, err
	}

	for _, e := range epics {
		key := sanitizeKey(e.Epic.ID)
		summary.total++

		if epicCfg, ok := existingEpics[key]; ok {
			if strings.TrimSpace(epicCfg.Alias) == "" {
				alias := finalizeAlias(key, e, aliasSuggestions, usedAliases)
				epicCfg.Alias = alias
			}
			newCfg.Epics[key] = epicCfg
			summary.kept++
			continue
		}

		alias := finalizeAlias(key, e, aliasSuggestions, usedAliases)

		newCfg.Epics[key] = config.EpicConfig{
			Name:   e.Epic.Title,
			ID:     e.Epic.ID,
			Prompt: "",
			Alias:  alias,
		}
		summary.added++
	}

	if existing != nil {
		for key := range existingEpics {
			if _, ok := newCfg.Epics[key]; !ok {
				summary.removed++
			}
		}
	}

	if len(newCfg.Epics) == 0 {
		return nil, summary, fmt.Errorf("no epics to write after refresh")
	}

	return newCfg, summary, nil
}

func fallbackString(val, def string) string {
	if strings.TrimSpace(val) == "" {
		return def
	}
	return val
}

type aliasRequest struct {
	Key         string
	Title       string
	Description string
}

func finalizeAlias(key string, e bdEpic, suggestions map[string]string, used map[string]struct{}) string {
	raw := suggestions[key]
	alias := enforceAliasCharset(strings.ToLower(strings.TrimSpace(raw)))
	if alias == "" {
		alias = fallbackAlias(e.Epic.Title)
	}
	return ensureUniqueAlias(alias, used)
}

func writeConfigFile(path string, cfg *config.Config) error {
	var sb strings.Builder
	sb.WriteString("# Obi discovers this file from your current directory upward.\n")
	sb.WriteString("# base_prompt text is always prepended first; each epic prompt is appended immediately after, so they combine.\n")
	sb.WriteString("# Each epic exposes a single alias used with `obi go <alias>`.\n\n")

	sb.WriteString(fmt.Sprintf("results_log = %q\n", cfg.ResultsLog))
	sb.WriteString(fmt.Sprintf("confirm_before_run = %t\n", cfg.ConfirmBeforeRunValue()))
	sb.WriteString(fmt.Sprintf("base_prompt = \"\"\"%s\"\"\"\n\n", escapeTripleQuotes(cfg.BasePrompt)))

	if cfg.Issues != nil {
		sb.WriteString("[\"issues outside epics\"]\n")
		if strings.TrimSpace(cfg.Issues.Prompt) == "" {
			sb.WriteString("prompt = \"\"\"\"\"\"\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("prompt = \"\"\"%s\"\"\"\n\n", escapeTripleQuotes(cfg.Issues.Prompt)))
		}
	} else {
		sb.WriteString("# Add an \"issues outside epics\" section to control `obi go` default behavior.\n\n")
	}

	if codexProvided(cfg.Codex) {
		sb.WriteString("[codex]\n")
		writeNonEmpty := func(key, value string) {
			if value != "" {
				sb.WriteString(fmt.Sprintf("%s = %q\n", key, value))
			}
		}
		writeNonEmpty("binary", cfg.Codex.Binary)
		writeNonEmpty("model", cfg.Codex.Model)
		writeNonEmpty("sandbox", cfg.Codex.Sandbox)
		writeNonEmpty("approval", cfg.Codex.Approval)
		if len(cfg.Codex.ExtraArgs) > 0 {
			sb.WriteString(fmt.Sprintf("extra_args = [%s]\n", formatStringSlice(cfg.Codex.ExtraArgs)))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("# Uncomment to override Codex defaults for this repo (use GPT-5 class models only).\n")
		sb.WriteString("# [codex]\n")
		sb.WriteString("# model = \"gpt-5-codex-medium\"\n")
		sb.WriteString("# sandbox = \"workspace-write\"\n")
		sb.WriteString("# approval = \"on-request\"\n\n")
	}

	summaryCfg := cfg.SummaryConfigValue()
	sb.WriteString("[summary]\n")
	sb.WriteString(fmt.Sprintf("prompt = \"\"\"%s\"\"\"\n", escapeTripleQuotes(summaryCfg.Prompt)))
	sb.WriteString(fmt.Sprintf("max_commits = %d\n", summaryCfg.MaxCommits))
	sb.WriteString(fmt.Sprintf("chunk_size = %d\n\n", summaryCfg.ChunkSize))

	keys := make([]string, 0, len(cfg.Epics))
	for key := range cfg.Epics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		e := cfg.Epics[key]
		sb.WriteString(fmt.Sprintf("[epic.%s]\n", key))
		if strings.TrimSpace(e.Alias) != "" {
			sb.WriteString(fmt.Sprintf("alias = %q\n", e.Alias))
		} else {
			sb.WriteString(fmt.Sprintf("alias = %q\n", key))
		}
		sb.WriteString(fmt.Sprintf("name = %q\n", e.Name))
		sb.WriteString(fmt.Sprintf("prompt = %q\n", normalizeSingleLine(e.Prompt)))
		sb.WriteString(fmt.Sprintf("id = %q\n", e.ID))
		if e.Tool != "" {
			sb.WriteString(fmt.Sprintf("tool = %q\n", e.Tool))
		}
		sb.WriteString("\n")
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func formatStringSlice(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return strings.Join(quoted, ", ")
}

func escapeTripleQuotes(s string) string {
	return strings.ReplaceAll(s, "\"\"\"", "\\\"\\\"\\\"")
}

func normalizeSingleLine(s string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func codexProvided(c config.CodexConfig) bool {
	return c.Binary != "" || c.Model != "" || c.Sandbox != "" || c.Approval != "" || len(c.ExtraArgs) > 0
}

func fallbackAlias(title string) string {
	title = strings.ToLower(title)
	fields := strings.FieldsFunc(title, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	if len(fields) == 0 {
		return "epic"
	}
	alias := fields[0]
	if len(fields) > 1 {
		alias = alias + "-" + fields[1]
	}
	return alias
}

func enforceAliasCharset(alias string) string {
	var b strings.Builder
	for _, r := range alias {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func ensureUniqueAlias(alias string, used map[string]struct{}) string {
	base := alias
	i := 2
	for {
		if _, exists := used[alias]; !exists && alias != "" {
			break
		}
		alias = fmt.Sprintf("%s-%d", base, i)
		i++
	}
	used[alias] = struct{}{}
	return alias
}

func boolPtr(val bool) *bool {
	b := val
	return &b
}

func generateAliasesBatch(codexCfg config.CodexConfig, requests map[string]aliasRequest) (map[string]string, error) {
	if len(requests) == 0 {
		return map[string]string{}, nil
	}

	keys := make([]string, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("Assign a short CLI alias to each epic. Respond with a JSON object mapping the provided key to its alias. Example: {\"foo\":\"orchard\"}.\n")
	sb.WriteString("Rules: aliases must be lowercase, one word or two words joined by a hyphen, contain only a-z, 0-9, or a single hyphen, and be <= 20 characters.\n")
	sb.WriteString("Epics:\n")
	for _, key := range keys {
		req := requests[key]
		sb.WriteString(fmt.Sprintf("- key: %s\n", req.Key))
		sb.WriteString(fmt.Sprintf("  title: %s\n", req.Title))
		sb.WriteString(fmt.Sprintf("  description: %s\n", truncate(req.Description, 400)))
	}
	sb.WriteString("\nReturn only the JSON object.\n")

	inv, err := codexexec.Build(codexCfg, sb.String())
	if err != nil {
		return nil, err
	}
	output, err := runCodexCapture(inv)
	if err != nil {
		return nil, err
	}

	jsonText, err := extractJSONObject(output)
	if err != nil {
		return nil, fmt.Errorf("alias batch output invalid: %w", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return nil, fmt.Errorf("alias batch parse: %w (json=%s)", err, jsonText)
	}
	return parsed, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func runCodexCapture(inv codexexec.Invocation) (string, error) {
	cmd := exec.Command(inv.Binary, inv.Args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("codex exec failed: %v\n%s", err, stderr.String())
	}
	return stdout.String(), nil
}

func extractJSONObject(output string) (string, error) {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end <= start {
		return "", fmt.Errorf("no JSON object found")
	}
	return output[start : end+1], nil
}

func sanitizeKey(id string) string {
	return strings.ReplaceAll(id, "-", "_")
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, "\"", "\\\"")
}
