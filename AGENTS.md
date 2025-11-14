## Issue Tracking with bd (beads)

**IMPORTANT**: This project uses **bd (beads)** for ALL planning and issue tracking. Do NOT use other tracking methods.

### Why bd?

- Dependency-aware: Track blockers and relationships between issues
- Git-friendly: Auto-syncs to JSONL for version control
- Agent-optimized: JSON output, ready work detection, discovered-from links
- Prevents duplicate tracking systems and confusion

### Quick Start

**Check for ready work:**
```bash
bd ready --json
```

**Create new issues:**
```bash
bd create "Issue title" -t bug|feature|task -p 0-4 --json
bd create "Issue title" -p 1 --deps discovered-from:bd-123 --json
```

**Claim and update:**
```bash
bd update bd-42 --status in_progress --json
bd update bd-42 --priority 1 --json
```

**Complete work:**
```bash
bd close bd-42 --reason "Completed" --json
```

### Issue Types

- `bug` - Something broken
- `feature` - New functionality
- `task` - Work item (tests, docs, refactoring)
- `epic` - Large feature with subtasks
- `chore` - Maintenance (dependencies, tooling)

### Priorities

- `0` - Critical (security, data loss, broken builds)
- `1` - High (major features, important bugs)
- `2` - Medium (default, nice-to-have)
- `3` - Low (polish, optimization)
- `4` - Backlog (future ideas)

### Workflow for AI Agents

Workflow expectation:
1. Run `bd ready --json` to find ready work
2. Claim your task with `bd update <id> --status in_progress --json` before coding.
3. Make sure tests pass before your implementation. If they don't, stop and report back to me.
4. If tests are green, implement
5. Did you find issues along the way? Create linked issue:
   - `bd create "Found bug" -p 1 --deps discovered-from:<parent-id>`
6. Make sure tests are still green after your implementation
7. Now stop and think again about testing, using creativity, common sense, and lateral thinking. Have we really tested all the important things? If you can think of new tests, add them, and ensure they pass.
8. Update/close beads when complete (include reason)

**Benefits:**
- ✅ Clean repository root
- ✅ Clear separation between ephemeral and permanent documentation
- ✅ Easy to exclude from version control if desired
- ✅ Preserves planning history for archeological research
- ✅ Reduces noise when browsing the project

### Important Rules

- ✅ Use bd for ALL task tracking
- ✅ Always use `--json` flag for programmatic use
- ✅ Link discovered work with `discovered-from` dependencies
- ❌ Do NOT create markdown TODO lists
- ❌ Do NOT use external issue trackers
- ❌ Do NOT duplicate tracking systems
- ❌ Do NOT clutter repo root with planning documents


## Technical Approach -- highly important
- Impose very clean separation of concerns, so that we can make changes with confidence, knowing they didn't break something elsewhere
- Emphasize clarity in the code, rather than terseness & cleverness, because we want ease of maintenance (by LLMs) and ease of understanding by humans
- Choose pure functions when possible, because they are idempotent, testable, and explicit about input and output. Generally, prefer explicit input/output and avoid tacit, implicit, and hidden state. Env var "secrets" are an important exception to this rule, but other than those, I really prefer to avoid hidden inputs.
- Any side-issues you find along the way, file as beads and keep going on your main task.
- Always escalate mutative/destructive git operations and db operations to me for permission to run them.

## Project guidance
- At this stage of the project, **this is an MVP, and we are uncertain of the value of the solution yet**. So while we do want robustness and predictability as I have said, there IS such a thing as over-engineering here. Use your judgment as a senior engineer to help me find the balance point between keeping it simple (so we can go fast) and building it right.

## Other tool guidance
- I use Mac, zsh, homebrew
- I'll stage and commit myself, your use of git is read-only
- When you look at staged git changes, you MUST use the full "git status" command, NOT "git status -sb", otherwise you won't see the same git state that I see, and it will cause confusion between us. This is important.
- When done with a bead, write a detailed git commit message covering ALL the work that you have done in the session so far, cumulatively. So if I ask you to work on a second bead, you would then write a commit message covering both beads.
   - The commit message first line follows conventional commits style.
   - The rest of the commit message is detailed enough to help humans and AI agents thoroughly understand project history later on. 

### ast-grep vs ripgrep

**Use `ast-grep` when structure matters.** It parses code and matches AST nodes, so results ignore comments/strings, understand syntax, and can **safely rewrite** code.

* Refactors/codemods: rename APIs, change import forms, rewrite call sites or variable kinds.
* Policy checks: enforce patterns across a repo (`scan` with rules + `test`).
* Editor/automation: LSP mode; `--json` output for tooling.

**Use `ripgrep` when text is enough.** It’s the fastest way to grep literals/regex across files.

* Recon: find strings, TODOs, log lines, config values, or non‑code assets.
* Pre-filter: narrow candidate files before a precise pass.

**Rule of thumb**

* Need correctness over speed, or you’ll **apply changes** → start with `ast-grep`.
* Need raw speed or you’re just **hunting text** → start with `rg`.
* Often combine: `rg` to shortlist files, then `ast-grep` to match/modify with precision.

**Snippets**

Find structured code (ignores comments/strings):

```bash
ast-grep run -l TypeScript -p 'import $X from "$P"'
```

Codemod (only real `var` declarations become `let`):

```bash
ast-grep run -l JavaScript -p 'var $A = $B' -r 'let $A = $B' -U
```

Quick textual hunt:

```bash
rg -n 'console\.log\(' -t js
```

Combine speed + precision:

```bash
rg -l -t ts 'useQuery\(' | xargs ast-grep run -l TypeScript -p 'useQuery($A)' -r 'useSuspenseQuery($A)' -U
```
**Mental model**

* Unit of match: `ast-grep` = node; `rg` = line.
* False positives: `ast-grep` low; `rg` depends on your regex.
* Rewrites: `ast-grep` first-class; `rg` requires ad‑hoc sed/awk and risks collateral edits.
