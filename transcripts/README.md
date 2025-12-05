# Transcript Test Suite

This directory contains [transcript](https://github.com/deref/transcript) fixtures that exercise the `wt` CLI exactly as users experience it. Each `*.cmdt` file records a shell session and is validated with `transcript check`.

## Workflow

1. Install the helper tool once:

   ```bash
   go install github.com/deref/transcript/cmd/transcript@latest
   ```

2. Record or update a session:

   ```bash
   transcript shell -o transcripts/init.cmdt
   ```

3. Keep the fixtures up to date as commands evolve:

   ```bash
   transcript check transcripts/*.cmdt
   ```

The `context/transcript.md` guide in this repository dives deeper into the format if you need a refresher.

