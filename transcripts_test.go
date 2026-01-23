package main_test

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Embed transcript fixtures so changes invalidate the Go test cache.
//
//go:embed transcripts/*
var transcriptFS embed.FS

func TestTranscripts(t *testing.T) {
	// Ensure the embed is referenced so it isn't optimized away.
	if _, err := transcriptFS.ReadDir("transcripts"); err != nil {
		t.Fatalf("read embedded transcripts: %v", err)
	}

	if _, err := exec.LookPath("transcript"); err != nil {
		t.Skipf("transcript not found on PATH (run via `mise run test`): %v", err)
	}

	paths, err := filepath.Glob("transcripts/*.cmdt")
	if err != nil {
		t.Fatalf("glob transcripts: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no transcripts found under transcripts/*.cmdt")
	}
	sort.Strings(paths)

	for _, path := range paths {
		path := path
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command("transcript", "check", path)
			cmd.Env = append(os.Environ(),
				"WT_CMDTEST_ID="+name,
				"WT_CMDTEST_TIMEOUT=60s",
			)

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("transcript check failed for %s: %v\n%s", path, err, out)
			}
		})
	}
}
