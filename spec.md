# wt Specification (Draft)

## Project Overview
- Repository: `github.com/brandonbloom/wt`
- License: MIT
- Implementation language: Go (using the Cobra CLI framework)
- CLI entrypoint: `wt` (must be a tiny shell script wrapper so it can change the caller’s `cwd`)
- Default behavior: running `wt` with no subcommand prints a status dashboard
- Scope: extremely minimal and opinionated, targeting Brandon Bloom’s personal workflow (others using it is a bonus)

## Directory & Worktree Assumptions
- A project that uses `wt` is converted into a directory containing multiple worktrees (e.g., `~/Projects/iaf` becomes `~/Projects/iaf/main`, `~/Projects/iaf/<other-worktree>`).
- Exactly one “default” worktree must exist and be named `main` or `master` (checked in that order of preference). The tool should exit with an explanatory error if neither folder exists or if they are not valid git worktrees.
- A `.wt/` directory lives alongside the worktrees (e.g., `~/Projects/iaf/.wt`) and is **not** part of the git repo, allowing machine- or user-specific configuration.
- When invoked from inside `main`/`master` or any other worktree under the project directory, `wt` must still function. The dashboard should show a detailed view for the current tree plus summary data for the others.

## Configuration (`.wt/config.toml`)
- Configuration lives at `<project>/.wt/config.toml` outside the repo so it need not be shared with collaborators.
- Primary setting: command or script to run after `wt new` creates a worktree and changes into it (e.g., for bootstrapping dependencies). Additional settings may be added later as needed.

## Worktree Naming (`wt new`)
- `wt new` creates a new git worktree rooted in the current project.
- New worktree names must be short, memorable, distinct, and inoffensive.
- Strategy: adjective–noun pairs chosen from hard-coded curated dictionaries (~100 safe words in each category).
- `wt new` accepts `--base=<branch>` to choose the branch used to seed the new worktree. Default base logic:
  - If invoked from an existing worktree with a current branch, use that branch.
  - Otherwise use the default `main`/`master`.
- After creation, the tool should change the shell’s working directory into the new worktree (accomplished via the shell wrapper described below).

## Shell Integration (`wt activate`)
- Because a binary cannot directly change the caller’s `cwd`, the `wt` shell script must output shell code that the wrapper function evaluates.
- `wt activate` is responsible for emitting the shell script that installs the wrapper function. Users add `eval "$(wt activate)"` to their shell rc (zsh assumed, but solution should be shell-agnostic where possible).
- Installation flow: `go install github.com/brandonbloom/wt/cmd/wt@latest`, then add the eval line to shell config.
- Goal: allow commands like `wt new` to create a worktree and automatically `cd` into it through the evaluated shell function.

## Status Dashboard (`wt`)
- Running `wt` with no subcommand prints a dashboard view of all worktrees.
- Required data per worktree:
  - Git details (branch name, ahead/behind vs upstream, dirty state).
  - Timestamp chosen as the newest among: modified file mtimes, staged file mtimes, or the last commit time if clean.
  - If the branch has an associated GitHub pull request, display its status.
- When run inside a specific worktree, highlight that worktree with additional detail while still summarizing the others.
- Output should respect the “silence is golden” philosophy where possible (e.g., avoid gratuitous chatter when nothing noteworthy changed).
- Performance expectations: local info renders essentially instantly, even with dozens or a few hundred worktrees; remote/GitHub data may stream in afterward, showing placeholders such as “pending…” and respecting Ctrl+C to abort remote fetches. Bonus idea: when attached to an interactive TTY, dynamically update remote sections; when piping to scripts, emit a single pass suitable for parsing.

## `wt doctor`
- Purpose: verify the environment and installation so that all `wt` functionality will succeed (shell wrapper installed, directory layout valid, git state sane, etc.).
- Checks must confirm required tooling is installed and usable, including git and the GitHub CLI (`gh`), that `gh` is authenticated and can reach GitHub, that the expected project directory layout is present, and that the shell wrapper is installed.
- Architecture: the actual checks should run opportunistically (cheap checks can run on every command), but reporting is separated.
  - Default behavior: only report problems (no news is good news).
  - `wt doctor` prints a positive confirmation (e.g., “healthy!”) when everything passes.
  - `wt doctor --verbose` lists each check and its result, even when passing.

## GitHub Integration
- All GitHub data (e.g., PR status) should be obtained via the GitHub CLI (`gh`) to piggyback on its configuration/auth and avoid duplicating logic.
