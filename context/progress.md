# wt Progress Checklist

This file tracks every requirement from `context/spec.md`. Update the checkboxes as work lands to keep future contributors oriented.

## Project Overview
- [x] CLI entrypoint is `wt` using Cobra, with the root command printing the status dashboard when no subcommand is provided.
- [ ] Binary detects when the shell wrapper is missing and emits installation guidance.
- [x] Logging/output avoids timestamped loggers (plain stdout/stderr messaging only).

## Directory & Worktree Assumptions
- [x] Projects use the `<project>/<worktree>` layout with `.wt/` adjacent to the worktrees and excluded from git.
- [x] Exactly one default worktree exists named `main` or `master`; conflicting/missing defaults error out.
- [x] All commands discover the project root via upward `.wt` search and error with instructions to run `wt init` when absent.
- [x] Dashboard works when invoked from any worktree and highlights the current one while summarizing others.

## Initialization (`wt init`)
- [x] Creates `.wt/` and a template `config.toml` under the project root.
- [x] Converts legacy single-directory repos into `<project>/<branch>` layout with rollback on failure.
- [x] Skips conversion when the layout already matches expectations, only ensuring `.wt/` and config exist.
- [x] Writes the validated default branch and bootstrap stub into the config.
- [x] `wt doctor` verifies the configured default branch matches GitHub’s default.

## Cloning (`wt clone`)
- [x] Wraps `git clone`, mirrors git exit codes, and only runs initialization after a successful clone.
- [x] Automatically runs `wt init` inside the new clone to install the layout.

## Configuration (`.wt/config.toml`)
- [x] Resides at `<project>/.wt/config.toml`, outside the repo.
- [x] Stores `default_branch` and `[bootstrap].run` values, validated on load.
- [x] README documents the config file, `default_branch`, and `[bootstrap]` semantics.
- [ ] `wt bootstrap` reruns the configured bootstrap script inside the current worktree.

## Worktree Naming (`wt new`)
- [x] Generates short, distinct, inoffensive adjective–noun names when none are provided.
- [x] Accepts explicit names and validates them (lowercase, digits, hyphen, non-reserved).
- [x] Supports `--base=<branch>`; default base follows the spec (current branch else main/master).
- [x] Invokes `git worktree add <project-root>/<worktree-name> <base>` and handles collisions.
- [x] Automatically changes into the new worktree via the shell wrapper (or instructs the user when missing).
- [x] Runs the `[bootstrap].run` command synchronously and aborts on failure with a clear message.

## Shell Integration (`wt activate`)
- [x] `wt activate` emits the shell script that installs/updates the wrapper function.
- [x] Wrapper shadows the binary, exports markers for directory change instructions, and can be reinstalled via `eval "$(wt activate)"`.
- [x] `wt status` proactively detects a missing wrapper and guides the user before rendering output.

## Status Dashboard (`wt`)
- [x] Prints exactly one status line per worktree with a marker for the current worktree.
- [x] Displays branch name, ahead/behind counts, and dirty state indicators.
- [ ] Also badges divergence from the default branch using `[+N -M]` when applicable.
- [x] Uses newest dirty/staged file mtime when dirty, else HEAD commit timestamp, and renders the value as a friendly relative string (e.g., `3s ago`, `2 min ago`, `yesterday 2pm`, `4 days ago`).
- [ ] Runs lightweight doctor checks (wrapper active, `.wt` present, default worktree healthy) before collecting git status.
- [x] Shows GitHub PR status for associated branches, fetching via `gh pr list`, with streaming placeholders (`pending…`) and Ctrl+C-friendly behavior.
- [x] Streams GitHub fetch progress by re-rendering the status table when attached to a TTY and degrades to single-pass output when redirected.
- [ ] Prefers silence when nothing noteworthy changed but surfaces actionable info when it does.

## `wt doctor`
- [x] Verifies git, GitHub CLI, and other required tooling are installed and usable.
- [x] Confirms `gh` is authenticated and can reach GitHub.
- [x] Checks the project directory layout (`.wt` discovery, default worktree validity).
- [x] Ensures the configured `default_branch` matches GitHub’s default branch.
- [x] Validates that the shell wrapper is installed and active.
- [x] Prints only failures by default; `--verbose` reports each check result even when passing.

## GitHub Integration
- [x] All GitHub data flows through the `gh` CLI (PR discovery, repo metadata).
- [x] Associates worktrees/branches with PRs via `gh pr list --head <branch>` (with fallbacks) and handles multiple/zero matches explicitly.

## Error Handling
- [ ] All commands emit actionable, early error messages and stop before causing damage.
- [ ] `wt doctor` proactively checks for conditions that would otherwise cause command failures.

## Methodology & Testing
- [ ] Follow strict TDD with failing tests first (unit or transcript) before implementing behavior.
- [ ] Use `git@github.com:brandonbloom/wt-playground.git` when exercising real worktree flows.
- [x] Cover user-facing CLI behavior with transcript `.cmdt` fixtures and keep them updated via `transcript update`.
- [x] Ensure transcript fixtures live under `transcripts/` and remain version-controlled.
