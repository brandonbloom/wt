# Transcript Test Suite

This directory contains [transcript](https://github.com/deref/transcript) fixtures that exercise the `wt` CLI exactly as users experience it. Each `*.cmdt` file records a shell session and is validated with `transcript check`.

## Workflow

1. Record or update a session (the `transcript` CLI is preinstalled in the toolchain):

   ```bash
   transcript shell -o transcripts/init.cmdt
   ```

2. Keep the fixtures up to date as commands evolve:

   ```bash
   transcript check transcripts/*.cmdt
   ```

Use `scripts/cleanup.sh tmprepo` (or another relative path) to drop temporary repos without tripping sandbox approvals.

The `context/transcript.md` guide in this repository dives deeper into the format if you need a refresher.

## Test Harness

`transcripts/entrypoint.sh` bootstraps a disposable wt project, stubs `gh` (with canned responses so tests stay deterministic/offline), and
optionally simulates the shell wrapper. Use it inside transcripts to avoid
duplicating setup/teardown boilerplate:

```bash
$ ./transcripts/entrypoint.sh --activate-wrapper --worktree main ../../bin/wt doctor
healthy!
```
