package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brandonharvey/automatic-octo-barnacle/tools/obi/internal/config"
)

const sampleConfig = `results_log = "~/obi.log"
confirm_before_run = true

[codex]
model = "gpt"
sandbox = "workspace-write"
approval = "on-request"

[epic.foo]
name = "Foo Work"
id = "automatic-octo-barnacle-foo"
prompt = "Foo prompt"
alias = "foo-work"

[epic.bar]
name = "Bar Work"
id = "automatic-octo-barnacle-bar"
prompt = "Bar prompt"

["issues outside epics"]
prompt = "Loose prompt"
`

func writeConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "obi.toml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadRoundTrip(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := cfg.Epics["bar"]; !ok {
		t.Fatalf("expected epic bar in map")
	}
	if cfg.Issues == nil || cfg.Issues.Prompt != "Loose prompt" {
		t.Fatalf("expected issues outside epics prompt loaded")
	}
}

func TestResolvePathPrecedence(t *testing.T) {
	t.Setenv("OBI_CONFIG", "/tmp/env-config")

	path, err := config.ResolvePath("/tmp/flag-config")
	if err != nil {
		t.Fatalf("resolve flag path: %v", err)
	}
	if path != "/tmp/flag-config" {
		t.Fatalf("expected flag path, got %s", path)
	}

	path, err = config.ResolvePath("")
	if err != nil {
		t.Fatalf("resolve env path: %v", err)
	}
	if path != "/tmp/env-config" {
		t.Fatalf("expected env path, got %s", path)
	}
}

func TestResolvePathSearchesUpwards(t *testing.T) {
	t.Setenv("OBI_CONFIG", "")

	root := t.TempDir()
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	configPath := filepath.Join(root, "obi.toml")
	if err := os.WriteFile(configPath, []byte(sampleConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWD)

	if err := os.Chdir(child); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	path, err := config.ResolvePath("")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlink actual: %v", err)
	}
	want, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		t.Fatalf("eval symlink want: %v", err)
	}
	if got != want {
		t.Fatalf("expected %s got %s", want, got)
	}
}

func TestResolvePathNotFound(t *testing.T) {
	t.Setenv("OBI_CONFIG", "")
	dir := t.TempDir()

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(origWD)

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if _, err := config.ResolvePath(""); err == nil {
		t.Fatalf("expected error when config missing")
	}
}

func TestResultsLogPathDefault(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.ResultsLog = ""
	val, err := cfg.ResultsLogPath()
	if err != nil {
		t.Fatalf("results log: %v", err)
	}
	if filepath.Base(val) != "results.log" {
		t.Fatalf("expected results.log basename, got %s", val)
	}
}

func TestEpicLookupByKey(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	name, tgt, err := cfg.Epic("foo")
	if err != nil {
		t.Fatalf("epic: %v", err)
	}
	if name != "foo" || tgt.Name != "Foo Work" {
		t.Fatalf("unexpected epic: %s -> %+v", name, tgt)
	}
}

func TestConfirmBeforeRunValue(t *testing.T) {
	var cfg config.Config
	if !cfg.ConfirmBeforeRunValue() {
		t.Fatalf("expected default confirm to be true")
	}
	cfg.ConfirmBeforeRun = boolPtr(false)
	if cfg.ConfirmBeforeRunValue() {
		t.Fatalf("expected override false")
	}
}

func boolPtr(val bool) *bool {
	b := val
	return &b
}

func TestEpicLookupRequiresName(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, _, err := cfg.Epic(""); err == nil {
		t.Fatalf("expected error for empty epic name")
	}
}

func TestEpicLookupByAlias(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	name, epic, err := cfg.Epic("foo-work")
	if err != nil {
		t.Fatalf("epic alias lookup: %v", err)
	}
	if name != "foo" || epic.ID != "automatic-octo-barnacle-foo" {
		t.Fatalf("unexpected alias lookup result: %s -> %+v", name, epic)
	}
}

func TestEpicLookupByID(t *testing.T) {
	path := writeConfig(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	name, epic, err := cfg.Epic("automatic-octo-barnacle-bar")
	if err != nil {
		t.Fatalf("epic id lookup: %v", err)
	}
	if name != "bar" || epic.Name != "Bar Work" {
		t.Fatalf("unexpected id lookup: %s -> %+v", name, epic)
	}
}
