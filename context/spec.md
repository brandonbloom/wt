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
  - `[bootstrap]` section with a single `run = "..."` field whose contents are executed in the user’s default shell (`$SHELL`) immediately after `wt new` creates and enters a worktree. The command runs synchronously and inherits stdin/stdout/stderr; failures abort the `wt new` flow with a clear message.
- The README must document the configuration file, the `default_branch` field, and the `[bootstrap]` section semantics so that users can edit it without referring to the source.

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
- When the shell wrapper is not installed, running the binary directly should detect the condition and provide installation guidance rather than failing silently.
- `wt activate` is responsible for emitting the shell script that installs/updates the wrapper function. Users add `eval "$(wt activate)"` to their shell rc (zsh assumed, but solution should be shell-agnostic where possible).
- Installation flow: `go install github.com/brandonbloom/wt/cmd/wt@latest`, then add the eval line to shell config.
- Goal: allow commands like `wt new` to create a worktree and automatically `cd` into it through the evaluated shell function.

## Status Dashboard (`wt`)
- Running `wt` with no subcommand prints a dashboard view of all worktrees, rendered as exactly one status line per worktree (current worktree line should include an additional marker/prefix to highlight it).
- Required data per worktree:
  - Git details (branch name, ahead/behind vs upstream, dirty state).
- Timestamp derived as: newest file mtime when the worktree is dirty or has staged changes; otherwise use the HEAD commit timestamp. Display the timestamp as a friendly relative string (e.g., `3s ago`, `2 min ago`, `yesterday 2pm`, `4 days ago`) instead of raw ISO text.
  - If the branch has an associated GitHub pull request, display its status.
- When run inside a specific worktree, highlight that worktree with additional detail while still summarizing the others.
- Output should respect the “silence is golden” philosophy where possible (e.g., avoid gratuitous chatter when nothing noteworthy changed).
- Performance expectations: local info renders essentially instantly, even with dozens or a few hundred worktrees; remote/GitHub data may stream in afterward, showing placeholders such as “pending…” and respecting Ctrl+C to abort remote fetches. Bonus idea: when attached to an interactive TTY, dynamically update remote sections; when piping to scripts, emit a single pass suitable for parsing.

## `wt doctor`
- Purpose: verify the environment and installation so that all `wt` functionality will succeed (shell wrapper installed, directory layout valid, git state sane, etc.).
- Checks must confirm required tooling is installed and usable, including git and the GitHub CLI (`gh`), that `gh` is authenticated and can reach GitHub, that the expected project directory layout is present (including a `.wt` directory discovered via the upward walk), that the configured `default_branch` matches GitHub’s default, and that the shell wrapper is installed.
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
