# wt User Guide

This guide describes how to install `wt`, prepare a repository for worktrees, and use the everyday commands that keep projects moving.

## Installation and Activation

1. Install or update the binary:
   ```bash
   go install github.com/brandonbloom/wt/cmd/wt@latest
   ```
2. Confirm the installed version:
   ```bash
   wt version
   ```
3. Install the shell wrapper so subcommands can change your `cwd`:
   ```bash
   eval "$(wt activate)"
   ```
   Add that line to your shell rc (`~/.zshrc` or similar) so the wrapper remains available in new terminals. The Go binary detects a missing wrapper and prints guidance before exiting.

## Project Layout Expectations

`wt` assumes projects live in a parent directory that contains a `.wt/` folder plus one subdirectory per worktree:

```
/your/projects/
├── .wt/             # local configuration & management state
├── main/            # required default worktree (or `master`)
├── feature-a/
└── demo-branch/
```

Key rules:
- Exactly one default worktree exists and is named `main` (preferred) or `master`.
- `.wt/` sits beside every worktree and holds `config.toml`. The directory is not part of git so it can store machine-local settings.
- Additional worktrees live alongside the default, each mapped to a git worktree and branch of the same name.
- Commands discover the project root by walking up from the current directory until a `.wt/` directory is found, so you can run `wt` from any worktree. Missing `.wt/` directories trigger an error that instructs you to run `wt init`.

## Initializing Repositories

### `wt init`

Run `wt init` inside an existing git repository to convert it into the `<project>/<worktree>` layout. The command:
- Creates `.wt/` and a starter `.wt/config.toml`.
- Validates that exactly one `main`/`master` worktree exists (creating the directory if the repo still lives at the old single-directory path).
- When invoked from a legacy layout, moves the repo into `<project>/<branch>` and leaves `.wt/` next to the worktrees. Each step validates destination paths and rolls back on failure.

### `wt clone <url> [<dest>]`

`wt clone` wraps `git clone`, honoring git’s exit codes. After a successful clone it runs `wt init` automatically inside the new project so the layout and config are ready immediately.

## Creating and Managing Worktrees

### `wt new [<name>] [--base=<branch>]`

Creates a new git worktree and branch under the current project. Behavior:
- When `<name>` is omitted, `wt new` picks a short adjective–noun pair from curated word lists so names stay distinct and safe.
- When the flag is provided, names must be lowercase with digits and hyphens; collisions or reserved names abort with an actionable message.
- `--base=<branch>` controls the branch used to seed the new worktree. By default it uses the current branch if you run from an existing worktree, otherwise the configured default branch (`main` or `master`).
- Implementation detail: the command runs `git worktree add <project-root>/<worktree-name> <base>` so every worktree lives directly beneath the project root. Naming collisions abort.

After the worktree is added, `wt new` instructs the shell wrapper to `cd` into the new directory and runs the configured bootstrap script. If the wrapper is missing, the command exits with instructions to run `wt activate`.

### `wt bootstrap`

Reruns the configured bootstrap script inside the current worktree. The command reads `.wt/config.toml` and obeys the `[bootstrap].strict` toggle. Flags:
- `--strict` / `--no-strict` temporarily override the strict-mode default.
- `-x`, `--xtrace` enable shell tracing before executing the bootstrap command.

Use this when dependencies drift or you need to reapply setup steps after `wt new`.

## Cleaning Up Worktrees (`wt tidy`)

`wt tidy` prunes finished or abandoned worktrees/branches so the project root stays sane without losing work. The command categorizes each non-default worktree before acting:

- **Safe** – Clean worktree/stash, commits already reachable from the default branch (or no unique commits), and at most one PR targeting the head. Safe items can be deleted without losing data.
- **Gray** – Clean but requires human judgment (e.g., diverged more than the configured threshold, stale activity, unique commits not merged yet, ambiguous PR state, or active processes still running inside the worktree).
- **Blocked** – Local changes, stash entries, multiple worktrees per branch, or other situations that guarantee data loss. These are never touched; `wt tidy` prints guidance instead.

Default branch comparisons are workflow-aware: if `origin/<default_branch>` exists locally and your local default branch is not ahead of it, wt treats `origin/<default_branch>` as the source of truth for “merged / unique commits” checks. If your local default branch is ahead of `origin/<default_branch>` (or the remote-tracking ref is missing), wt treats the local default branch as the source of truth.
When the repo is treated as local-first, the dashboard omits the literal `No PR` label (PRs aren’t an expected workflow step), but still shows PR metadata when PRs exist.
Missing/unknown CI does not block deleting safe worktrees; it only becomes a “gray reason” when there is pending work to potentially lose.

Cleanup (for safe items or approved gray ones) removes the worktree directory, deletes the local and remote branches, and finally runs `git remote prune origin` once to drop stale refs.

### Flags & Policies

- `-n, --dry-run` – Print the planned actions without mutating anything.
- `--policy=<auto|safe|all|prompt>` – `auto` (default) cleans safe worktrees automatically and prompts for gray ones; `safe` cleans safe worktrees and automatically declines gray ones (non-interactive); `prompt` asks before every cleanup (including safe); `all` auto-cleans safe and gray.
- Shorthands: `--safe`/`-s`, `--all`/`-a`, and `--prompt`/`-p` map to the policy values so `wt tidy -s` becomes the non-interactive “only touch the obvious stuff” flow and `wt tidy -p` becomes the “ask about everything” flow.

When prompting for gray candidates, `wt tidy` renders a mini status panel showing PR state, ahead/behind counts, divergence badge vs the default branch, last-activity timestamp (max of HEAD, PR updates, or worktree mtime), dirty indicators, stash presence, and any running processes that have their `cwd` inside the worktree. Answer `y` to proceed, `n` to skip, or Ctrl+C to cancel the whole command.

`wt tidy` uses the GitHub CLI for PR/CI metadata when available, but can still clean up safe worktrees without it.

### Targeted Removal (`wt rm`)

`wt rm` applies the same safety rules as `wt tidy`, but only to specific worktrees instead of scanning everything. It’s handy when you already know which branches need to go:

```bash
# Remove the current worktree (must not be main/master).
wt rm

# Remove a couple of specific worktrees by name.
wt rm feature-123 bugfix-alpha

# Paths also work; wt resolves them to their containing worktrees.
wt rm ../demo-branch /path/to/project/foo
```

Key behaviors:
- Names are resolved before paths, and duplicates are ignored so each target is handled once in the order you provided.
- The command refuses to touch the default worktree (`main`/`master`) and errors if you pass a detached HEAD, dirty tree, shared branch, or other blocked state—forcing does not override safety.
- Each target inherits the safe/gray classification logic from `wt tidy`. Safe worktrees delete immediately; gray ones prompt with the same mini status panel unless you pass `-f/--force`.
- Flags:
  - `-n, --dry-run` – Show the planned actions (including per-target reasons and whether remote pruning is needed) without mutating anything.
  - `-f, --force` – Skip prompts for gray worktrees. Blocked targets still refuse to run.
- When you run `wt rm` from inside a worktree that gets deleted, the command instructs the wrapper to `cd` back to the project root first. If the wrapper isn’t active you’ll see a message reminding you to change directories manually.

Cleanup steps mirror `wt tidy`: remove the worktree directory, delete the local branch, delete the remote branch if its tip still matches, and run `git remote prune origin` once if at least one remote ref was removed.

## Process Cleanup (`wt kill`, `wt tidy --kill`)

Active processes inside a worktree force `wt tidy` to classify it as gray. Use the new process cleanup commands when those long-running jobs are safe to terminate so tidying can proceed.

### `wt kill <worktrees...>`

Targets one or more worktrees (names or paths resolved the same way as `wt rm`) and sends signals to any processes whose working directory lives inside each worktree. At least one target is required; duplicates are ignored.

- `-n, --dry-run` – List the processes and signals that would be sent without mutating anything.
- `--signal, -s <value>` – Choose the signal (numeric or name like `TERM`, `HUP`). `-9` is shorthand for `--signal=9`.
- `--timeout=<duration>` – Override how long the command waits for processes to exit before declaring failure (defaults to the configured `kill_timeout`).

Output renders a small block per worktree:

```
busy-branch:
  - server (1111)
  - worker (2222)
  sending SIGTERM (15) to 2 processes
  cleared

idle-branch:
  nothing to kill
```

Failures (e.g., `EPERM`, `ESRCH`, timeouts) produce per-worktree errors and the command exits non-zero while still attempting later targets.

### `wt tidy --kill`

Add `--kill` (or `-k`) when running `wt tidy` to automatically terminate blocking processes before prompting or deleting worktrees. Supplying a value (`--kill=9`, `-k9`) changes the signal; otherwise the default `SIGTERM` is used. The flag respects `--dry-run` by only reporting which processes would be terminated.

Additional knobs:

- `--timeout=<duration>` overrides the wait time (shared with `wt kill`).
- `.wt/config.toml` exposes `[process].kill_timeout = "3s"` to change the default wait globally (see the Configuration Reference).

`wt tidy --kill` re-scans processes after the termination attempt. Successfully cleared worktrees drop back into the safe/gray flow, while those that refuse to exit remain in the blocked set with a new block reason explaining the failure. When stdout is not a TTY, the preflight plan now includes a “Process cleanup” section listing the worktrees and signals slated for termination.

## Shell Integration

The installed Go binary emits shell code when you run `wt activate`. Evaluating the output defines a shell function (also named `wt`) that proxies to the binary and applies directory changes requested by subcommands such as `wt new`. The root command (`wt` or `wt status`) also detects when the wrapper is missing and prints instructions before doing other work.

## Dashboard (`wt` / `wt status`)

Running `wt` with no subcommand prints a status dashboard:
- Exactly one status line per worktree; the current worktree receives an additional highlight plus extended detail.
- Each line shows branch name (only when it differs from the worktree directory), ahead (`↑N`) / behind (`↓M`) counts relative to the worktree’s upstream, dirty indicators, and a divergence badge relative to the default branch. The badge uses `[+N -M]` and is omitted when both counts are zero.
- Timestamps come from the newest dirty/staged file when the worktree has changes; otherwise they use the HEAD commit timestamp. Values render as friendly relative strings such as `3s ago`, `2 min ago`, or `yesterday 2pm`.
- If the branch has an associated GitHub pull request, its status appears inline.
- CI status appears next to the pull-request summary. `CI: ✓` means every GitHub Actions check run succeeded, `CI: ◷` indicates a queued or running workflow, `CI: ✗ <job>` highlights the most relevant failure with a relative timestamp, and `CI: ? …` surfaces problems (missing remotes, unauthenticated `gh`, etc.). The `[ci]` table in `.wt/config.toml` controls which remote wt inspects.
- On macOS and Linux, the dashboard also lists processes owned by the current user whose `cwd` lives inside the worktree (subdirectories included). Entries render as `command (pid)` separated by commas with truncation when the column runs long. Unsupported platforms simply omit this summary.
- When you run `wt status` from inside a worktree whose CI failed, a short “CI details” section prints beneath the table with the failing job name, start/completion times, and the run URL so you can jump straight into logs without digging through the Actions UI.

Before collecting git data, the dashboard performs quick “doctor-lite” checks (wrapper active, `.wt` present, default worktree healthy) and surfaces any issues so you’re not looking at stale information.

When attached to a TTY the dashboard streams updates in place, allowing GitHub data to appear asynchronously while remaining responsive to Ctrl+C. When stdout is redirected the command emits a single non-interactive pass suitable for scripts.

## Health Checks (`wt doctor`)

`wt doctor` verifies the environment so commands succeed later. Checks include:
- Git and GitHub CLI installations plus authentication to GitHub.
- Project layout validity (discoverable `.wt/`, default worktree sanity, readable config file).
- Configured default branch matches GitHub’s reported default.
- Shell wrapper availability.

By default it prints only failures; `wt doctor --verbose` lists each check with a status. The dashboard reuses many of these checks opportunistically.

## GitHub Integration

All GitHub data flows through the `gh` CLI so `wt` relies on its auth and config.
- Pull request association uses `gh pr list --head <branch>` (falling back to other queries as needed) and surfaces statuses when exactly one PR matches. Multiple matches or no matches are reported explicitly.
- Commands stream progress so you can interrupt long-running GitHub calls.

## Error Handling Philosophy

`wt` favors early, actionable errors:
- Each command validates prerequisites (layout, tooling, wrapper, naming) before mutating state.
- Messages describe how to fix the issue (run `wt init`, install the wrapper, resolve naming collisions, etc.).
- When a problem could have been detected by `wt doctor`, add or reference the relevant doctor check so it can be caught proactively.

## Testing & Transcripts

User-facing CLI behavior is covered with transcript fixtures stored under `transcripts/`. Update them via `transcript update` after verifying the new output matches expectations. For workflows that need real git worktrees, use `git@github.com:brandonbloom/wt-playground.git` as the canonical playground repository.

## Next Steps

- Read `doc/configuration.md` to customize `.wt/config.toml`.
- See `DEVELOPING.md` for contributor-focused build/test instructions.
- Run `wt doctor` regularly to confirm your environment stays healthy.
