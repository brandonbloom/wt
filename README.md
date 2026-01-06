# wt

My opinionated, personal worktree manager.

If you're not me, you probably don't want to use this.

## Features

- **Multi-worktree dashboard** – Live status of all branches, PRs, dirty state, and running processes in one view
- **GitHub integration** – Automatic PR status sync and branch validation
- **Safe worktree cleanup** – Intelligently identifies merged/stale worktrees without losing work
- **Zero setup** – Clone any repo and get full worktree management instantly

## Quick Start

```bash
# Install or upgrade.
go install github.com/brandonbloom/wt/cmd/wt@latest

# Confirm the installed version.
wt version

# Install the shell wrapper (add to your shell rc).
# This enables wt to automatically change directories.
eval "$(wt activate)"

# Convert an existing repo.
cd /path/to/repo && wt init

# Or clone a new one.
wt clone "git@github.com:${owner}/${project}.git"

# Customize your wt config (optional).
$EDITOR .wt/config.toml

# Day-to-day flow:
wt new      # spin up a fresh worktree (auto-runs bootstrap)
wt status   # review every worktree + PR state
wt tidy     # prune finished branches/worktrees/etc
wt kill foo # terminate blocking processes inside a worktree
wt rm foo   # remove a specific worktree (use -n/-f as needed)
```

## Documentation

- [User Guide](doc/user-guide.md) – expected directory layout, end-to-end workflows, dashboard/doctor behavior, and error-handling philosophy.
- [Configuration Reference](doc/configuration.md) – every field supported in `.wt/config.toml`, plus editing tips.
- `DEVELOPING.md` – contributor build/test details.
- `context/` – transcript guidance and historical specs.

## Docs & License

- MIT License (see `LICENSE` in the upstream repo).
