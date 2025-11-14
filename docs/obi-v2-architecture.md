# Obi v2 Interactive Orchestration Architecture

## Context
Obi v1 shells out to `codex exec` once per bead, waits for stdout to contain the footer, and appends a single-line log entry. V2 must support fully interactive Codex sessions (streaming UI, PTY semantics, signal forwarding) so operators can run an entire epic unattended while still preserving bead-level accountability, metadata capture, and post-run summarization.

## Goals
- Provide a deterministic, restartable pipeline that can execute every bead in an epic by repeatedly launching scoped Codex sessions with repo-specific prompts.
- Move from a one-shot `codex exec` to a robust interactive runner (PTY aware, cross-platform) without losing transcript capture or footer parsing.
- Persist rich session metadata (footer values, multi-line commit body, timestamps, Codex binary/model, flags, log paths) in a schema-versioned store that downstream tools can query.
- Offer an "omnibus" summarizer run once the epic has no ready beads so humans receive a single commit narrative derived from the collected per-bead messages.
- Enforce AGENTS.md workflows (bd ready/claim/close, testing, footer) through prompt layering so every Codex session receives the same guardrails.
- Survive cancellations (Ctrl-C), Codex crashes, or orphaned PTYs without wedging the operator's terminal or corrupting logs.

Non-goals:
- Building a new issue tracker (we rely on bd).
- Embedding Codex itself; obi just orchestrates CLI invocations.
- Persisting full transcripts forever (only structured metadata + optional tee output).

## System Overview
```
obi CLI -> Config loader -> Session planner -> Interactive runner -> Footer/metadata parser -> Session log store
                                                             |                                       |
                                                             |                               Omnibus summarizer trigger
                                                        Transcript tee
```
Each invocation of `obi go <alias>` flows through these layers:
1. Resolve configuration + prompts (`internal/config`).
2. Build a `session.Plan` (epic metadata, prompts, codex flags).
3. Preview the prompt and confirm with the operator (existing behavior).
4. Launch the interactive runner, injecting the assembled prompt and streaming Codex output through a PTY.
5. Parse the Codex footer from the captured stream, redact secrets, and append a structured session record.
6. When beads for the epic are exhausted, spin up the omnibus summarizer session to produce a final commit synopsis.

## Components
### 1. Planner & Prompting
- Reuse `sessionPlan` but enrich it with `RunID`, `PromptHash`, and `Env` metadata needed by the runner/logs.
- Continue layering `base_prompt` + per-epic prompt + auto-generated footer instructions.
- Add prompt decorators that remind agents to close their bead (`bd update … --status completed`) so V2 enforces epic completion from the prompt itself.

### 2. Interactive PTY Runner (`internal/interactive`)
Responsibilities:
- Create/destroy a PTY on macOS/Linux/WSL; fall back with actionable errors when `/dev/ptmx` is unavailable.
- Inject the prompt via stdin before showing the Codex UI; allow optional "initial prompt" overrides for debugging.
- Stream stdout/stderr to the user's terminal while teeing into in-memory buffers for footer detection.
- Redact secrets before writing tee outputs when operators opt in to transcript files.
- Return a `RunnerResult` struct containing exit status, footer block (if found), truncated transcript, and timing data.
Testing:
- Provide fake PTY shims so unit tests can simulate streaming + footer parsing without a real terminal.
- Cover error cases (missing PTY, redaction failure, footer timeout, Codex non-zero exit).

### 2b. Fenced report parser & sentinel
- Streaming parser (`internal/fenced`) that incrementally scans PTY output, tolerates noise, enforces the ` ```obi:<SESSION_UUID>` fence, and surfaces structured results (`status`, `commit_msg`, `details`, `escalation`).
- Sentinel helper (`internal/interactive/sentinel.go`) wired to session events so the TUI can stop a run immediately on malformed fences or mismatched UUIDs instead of waiting for process exit.
- Legacy footer parsing stays in place for compatibility, but Obi now treats the fenced block as the source of truth and cross-checks the footer for drift.

### 2c. TUI shell scaffolding (`internal/tui`)
- Raw-mode terminal controller that hides/restores the cursor, clears the screen, and guarantees the terminal state is restored even if the session aborts.
- Header/status line that reflects the latest `SessionEvent` state transitions, plus a footer with the hotkey legend Codex operators rely on (`Ctrl+C` soft-stop, second `Ctrl+C` abort, etc.).
- Scrollable log pane that buffers the most recent PTY output, redrew on every incoming `log_chunk` event, and exposes hooks (`Shell.Run`/`Shell.HandleEvent`) so future panes/widgets can subscribe to the session runner stream.
- Obi launches this TUI by default for `obi go`; pass `--no-tui` when you need legacy raw streams for piping/log scraping.

### 3. Session Lifecycle Manager
- Wrap the runner inside a controller that handles signal forwarding (SIGINT/SIGTERM) and process reaping.
- Guarantee terminal state reset via `defer`/`tcell` style helpers even on panic.
- Surface cancellation status back to `obi go` so it can mark the bead as `needs_help` with an explanatory `ESCALATION` reason.
- Provide hooks for future commands (e.g., `obi doctor`) to inspect stuck sessions.

### 4. Metadata Store & Schema
- Replace the line-oriented `results.log` with JSONL stored under the path declared in `obi.toml` (default `$XDG_CONFIG_HOME/obi/results.log`).
- Session record fields: schema_version, run_id, repo_root, epic_key/id/name, bead_id (when parseable from Codex footer), codex binary/model/sandbox/approval flags, status, commit_summary (single line), commit_body (multi-line), timestamps (queued, started, finished), transcript_path (if tee enabled), config hash, redaction status, and optional escalation reason.
- Provide migration helpers that upgrade older logs in-place or emit warnings when auto-upgrade is unsafe.
- Lock down file permissions (0o600) because commit bodies can contain sensitive info.

### 5. Omnibus Commit Summarizer
- After each successful bead, check bd for remaining ready work under the same epic filters. When none exist, enqueue a summarizer session that:
  1. Reads all records for the epic from the metadata store, newest first.
  2. Chunks commit bodies into prompt-sized windows, feeding them to a dedicated summarizer prompt.
  3. Emits the resulting summary to stdout and appends a final `summary` record to the log.
- Provide CLI flags to skip/force summarization for debugging.

### 6. CLI Entry Points
- `obi init/refresh`: unchanged aside from writing the new schema version + prompts.
- `obi go`: adds `--no-interactive` (debug), `--tee path`, and `--resume run_id` toggles so operators can recover from crashes.
- `obi list`: continue showing loose issues plus interactive-specific warnings (e.g., config missing `codex.binary`).

### 7. Testing & Regression Coverage
- Unit tests per package (`config`, `planner`, `interactive`, `lifecycle`, `metadata`, `summary`).
- Golden tests for prompt rendering and log serialization to catch regressions.
- End-to-end smoke test that shells out to a stub Codex binary producing canned footer output; ensures `obi go --execute` wires the entire pipeline.

## Sequencing & Dependencies
1. **Architecture Brief (this bead)** – locks target state so downstream beads have shared terminology.
2. **PTY Codex runner** – builds the `internal/interactive` package + fake PTY tests.
3. **Wire `obi go` to the runner** – replace `executeCodex` with lifecycle controller, ensure prompts + transcripts still preview.
4. **Prompt enforcement + lifecycle** – tighten the prompt builder and handle cancellations.
5. **Metadata persistence** – land JSONL schema + migrations, update `obi.toml` + docs.
6. **Omnibus summarizer** – consume metadata store, add CLI surfacing.
7. **Documentation & regression suite** – README/CLI help + automated coverage.

Delivering the beads in this order keeps high-risk PTY and lifecycle work isolated before schema/log churn and summarizer layering. The architecture above highlights the contracts each bead must honor so we can evolve Obi without surprising downstream automation.
