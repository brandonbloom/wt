# wt Configuration Reference

Configuration lives in `<project>/.wt/config.toml`, beside your worktrees but outside git so each machine can customize settings safely. This document explains every supported field.

```toml
default_branch = "main"

[bootstrap]
run = "mise run deps"
# strict = false

[ci]
# remote = "origin"
```

## `default_branch`

- Type: string.
- Required: yes.
- The name of the default branch (`main` or `master` for most repositories). `wt init` populates this value after validating it against GitHub’s default branch for the repository.
- `wt doctor` re-validates the value periodically and fails if it diverges from GitHub, preventing surprises when new branches are created.
- Commands that need a fallback branch (`wt new` without `--base`, dashboard comparisons, divergence badges) read this value, so keep it accurate.

## `[bootstrap]` Table

### `run`

- Type: string (required).
- Shell command that runs immediately after `wt new` creates and enters a worktree. Common tasks include installing dependencies or running project-specific setup scripts.
- The command executes inside your default shell (`$SHELL`) with stdin/stdout/stderr attached so you can interact with prompts.
- Failures abort `wt new` or `wt bootstrap` with a clear message so you can fix the issue before continuing.

### `strict`

- Type: boolean (optional, default `true`).
- When omitted or set to `true`, the bootstrap command runs under `set -euo pipefail` for defensive shell semantics.
- Set `strict = false` if your bootstrap command relies on lenient behavior.
- `wt bootstrap` accepts `--strict` or `--no-strict` to override the configuration temporarily, plus `-x/--xtrace` to print commands before executing them. This is useful for troubleshooting flaky setups.

## `[tidy]` Table

Controls the default behavior of `wt tidy`. All keys are optional; the CLI falls back to built-in defaults when omitted.

### `policy`

- Type: string (`"auto"`, `"safe"`, `"all"`, or `"prompt"`). Default: `"auto"`.
- Mirrors the `--policy` flag:
  - `"auto"` (default) cleans safe worktrees automatically and prompts only for gray ones.
  - `"safe"` cleans safe worktrees and automatically declines gray ones (no prompts).
  - `"all"` auto-cleans both safe and gray.
  - `"prompt"` asks before touching anything, including clearly safe worktrees.

### `stale_days`

- Type: integer (default `14`).
- Branches whose last activity (max of worktree mtime, HEAD commit, or PR update) exceeds this age are marked gray so you can decide whether to delete them.

### `divergence_commits`

- Type: integer (default `20`).
- Branches with more than this many commits ahead or behind the default branch become gray even if they are otherwise clean.

## `[process]` Table

Controls process cleanup defaults shared by `wt kill` and `wt tidy --kill`.

### `kill_timeout`

- Type: duration string (default `"3s"`).
- Determines how long the commands wait for a process to exit after sending the signal. Values follow Go’s duration syntax (`500ms`, `2s`, `1m30s`, etc.).
- `wt kill --timeout` and `wt tidy --kill --timeout` override this per invocation.

## `[ci]` Table

Controls how wt discovers GitHub CI metadata for the dashboard and tidy prompts.

### `remote`

- Type: string (default `"origin"`).
- Specifies which git remote contains the canonical GitHub repository. `wt status`, `wt tidy`, and `wt rm` shell out to `gh` against this remote to fetch check runs and workflow information.
- Override the default when your local clone uses a different remote name (e.g., `upstream`). Projects with mirrored repositories can point wt at whichever remote GitHub hosts.

## Editing Tips

- Because `.wt/` is not part of git, edits affect only the local machine. Copy the file manually if you need to share settings.
- Keep file permissions restrictive if secrets (such as API tokens) end up in custom bootstrap commands.
- After editing `config.toml`, run `wt doctor` to confirm the file parses and the environment remains healthy.

## Future Additions

`wt` intentionally keeps configuration surface area small. If you need additional knobs, prefer adding flags to subcommands (documented in `wt --help`) so the config file stays reserved for long-lived project defaults.
