# wt

Opinionated worktree helper for repositories that live as `<project>/<worktree>` directories.

## Install

```bash
go install github.com/brandonbloom/wt/cmd/wt@latest
eval "$(wt activate)"
```

The `activate` step installs the shell wrapper so subcommands (e.g., `wt new`) can change your `cwd`. The binary warns when the wrapper is missing.

## Everyday Usage

- `wt clone <url>` – clone a repo and immediately convert it to the `<project>/<branch>` layout.
- `wt init` – convert an existing repo in-place.
- `wt new [name] [--base=<branch>]` – create a new branch/worktree (names default to curated adjective–noun pairs). After creation the wrapper `cd`s into the new tree and runs the configured bootstrap command.
- `wt` – dashboard with one line per worktree showing branch state, relative “time ago” updates, and PR placeholders.
- `wt doctor [--verbose]` – verify git/gh installations, layout, config, and wrapper state.

Project-specific settings live in `<project>/.wt/config.toml` next to the worktrees so you can tune the default branch and bootstrap commands without touching the git repo.

## Building

Local builds use [`mise`](https://github.com/jdx/mise) to keep `bin/` on your `PATH` and pin Go caches inside the repo:

```bash
mise run build
```

This runs `go build -o bin/ ./cmd/wt` with `GOCACHE={{repo}}/.gocache` and `GOMODCACHE={{repo}}/.gopath`. Testing and contributor workflow live in `DEVELOPING.md`.

## Docs & License

- MIT License (see `LICENSE` in the upstream repo).
- Contributor instructions live in `DEVELOPING.md`.
- Full product spec and transcript guidance live under `context/`.
