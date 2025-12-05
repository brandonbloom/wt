# wt

My opinionated, personal worktree manager.

If you're not me, you probably don't want to use this.

## Install

```bash
# Download.
go install github.com/brandonbloom/wt/cmd/wt@latest

# Install into .zshrc or similar.
eval "$(wt activate)"
```

The `activate` step installs the shell wrapper so subcommands (e.g., `wt new`) can change your `cwd`. The binary warns when the wrapper is missing.

## Everyday Usage

- `wt clone <url>` – clone a repo and immediately convert it to the `<project>/<branch>` layout.
- `wt init` – convert an existing repo in-place.
- `wt new [name] [--base=<branch>]` – create a new branch/worktree (names default to curated adjective–noun pairs). After creation the wrapper `cd`s into the new tree and runs the configured bootstrap command.
- `wt` / `wt status` – dashboard with one line per worktree showing branch state, relative “time ago” updates, and PR placeholders.
- `wt doctor [--verbose]` – verify git/gh installations, layout, config, and wrapper state.

Project-specific settings live in `<project>/.wt/config.toml` next to the worktrees so you can tune the default branch and bootstrap commands without touching the git repo.

## Project Layout

`wt` expects every project to look like:

```
/your/projects/
├── .wt/             # local configuration & management state
│   └── config.toml
├── main/            # required default worktree (or `master`)
├── feature-x/       # examples of worktrees
└── demo-branch/
```

Key rules:

- `.wt/` sits beside every worktree and holds `config.toml`.
- Exactly one default worktree exists and is named `main` (preferred) or `master`.
- Additional worktrees live alongside the default, each corresponding to a git worktree/branch.
- Commands discover the project root by walking upward until `.wt/` is found, so you can run `wt` from any worktree.

`wt init` converts legacy single-directory repos into this layout; `wt clone` sets it up automatically after cloning.

## Configuration

Each project keeps machine-local settings in `<project>/.wt/config.toml` (outside git):

```toml
default_branch = "main"

[bootstrap]
run = "mise run deps"
```

- `default_branch` must match GitHub’s default for the repo; `wt doctor` verifies this via `gh`.
- `[bootstrap].run` executes in your shell immediately after `wt new` creates a worktree. Failures abort the command so you can fix dependencies before continuing.
- Because `.wt/` is not in git, teammates can customize their bootstrap commands or defaults without conflicts.

## Docs & License

- MIT License (see `LICENSE` in the upstream repo).
- Contributor instructions (including build/test steps) live in `DEVELOPING.md`.
- Full product spec and transcript guidance live under `context/`.
