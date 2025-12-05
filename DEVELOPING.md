# Developing `wt`

## Workflow

- Read `context/spec.md` and keep `context/progress.md` up to date.
- The agent playbook lives in `AGENTS.md`; read it before automating changes.
- User-facing CLI behavior should be covered by transcript fixtures in `transcripts/`.

## Building & Testing

```bash
mise run build        # go build -o bin/ ./cmd/wt with repo-local caches
mise run test         # go test ./... && go tool github.com/deref/transcript check transcripts/*.cmdt
```

Use `bin/wt` for manual experiments. When recording CLI tests, follow `context/transcript.md`. Set `WT_NOW=<RFC3339>` when deterministic relative timestamps are needed (the transcripts rely on this).

## Transcript Harness

`transcripts/entrypoint.sh` provisions a throwaway repo, runs `wt init`, stubs `gh`,
and can simulate the wrapper (`--activate-wrapper`) plus `cd` into a worktree
(`--worktree <dir>`). Use it inside `.cmdt` files to avoid repeating setup:

```bash
$ ./transcripts/entrypoint.sh --activate-wrapper --worktree main ../../bin/wt doctor
healthy!
```

## Contributing

- Prefer TDD: author/refresh a failing transcript or Go test before changing code.
- Keep `.wt/config` behavior documented in the README whenever it changes.
- Run `wt doctor` (or `wt doctor --verbose`) after environment tweaks to ensure future users won’t hit regressions.

Pull requests should mention any spec updates and link to the transcript changes that prove the behavior.***
