# wt

`wt` is an opinionated helper for managing Git worktrees as first-class project roots. It converts repositories into `<project>/<worktree>` directories, keeps developer-specific configuration outside the repo, and automates short-lived worktree creation with memorable names.

## Installation

```bash
go install github.com/brandonbloom/wt/cmd/wt@latest
eval "$(wt activate)"
```

The `activate` step installs a shell function named `wt` that wraps the Go binary so commands like `wt new` can change your current working directory after completing their work. When the wrapper is missing, commands still run but will remind you to install it.

## Getting Started

1. Clone a repository with `wt clone <url>` **or** run `wt init` inside an existing Git repo. Initialization converts the repo into the `<project>/<branch>` layout and creates a `.wt/` directory next to the worktrees.
2. Use `wt new` to spin up additional worktrees. Names are adjective–noun pairs by default (e.g., `bold-raven`), but you can supply an explicit name or `--base=<branch>` to choose the starting point.
3. Run `wt` with no subcommand to see the dashboard of worktrees, their branch state, and recency.
4. Periodically run `wt doctor` (or `wt doctor --verbose`) to confirm Git/GitHub/direnv prerequisites are still healthy.

## Configuration (`.wt/config.toml`)

Each project stores developer-specific configuration at `<project>/.wt/config.toml`. The file is intentionally outside the Git repository so you can tune settings without creating noise for collaborators.

```toml
default_branch = "main"

[bootstrap]
run = "make deps"
```

- `default_branch` (string) **must** match the repository's default branch on GitHub. `wt doctor` verifies this by asking the GitHub CLI for the canonical branch.
- `[bootstrap].run` is an arbitrary shell snippet that runs inside every freshly created worktree **after** the `git worktree add` step succeeds. The command inherits stdin/stdout/stderr and blocks the `wt new` flow until it finishes. Return codes other than zero abort the flow with the captured error so you can fix bootstrap problems quickly.

Edit this file directly—it's regular TOML—and rerun `wt doctor` if you want to validate the changes.

## Transcript Tests

Integration tests live under `transcripts/` as [command transcripts](https://github.com/deref/transcript). Use `transcript check transcripts/*.cmdt` to keep them in sync whenever you change CLI behavior. See `context/transcript.md` for a deep dive into the format and toolchain.

