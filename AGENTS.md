# Agent Notes

- The canonical product specification lives at `context/spec.md`. Read it before making changes so you stay aligned with the workflow expectations (including the logging guidance about avoiding timestamped loggers). The public `README.md` is a user guide; contributor info belongs in `DEVELOPING.md`.
- Only surface high-traffic options (those most users need daily) in the README “Everyday Usage” section; niche flags stay in `--help` to keep the doc approachable.
- When the user says “new requirement:” (or similar), pause to interview/clarify the ask, then update `context/spec.md` before resuming prior work (or move to the next requirement if the old task is done).
- `context/transcript.md` documents how to capture `.cmdt` transcript tests; keep new CLI behavior covered there.
- Always build the `wt` binary with `mise run build`.
- Build whenever the user is going to use the binary, as `./bin/wt` is symlinked globally, so that the user can test in other directories.
- Follow strict TDD with failing tests first (unit or transcript) before implementing behavior.
- The primary user prefers light themes. Selected colors should be readable on a white-background terminal.
- Part of implementing new features is documenting them correctly in the user guide and references. If the new feature affects the every-day-use or getting-started experience, ask the user about updating the main README.md as well.
