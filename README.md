# obi: a task runner for beads

obi runs a series of Codex sessions to complete a whole epic's worth of beads. 

1. Have codex use `bd` to thoroughly plan your feature into an epic
2. Run obi against that epic
3. Each bead is completed start to finish by a fresh codex session
4. obi continues the process until the epic is done, and outputs detailed commit messages for you

You configure obi with a human-friendly obi.toml in the repo root.

# Quick start

```
$ obi init
$ obi list
$ obi --version
$ obi go {epic alias or ID}             # launches the TUI after preview/confirmation
$ obi go --no-tui {alias}               # legacy raw-stream mode (useful for piping)
```

## Configuration file

Run `obi init` at the repo root; it discovers open bead epics, generates aliases (one word or hyphenated) via Codex, and writes `obi.toml` while narrating each step so you know what it’s doing—the command always ends with `Created/Updated <path>` on success. Re-run `obi refresh` whenever epics change—it is safe to run repeatedly (adds new open epics, removes fully closed ones, keeps existing prompts/aliases). Obi looks for `obi.toml` in the current directory and walks up parent directories until it finds one. Override this discovery with `OBI_CONFIG=/path/obi.toml` or `obi go --config /path/obi.toml`.

Open the generated `obi.toml` to see:
- `results_log`: path where run summaries land.
- `confirm_before_run`: when `true` (default), `obi go` pauses after the preview and asks `[Y/n]` before launching Codex. Set it to `false` once you’re comfortable letting runs start immediately after the preview.
- `base_prompt`: shared text prepended to every Codex session.
- `["issues outside epics"]`: optional section that defines how `obi go` behaves when you don’t pass an alias. Obi ships with a default prompt that focuses on standalone issues; edit it to fit your repo or delete the section if you prefer to always specify an epic.
- `[epic.<key>]` sections: each bead epic with `name`, `id`, `alias`, and a per-epic `prompt`. Obi always sends `base_prompt` first and the epic prompt immediately after, so the two prompts are additive rather than overrides.
- Optional `[codex]` overrides (default comments mention GPT‑5 models only—e.g., `gpt-5-codex-medium`). Remove the `binary/model` lines entirely to let Obi use your CLI defaults; otherwise set them explicitly.

**Prompt layering:** Obi first inserts `base_prompt`, then appends the epic’s `prompt`. They’re additive—editing an epic prompt never replaces the base prompt. Use the base prompt for global reminders (e.g., “always finish with STATUS/COMMIT_MSG”), and each epic prompt for epic-specific instructions. Obi also auto-appends an “epic completion contract” block that reminds Codex to claim a bead via `bd update <id> --status in_progress --json`, close it with `bd close`/`bd update --status completed`, and only emit `STATUS: success` once the bead is closed (loose issues get a similar contract).

### Field reference

### Environment overrides & refresh

1. `obi go --config path` forces a specific file regardless of location.
2. `OBI_CONFIG=/path/obi.toml` overrides discovery for all runs in that shell.
3. Otherwise Obi searches for `obi.toml` starting at `$PWD` and walking up to the filesystem root; if none is found it errors.
4. Run `obi refresh` any time your bead epics change. It is idempotent: new open epics are added (with Codex-generated aliases), closed epics are removed, and existing entries are preserved.

Use `obi list` to view the “issues outside epics” block plus every configured epic in a four-column table (Alias / Ready/Total / Name / Epic ID – always rightmost) keyed to your repo root. The command also reports how many ready beads live outside any epic so you know when `obi go` without arguments will find work.

### Interactive lifecycle & cancellation

Obi always launches Codex inside a PTY and owns the lifecycle:
- First `Ctrl+C` sends a soft-stop marker through the PTY so Codex can gracefully wrap up and emit the fenced report.
- A second `Ctrl+C` upgrades to a SIGINT and immediately aborts the Codex subprocess.
- `SIGTERM`/`SIGHUP` (e.g., CI cancellation) abort Codex right away; Obi still waits for the process to exit so PTYs are never orphaned.
- Regardless of exit path (success, crash, or manual abort), Obi reaps the Codex process and restores terminal state before returning control to the operator.
- The header at the top of the TUI now shows the live epic alias/id, bead, run state (running/stopping/exited), elapsed time, and token usage placeholders so humans can tell at a glance what the current session is doing. Press `h` to enter hint mode, type your note, and Obi wraps it in a `[[OBI:HUMAN_HINT]]` block sent into Codex; press `s` to send the one-per-run `[[OBI:SOFT_STOP]]` marker. Every operator intervention is logged to both the visible stream and the ledger/transcript metadata for auditing.

These guardrails are shared by the non-TUI flow and the interactive shell so other commands can depend on consistent cancellation semantics.

Need plain stdout (e.g., for CI scraping or piping)? Pass `--no-tui` to `obi go` and Obi will stream Codex output directly without entering raw terminal mode.

## Testing with the fake Codex harness

The `internal/fakecodex` package provides a deterministic stand-in for the real Codex CLI so tests can cover the full PTY runner + ledger pipeline. `go test ./internal/app` builds the harness automatically and points `executeSession` at the resulting binary—a pair of end-to-end tests exercise both the success and `needs_help` flows by setting `FAKE_CODEX_SCENARIO`. You can also run the binary manually from the repo root:

```bash
go build -o bin/fakecodex ./internal/fakecodex/cmd/fakecodex
FAKE_CODEX_SCENARIO=long_logs bin/fakecodex < prompt.txt
```

Set `OBI_PIPE_LAUNCHER=1` when running these tests outside of a real TTY; this flips the session runner into a pipe-based launcher so the fake Codex binary can execute inside CI sandboxes. The built-in scenarios (`success`, `needs_help`, `malformed`, `long_logs`) emit realistic stdout/stderr streams, fenced reports, and legacy footers—perfect for future CLI integration smoke tests.

Generate zsh completions with:

```bash
obi completion zsh > ~/.zsh/completions/_obi
```

Reload your shell (or source the file) and `obi go <alias-or-epic-id>` will tab-complete using both the configured aliases and raw epic IDs.

## Codex session markers

Every Codex session launched by Obi must finish with a short footer Obi can parse deterministically:

```
STATUS: success|needs_help
COMMIT_MSG: <single-line imperative summary>
ESCALATION: <short reason>   # only when status=needs_help
```

Rules:
- Exactly one `STATUS:` line is required. Use `success` when the bead is complete, `needs_help` otherwise.
- `COMMIT_MSG:` must always follow, even when the bead failed (describe the partial work or leave a note for humans).
- Include an `ESCALATION:` line whenever `STATUS: needs_help` to tell the operator what approval or action is required. Omit it on success.
- No additional output should come after these markers; Obi treats them as end-of-run sentinels.

Add the footer verbatim to your prompt template so Codex agents know how to report completion. Later beads in this epic will teach Obi how to parse and act on these markers.

Obi currently appends the following helper text to the prompt body so agents remember the expected footer:

```
When you are done, respond with the lines:
STATUS: success|needs_help
COMMIT_MSG:
<detailed multi-line summary of everything you changed>
ESCALATION: <reason>  # only if status=needs_help
```

## Workflow delegation

Obi does **not** select beads or restate AGENTS.md. The spawned Codex agent is responsible for:
- running `bd ready --json` to pick work within the provided epic/tool scope,
- updating bead status, creating follow-up issues, and running tests per AGENTS.md,
- emitting the footer markers above when finished.

Obi merely feeds the agent your epic-specific (or loose-issue) prompt plus metadata so the agent can discover the rest from the repo itself.

### Status

At this stage the CLI validates the configuration and alias lookup:

```bash
obi go
# uses the "issues outside epics" section; prints session metadata + prompt preview

obi go foo-alias
# targets the specific epic alias

obi go foo-alias --execute
# launches `codex exec ...` with the assembled prompt

obi go foo-alias --execute --out ./logs/foo.txt
# streams to stdout as usual and tees Codex stdout/stderr to ./logs/foo.txt

obi go foo-alias --resume
# loads completed beads for the epic from results.log, skips them, and halts if a prior run emitted STATUS: needs_help
```
When you target a specific epic, `obi go` now loops automatically: after each successful Codex session it re-checks `bd ready` and, if more beads exist, immediately launches the next run (skipping previously completed beads from the current session and any you passed via `--resume`). The loop stops as soon as no ready beads remain, or immediately when Codex reports `STATUS: needs_help` or fails to emit a report. The confirmation prompt (when enabled) only appears before the first session; disable it in `obi.toml` once you want unattended runs.

When the loop finishes (no ready beads remain), Obi automatically kicks off an **omnibus summary** run. That session reads the multi-line commit bodies stored in `results_log`, chunks them according to your config, and asks Codex to write one cohesive narrative covering the entire epic. The summary is printed to the terminal and logged as another ledger entry so humans can review it later. Control the behavior via the `[summary]` block in `obi.toml`—tweak the prompt, `max_commits`, or `chunk_size` (set `max_commits = 0` to disable the summarizer altogether).
If the “issues outside epics” section is missing, `obi go` will tell you so and list available epic aliases—run `obi go <alias>` in that case.

When `--execute` is used, Obi pipes codex output through, parses the footer, and appends a JSON line to `results_log` (default: `$XDG_CONFIG_HOME/obi/results.log`). Each entry now captures the run/session IDs, repo root, epic metadata, bead ID, Codex binary/model/sandbox/approval flags, prompt hash, config digest, timestamps, transcripts, and whether any redactions were applied before persisting. The log file (and transcripts) are written with `0600` permissions and Obi automatically upgrades legacy v1 logs the first time you run the new CLI—no manual migration script required. Use this file as your running summary of what Codex accomplished or as input for the omnibus summarizer.

If Codex exits non-zero or fails to emit the footer, Obi stops immediately with an error. Likewise, when the footer reports `STATUS: needs_help`, Obi surfaces the `ESCALATION` reason, records the log entry, and exits with a non-zero status so you know manual intervention is required before the next run.
```

Future beads will add bd querying, prompt assembly, Codex execution, logging, and escalation handling per the epic plan.

## Interactive runs & transcripts

`obi go` now launches Codex inside a PTY so the interactive UI (color, prompts, footer) streams exactly as if you ran `codex` manually. Pass `--out path/to/log.txt` to tee the redacted transcript to disk; Obi creates the parent directories automatically and overwrites the file on each run. To scrub sensitive tokens from the transcript **and** the stored commit summaries/details, set `OBI_REDACT="secret1,secret2"` (commas, semicolons, or newlines work as separators) before starting the run. The terminal stream stays untouched so you can still copy/paste while the stored transcript/log are safe to share.

# Installation

The most common operations are building `obi` from source and installing it globally so `obi` is on your `PATH`.

From the repo root:

```bash
# build local binary into ./bin/obi
go build -o bin/obi ./cmd/obi

# install globally into $GOBIN (typically on PATH)
go install ./cmd/obi
```

To install from elsewhere with an explicit module path and version:

```bash
go install github.com/brandonharvey/obi/cmd/obi@latest
```

After installing, run `obi init` (once per repo) to generate `obi.toml`, tweak the prompts/aliases as needed, then run:

```bash
obi go foo-alias          # previews, then prompts [Y/n] before running Codex
obi go epic-1234-foo      # use raw epic IDs interchangeably with aliases
```

Use `OBI_CONFIG` or `--config` to point to alternate configs; Obi itself is repo-agnostic aside from relying on AGENTS.md and `bd` in the working tree.
- `[codex]` overrides (optional): only reference GPT‑5 class models here (e.g., `gpt-5-codex-medium`). Obi no longer documents legacy models like o3/o4.

Example `[summary]` configuration (generated by `obi init`):

```toml
[summary]
prompt = """
You will receive commit summaries and detailed notes for every bead completed in this epic. Your job is to write one cohesive, multi-line commit message (subject line + detailed body) that captures the entire story so humans can understand what shipped.

Guidelines:
- Highlight major functional threads (features, bugs, migrations) rather than restating every bead verbatim.
- Call out tests, docs, and follow-ups when they matter.
- If information appears truncated or missing, acknowledge the limitation rather than inventing details.
"""
max_commits = 20   # show at most the N most recent commits to avoid blowing past context
chunk_size = 5     # how many commits to place in each chunk block inside the summary prompt
```
