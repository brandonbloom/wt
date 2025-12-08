# wt Specification (Draft)

## Project Overview

- Repository: `github.com/brandonbloom/wt`
- License: MIT
- Implementation language: Go (using the Cobra CLI framework)
- CLI entrypoint: `wt` (a shell wrapper/function that shadows the Go binary of the same name so it can change the caller’s `cwd`; when the wrapper is missing, the binary still runs and reports the misconfiguration)
- Default behavior: running `wt` with no subcommand prints a status dashboard
- Scope: extremely minimal and opinionated, targeting Brandon Bloom’s personal workflow (others using it is a bonus)
- Logging/output: avoid timestamped loggers (e.g., Go’s default `log` package formatter) since the CLI should finish fast enough that timestamps add no value; stick to plain stdout/stderr messaging instead.

## Directory & Worktree Assumptions

- A project that uses `wt` is converted into a directory containing multiple worktrees (e.g., `~/Projects/iaf` becomes `~/Projects/iaf/main`, `~/Projects/iaf/<other-worktree>`).
- Exactly one “default” worktree must exist and be named `main` or `master` (checked in that order of preference). If both folders exist or neither folder exists (or if they are not valid git worktrees), the tool must exit with a clear error.
- A `.wt/` directory lives alongside the worktrees (e.g., `~/Projects/iaf/.wt`) and is **not** part of the git repo, allowing machine- or user-specific configuration.
- All commands discover `.wt` (and therefore the project root) by walking upward from the current directory until `<dir>/.wt` is found. If no `.wt` directory exists before reaching the filesystem root, exit with an error directing the user to run `wt init`.
- When invoked from inside `main`/`master` or any other worktree under the project directory, `wt` must still function. The dashboard should show a detailed view for the current tree plus summary data for the others.

## Initialization (`wt init`)

- `wt init` creates the `.wt` directory and a template `config.toml` under the project root.
- If run from within an existing git repository that has not yet been converted into the `project/{branch}` layout, the command must:
  - Determine the current project name and branch.
  - Change to the parent directory, move the repository to `${project}-${branch}`, create `${project}/`, then move `${project}-${branch}` into `${project}/${branch}` (validating at each step that the target paths do not already exist and rolling back on failure).
- If a `main` or `master` directory already exists beneath the current directory (and the structure is otherwise consistent with a converted project), `wt init` should simply create `.wt/` and the config file without rearranging directories.
- The generated config file must include the validated default branch name (matching GitHub’s default branch) and a stub `[bootstrap]` section (see below). `wt doctor` must verify that the configured default branch matches GitHub’s reported default.

## Cloning (`wt clone <url> [<dest>]`)

- `wt clone` wraps `git clone`. After cloning into `<dest>` (or the Git default when omitted), it must run `wt init` inside the freshly cloned repository to install the `.wt` layout automatically.
- Honor all `git clone` exit codes and surface failures clearly before attempting initialization.

## Configuration (`.wt/config.toml`)

- Configuration lives at `<project>/.wt/config.toml` outside the repo so it need not be shared with collaborators.
- File format: TOML. At minimum it contains:
  - `default_branch = "main"` (string) which must match the default branch reported by GitHub for the repository.
  - `[bootstrap]` section with a `run = "..."` field whose contents are executed in the user’s default shell (`$SHELL`) immediately after `wt new` creates and enters a worktree. The command runs synchronously and inherits stdin/stdout/stderr; failures abort the `wt new` flow with a clear message.
  - Optional `[bootstrap].strict = false` toggle; when omitted, bootstrap scripts execute under `set -euo pipefail` for safety. Setting `strict = false` reverts to lenient shell semantics.
  - Optional `[process]` section with `kill_timeout = "3s"` (Go duration syntax) that provides the default wait time used by `wt kill` and `wt tidy --kill` before they report a stubborn process as still running. Command-line `--timeout` flags override this value.
- The README must document the configuration file, the `default_branch` field, and the `[bootstrap]` section semantics so that users can edit it without referring to the source.
- A dedicated `wt bootstrap` command reruns the configured bootstrap script within the current worktree, allowing users to reset dependencies or rerun setup later. It respects the `[bootstrap].strict` setting but also accepts `--strict`, `--no-strict`, and `-x/--xtrace` flags to temporarily override strict mode or enable shell tracing.

## Worktree Naming (`wt new`)

- `wt new` creates a new git worktree rooted in the current project.
- New worktree names must be short, memorable, distinct, and inoffensive.
- Strategy: adjective–noun pairs chosen from hard-coded curated dictionaries (~100 safe words in each category).
- `wt new [<name>]` accepts an optional explicit worktree/branch name; omit `<name>` to use the adjective–noun generator.
- `wt new` accepts `--base=<branch>` to choose the branch used to seed the new worktree. Default base logic:
  - If invoked from an existing worktree with a current branch, use that branch.
  - Otherwise use the default `main`/`master`.
- Implementation detail: `wt new` must call `git worktree add <project-root>/<worktree-name> <base>` (creating a new branch named `<worktree-name>` unless that branch already exists) so every worktree lives directly under the project root alongside the default branch directory (the directory that originally contained `.git`). Handle naming collisions by aborting with an actionable error.
- After creation, the tool should change the shell’s working directory into the new worktree (accomplished via the shell wrapper described below).

## Shell Integration (`wt activate`)

- Because a binary cannot directly change the caller’s `cwd`, the installed Go binary is named `wt` and emits shell code that defines a shell wrapper function (also named `wt`) which shadows the binary on `$PATH`.
- Commands that need to `cd` (e.g., `wt new`) should fail politely when the wrapper is missing, but `wt`/`wt status` must proactively detect an inactive wrapper and emit installation guidance before rendering the dashboard.
- `wt activate` is responsible for emitting the shell script that installs/updates the wrapper function. Users add `eval "$(wt activate)"` to their shell rc (zsh assumed, but solution should be shell-agnostic where possible).
- Installation flow: `go install github.com/brandonbloom/wt/cmd/wt@latest`, then add the eval line to shell config.
- Goal: allow commands like `wt new` to create a worktree and automatically `cd` into it through the evaluated shell function.

## Status Dashboard (`wt`)

- Running `wt` with no subcommand prints a dashboard view of all worktrees, rendered as exactly one status line per worktree (current worktree line should include an additional marker/prefix to highlight it).
- Before hitting git, `wt status` should perform lightweight “doctor-lite” checks (wrapper active, `.wt` discoverable, default worktree healthy) and surface any failures inline so users fix issues before reading stale data.
- Required data per worktree:
  - Git details (branch name, ahead/behind vs upstream, dirty state).
- Timestamp derived as: newest file mtime when the worktree is dirty or has staged changes; otherwise use the HEAD commit timestamp. Display the timestamp as a friendly relative string (e.g., `3s ago`, `2 min ago`, `yesterday 2pm`, `4 days ago`) instead of raw ISO text.
  - If the branch has an associated GitHub pull request, display its status.
- Pull request summaries follow the same rules as `wt tidy`: only show badges when the worktree has local changes or commits that are not yet on the default branch. Branches with commits but no PR show `PR: no PR`, while branches with closed/merged PRs and new commits show `PR #123 merged; new commits pending` (or an equivalent state string). Clean branches that already match the default branch fall back to `PR: none`.
- When run inside a specific worktree, highlight that worktree with additional detail while still summarizing the others.
- Display a per-worktree summary of processes owned by the current user whose working directories (after resolving symlinks) live anywhere within that worktree. Format entries as `command (pid)` separated by commas, include at least three entries when available, and append `+ N more` when truncating to fit within roughly 80 columns. On macOS and Linux this data must be gathered via platform APIs (`/proc` on Linux, `sysctl`/`proc_pidpath` on macOS). Unsupported platforms may omit the column entirely, but supported platforms must fail the command if process discovery fails outright.
- Output should respect the “silence is golden” philosophy where possible (e.g., avoid gratuitous chatter when nothing noteworthy changed).
- Performance expectations: local info renders essentially instantly, even with dozens or a few hundred worktrees; remote/GitHub data may stream in afterward, showing placeholders such as “pending…” and respecting Ctrl+C to abort remote fetches. When attached to an interactive TTY, continuously re-render the status table in place so PR updates stream live without relying on external progress libraries. When stdout is not a TTY, emit a single non-interactive pass suitable for scripts.
- Branch status must convey two perspectives without overwhelming the table:
  - Upstream divergence (relative to the branch’s configured upstream, or inferred equivalent) stays as the existing `↑N`/`↓M` markers.
  - Divergence from the configured default branch (e.g., `origin/main`) is shown inline via a short badge appended to the branch column, e.g., `[+5 -2]` when the worktree is five commits ahead and two commits behind the default branch. Omit the badge entirely when both counts are zero.

## Worktree Cleanup (`wt tidy`)

- Purpose: prune finished or abandoned worktrees/branches so the project root stays manageable without losing work.
- Safety classification:
- **Safe** candidates satisfy the “nothing of value will be lost” rule: the worktree has no staged/unstaged changes, no stash entries, its HEAD (and therefore every unique commit) is already reachable from the configured default branch, `git status` is clean, and at most one GitHub pull request targets the branch. If an open PR exists but the commits already landed on the default branch, `wt tidy` treats it as safe and closes the PR as part of cleanup.
  - Feature branches that were merged via squash/rebase (so their commits are no longer ancestors of the default branch) still qualify as safe when their tree matches the default branch—`wt tidy` must detect this and avoid flagging “commits not merged” for these fully synchronized branches.
  - Branches that lag behind the default branch but whose ahead commits are patch-identical to commits already present on the default branch (i.e., `git cherry` reports no unique commits) must also be treated as safe, since deleting them does not lose any effective change.
  - Branches with new commits but only merged/closed PRs must hide the stale PR badge and include a gray reason like “PR #123 merged; new commits pending” so operators know to open a new PR (or discard the work) before tidying.
  - **Gray** candidates carry some ambiguity (e.g., commits not merged yet, a lone PR that has stalled, last activity older than the stale threshold, or divergence beyond the configured limit) but still have a clean worktree/stash so the user can explicitly discard them.
  - **Blocked** candidates have local state that would definitely cause data loss (untracked/staged changes, stash entries, other worktrees pointing at the same branch, or multiple PRs for the same head); `wt tidy` refuses to touch them and prints guidance to resolve the blockers manually.
- Cleanup actions for safe or approved gray candidates happen in one transaction per worktree:
  - Emit a short recap of the branch/worktree slated for deletion.
  - Delete the worktree directory.
  - Delete the corresponding local branch (after confirming no other worktree references it).
  - Delete the remote branch (default `origin`) once HEAD parity is confirmed to avoid nuking rewritten history.
  - Close the associated PR via `gh pr close --comment "...tidy..."`. This runs even for open PRs whose commits already landed in the default branch so dangling references disappear.
  - Prune the remote (`git remote prune origin`) once at the end of the command to remove stale refs.
- CLI ergonomics:
  - `wt tidy` defaults to scanning every non-default worktree. Flags include:
    - `-n, --dry-run`: never mutate anything; instead print “Will clean up:” followed by the per-worktree actions and “Will prompt for:” entries for gray candidates.
    - `--policy=<auto|safe|all|prompt>` where `auto` is the default.
      - `auto` automatically cleans safe candidates and prompts for gray ones.
      - `safe` automatically cleans safe candidates and declines gray ones (no prompts). This is handy for CI or “just clean the obvious stuff” runs.
      - `all` auto-cleans both safe and gray.
      - `prompt` prompts for every candidate, including safe ones.
    - Convenience aliases: `--safe`/`-s`, `--all`/`-a`, and `--prompt`/`-p` map to their respective policy values.
- Prompts render a “mini status panel” for each gray candidate before requesting confirmation. The panel lists PR status, ahead/behind/divergence vs the default branch, last activity timestamp (max of HEAD commit time, PR update time, or worktree mtime), local dirty status, and whether a stash exists.
- When the prompt panel is shown on an interactive TTY and the worktree is classified as gray with commits ahead of the default branch, immediately below the divergence line display up to roughly ten lines of `git log --oneline --graph --decorate` output for the commits that would be discarded (`git log <branch> --not <default>`). This inline graph keeps the operator from cd’ing into the worktree to remember what the branch contains. Skip this snippet for non-interactive runs, safe candidates, or branches with no ahead commits.
- While prompting, `y` proceeds with cleanup, `n` skips, and Ctrl+C aborts the entire run.
  - Output must match the status dashboard ergonomics: when stdout is an interactive TTY, render a live table that updates as data (git + GitHub) streams in, reusing the same column layout/renderer used by `wt status`; when stdout is not a TTY, emit a single non-interactive log with grouped sections (“Will clean up/Will prompt/Will skip”) plus progress updates for each worktree as it finishes.
  - Remote/GitHub fetches (PR metadata, other network calls) should kick off in parallel so the UI updates incrementally instead of blocking on each branch sequentially.
- Gray classification heuristics (all configurable):
  - A branch whose last activity is older than 14 days (default) is considered stale. The counter uses the same timestamp as the prompt panel.
  - More than 20 commits of divergence (ahead or behind) relative to the default branch marks the branch as gray even if it is otherwise clean, since the drift suggests abandonment.
  - Any open PR whose commits have not yet merged into the default branch is gray; `wt tidy` highlights the PR URL/status so the user can make the call.
  - Any branch with multiple PRs (reopened or duplicate heads) is gray because we cannot automatically determine which to close.
- Worktrees that have active processes in their directory tree must be classified as gray even if they would otherwise be safe; the prompt should reuse the same per-worktree process summary rendered by `wt status`.
- When `wt tidy` is invoked from inside a worktree that ultimately gets deleted, the command must automatically change directories back to the project root (or another surviving worktree) before removal so the user never loses their active shell.
- Configuration:
  - `.wt/config.toml` grows a `[tidy]` section with the following keys (all optional):
    - `policy = "auto"` sets the default policy (`auto`, `safe`, `all`, or `prompt`).
    - `stale_days = 14` controls the inactivity threshold in days.
    - `divergence_commits = 20` controls how many commits of ahead/behind drift marks a branch as gray.
  - Future knobs (e.g., remote name) should also live under `[tidy]`.
- Error handling & UX:
  - Refuse to run if `gh` is unavailable, since PR inspection/closure is mandatory for the feature.
  - When skipping a worktree (blocked or rejected prompt), explain why so the user can fix or rerun.
  - The README “Everyday Usage” section should mention `wt tidy` with only the high-traffic flags (`--dry-run`, `--safe`, `--all`), while the detailed config and prompt behaviors belong in `DEVELOPING.md` or a dedicated docs section.
- Cover the new command with transcript fixtures that demonstrate safe cleanup, gray prompting, dry-run output, and blocked cases.

## Process Termination (`wt kill`, `wt tidy --kill/-k`)

- Definition: a “tidy-blocking process” is any process owned by the current user whose working directory (after resolving symlinks) is located inside a worktree directory. These are already surfaced on the status dashboard and cause `wt tidy` to classify the worktree as gray/blocked.
- `wt kill <worktree ...>` targets one or more specific worktrees (names or paths resolved using the same resolver shared with `wt rm`). At least one target is required; duplicates collapse to a single worktree.
  - The command inspects each target to find its tidy-blocking processes. It prints a concise header per worktree followed by `command (pid)` entries; if none exist it reports “nothing to kill” and proceeds.
  - Signals default to `SIGTERM (15)` and can be changed via `--signal=<name|number>`. Provide a shorthand `-9` flag equivalent to `--signal=9`. Symbolic names (e.g., `TERM`, `HUP`) and numeric IDs must both be accepted. `-9` can be combined with other flags (`wt kill -9 -n foo`).
  - `--dry-run/-n` lists the processes and signals that would be sent without actually delivering them. The command must not mutate anything in dry-run mode but still exits non-zero if an invalid worktree name/path was supplied.
  - Signal delivery happens per process; failures (e.g., `ESRCH`, `EPERM`) are reported inline. Any failure after attempting all processes forces a non-zero exit code even when other processes were terminated successfully so operators notice the incomplete cleanup.
  - `--timeout=<duration>` (default 3s) controls how long the command waits for each process to exit after receiving the signal. While waiting it periodically refreshes the process list; if the processes survive past the timeout the command reports the holdouts and fails.
- `wt kill` and `wt tidy` share the resolver/timeout/signal parsing logic to avoid drift. The timeout default comes from a config knob (see below) but can always be overridden by the flag.
- `wt tidy` grows `--kill` / `-k` (optionally `--kill=<signal>`). This flag instructs tidy to proactively terminate tidy-blocking processes for any worktree it plans to clean up.
  - `--kill` without a value uses the same default signal as `wt kill` (SIGTERM). Supplying a value (e.g., `--kill=9` or `-k9`) overrides the signal; both numeric IDs and symbolic names are accepted, though `-k` with an attached value (`-k9`) only supports numeric for simple parsing.
  - `--timeout=<duration>` (default 3s, shared with `wt kill`) governs how long tidy waits after signaling before re-checking the classification. Timeouts happen per worktree so a long-running process in one tree does not stall the entire command.
  - In `--dry-run` mode the kill flag only reports which processes would be terminated.
  - The kill attempt runs after classification but before prompting/deletion so blocked worktrees can become eligible for cleanup. Once all targeted processes exit (confirmed via the same detection logic), tidy re-runs the dirty/process checks and resumes the normal policy flow.
  - If a process refuses to exit after the configured signal and a short retry window, tidy leaves the worktree in the blocked set and reports the failure instead of forcefully deleting the directory.
- Both commands default to the timeout configured under `[process].kill_timeout` (Go duration syntax, default `3s`). Flag values override the config, and environment variables are not required.
- Testing/documentation:
  - Add transcript coverage showing `wt kill` dry-run vs actual termination and the way tidy uses `--kill`.
  - Update the README “Everyday Usage” section (high-traffic flags only) plus deeper docs (`DEVELOPING.md` or a dedicated tidy reference) to describe both commands and the risk involved in terminating processes.
- Future process config: leave room for ignore/allow lists to refine how processes influence `wt tidy` (e.g., ignore `code` helper tasks but always block `psql` or `terraform` runners) and for default signal selection once we learn whether `wt tidy --kill` should default to on/off per project.

## Targeted Removal (`wt rm`)

- Purpose: delete one or more specific worktrees/branches/PRs in the same way `wt tidy` would, without scanning others.
- Invocation: `wt rm [target ...]` where `target` is optional.
  - With no arguments, the command must be run from inside a non-default worktree; that worktree becomes the sole target and the command errors otherwise.
  - With arguments, resolve each (in order) first as a worktree name under the project root, falling back to interpreting it as a path (absolute or relative). Paths must map to a known worktree directory (or something contained within it); ambiguous matches should produce an error. Duplicate identifiers should be ignored so each worktree is processed at most once per invocation.
- Safety/classification logic mirrors `wt tidy`:
  - Inspect the target with the same heuristics (dirty, stash, shared branches, PR state, divergence, stale clocks, process usage, etc.) to determine whether it is safe, gray, or blocked.
  - Blocked worktrees (including the default `main`/`master`, detached HEADs, dirty trees, stash entries, shared branches, or currently-active shells) must always refuse to run, even when forced.
  - Safe worktrees delete immediately; gray worktrees display the same mini status/prompt panel `wt tidy` uses.
- Flags: only `--dry-run/-n` and `--force/-f`. Dry-run prints the planned actions for all requested targets (in order) and never mutates. Force skips the gray prompts (safe worktrees never prompt) but still refuses blocked targets.
- Cleanup steps are identical to `wt tidy`: remove the worktree directory, delete the local branch, delete the remote branch if its tip still matches, close any open PRs referencing the branch, and run `git remote prune origin` if a remote ref was touched. `gh` remains a hard requirement.
- When invoked from inside the worktree being deleted, `wt rm` must change directories back to the project root (or another surviving worktree, mirroring `wt tidy`) before removal. In multi-target runs, this relocation happens before deleting the first target that contains the current directory.
- Document `wt rm` in the spec/README/DEVELOPING contexts alongside `wt tidy`, and cover the behavior with transcript tests (safe deletion, gray prompt, dry-run, blocked/forbidden cases, and forcing through gray).

## `wt doctor`

- Purpose: verify the environment and installation so that all `wt` functionality will succeed (shell wrapper installed, directory layout valid, git state sane, etc.).
- Checks must confirm required tooling is installed and usable, including git and the GitHub CLI (`gh`), that `gh` is authenticated and can reach GitHub, that the expected project directory layout is present (including a `.wt` directory discovered via the upward walk), that the configured `default_branch` matches GitHub’s default, and that the shell wrapper is installed.
- Include a process-detection check on supported platforms that exercises the same discovery logic used by `wt status`/`wt tidy` (e.g., ensure the current process can be observed). Surfacing this via `wt doctor` helps users fix permission issues before other commands fail.
- Architecture: the actual checks should run opportunistically (cheap checks can run on every command), but reporting is separated.
  - Default behavior: only report problems (no news is good news).
  - `wt doctor` prints a positive confirmation (e.g., “healthy!”) when everything passes.
  - `wt doctor --verbose` lists each check and its result, even when passing.

## GitHub Integration

- All GitHub data (e.g., PR status) should be obtained via the GitHub CLI (`gh`) to piggyback on its configuration/auth and avoid duplicating logic.
- To associate a branch/worktree with PRs, use `gh pr list --head <branch>` (falling back to `gh pr list` if needed) and present the most relevant PR status when exactly one match exists; handle multiple matches or empty results explicitly.

## Error Handling Expectations

- Every command must provide actionable error messages and avoid proceeding when validation could have caught a problem earlier.
- When implementing new checks, consider whether `wt doctor` could have reported the issue proactively; if so, add or reference the corresponding doctor check so users can fix their environment before rerunning operational commands.

## Methodology & Testing

- Follow strict TDD for all features. Write a failing unit or transcript test first, make it pass, and keep the suite green.
- Use `git@github.com:brandonbloom/wt-playground.git` as the canonical repository for workflow testing, especially when exercising real worktree operations.
- All user-facing CLI behavior (anything non-trivial to unit test) must be covered with [transcript](https://github.com/deref/transcript) `.cmdt` fixtures. Consult `context/transcript.md` in this repo for usage instructions.
- Keep transcripts under version control and update them via `transcript update` only after validating that the new output matches the spec.
