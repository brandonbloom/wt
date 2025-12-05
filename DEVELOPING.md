# Developing `wt`

## Workflow

- Read `context/spec.md` and keep `context/progress.md` up to date.
- The agent playbook lives in `AGENTS.md`; read it before automating changes.
- User-facing CLI behavior should be covered by transcript fixtures in `transcripts/`.

## Building & Testing

```bash
mise run build        # go build -o bin/ ./cmd/wt with repo-local caches
mise run test         # go test ./...
```

Use `bin/wt` for manual experiments. When recording CLI tests, follow `context/transcript.md`.

## Contributing

- Prefer TDD: author/refresh a failing transcript or Go test before changing code.
- Keep `.wt/config` behavior documented in the README whenever it changes.
- Run `wt doctor` (or `wt doctor --verbose`) after environment tweaks to ensure future users won’t hit regressions.

Pull requests should mention any spec updates and link to the transcript changes that prove the behavior.***
