# Agent Notes

- The canonical product specification lives at `context/spec.md`. Read it before making changes so you stay aligned with the workflow expectations (including the logging guidance about avoiding timestamped loggers). The public `README.md` is a user guide; contributor info belongs in `DEVELOPING.md`.
- When the user says “new requirement:” (or similar), pause to interview/clarify the ask, then update `context/spec.md` and `context/progress.md` before resuming prior work (or move to the next requirement if the old task is done).
- Keep the requirement checklist in `context/progress.md` up to date while you work so other agents can see what remains.
- `context/transcript.md` documents how to capture `.cmdt` transcript tests; keep new CLI behavior covered there.
- Always build the `wt` binary with `mise run build`.
- Build whenever the user is going to use the binary, as `./bin/wt` is symlinked globally, so that the user can test in other directories.
