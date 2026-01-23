# Developing `wt`

## Workflow

- Read `context/spec.md` so changes stay grounded in the product brief.
- The agent playbook lives in `AGENTS.md`; read it before automating changes.
- User-facing CLI behavior should be covered by transcript fixtures in `transcripts/`.
- `wt rm` shares the `wt tidy` safety heuristics; keep their implementations and docs in sync (rm only exposes `-n/--dry-run` and `-f/--force`).
- Reset throwaway repos with `scripts/cleanup.sh <relative-path>` instead of direct `rm -rf`.

## Building & Testing

```bash
mise run build        # go build -o bin/wt . (plus internal tooling) with repo-local caches
mise run test         # go test ./... && transcript check transcripts/*.cmdt
```

Use `bin/wt` for manual experiments. The `transcript` CLI is already on `$PATH`, so run `transcript shell`, `transcript update`, etc., directly when refreshing fixtures. When recording CLI tests, follow `context/transcript.md`. Set `WT_NOW=<RFC3339>` when deterministic relative timestamps are needed (the transcripts rely on this).

## Debugging

`wt` can write a Go execution trace for offline profiling:

```bash
bin/wt --trace trace.out status
go tool trace trace.out
```

The resulting `trace.out` can also be loaded directly into Perfetto (https://ui.perfetto.dev).

`--trace` and `-C/--directory` are applied in argv order, so `-C ... --trace trace.out` writes inside the chdir target while `--trace trace.out -C ...` writes next to where you ran `wt`.

## Transcript Harness

`wtcmdtest` provisions a throwaway repo, runs `wt init`, stubs `gh`, and can
simulate the wrapper (`--activate-wrapper`) plus `cd` into a worktree
(`--worktree <dir>`). Use it inside `.cmdt` files to avoid repeating setup:

```bash
$ wtcmdtest --activate-wrapper --worktree main ../../bin/wt doctor
healthy!
```

## Contributing

- Prefer TDD: author/refresh a failing transcript or Go test before changing code.
- Keep `.wt/config` behavior documented in the README whenever it changes.
- Run `wt doctor` (or `wt doctor --verbose`) after environment tweaks to ensure future users won’t hit regressions.

Pull requests should mention any spec updates and link to the transcript changes that prove the behavior.
