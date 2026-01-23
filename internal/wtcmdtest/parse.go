// Argument parsing for the `wtcmdtest` harness.
//
// Supported flags:
//   - `--skip-init` (leave repo unconverted)
//   - `--activate-wrapper` (set `WT_WRAPPER_ACTIVE=1` + `WT_INSTRUCTION_FILE=...`)
//   - `--worktree <dir>` (cd under the temp repo before running)
//   - `--keep` (preserve the temp repo for debugging)
//   - `-h/--help`
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type options struct {
	skipInit        bool
	activateWrapper bool
	worktree        string
	keepRepo        bool
	help            bool
}

func parseArgs(args []string) (options, []string, error) {
	var opts options

	fs := flag.NewFlagSet("wtcmdtest", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.BoolVar(&opts.skipInit, "skip-init", false, "")
	fs.BoolVar(&opts.activateWrapper, "activate-wrapper", false, "")
	fs.StringVar(&opts.worktree, "worktree", "", "")
	fs.BoolVar(&opts.keepRepo, "keep", false, "")

	fs.BoolVar(&opts.help, "help", false, "")
	fs.BoolVar(&opts.help, "h", false, "")

	if err := fs.Parse(args); err != nil {
		return options{}, nil, err
	}
	if opts.help {
		return opts, nil, nil
	}

	if opts.worktree != "" {
		if filepath.IsAbs(opts.worktree) {
			return options{}, nil, errors.New("worktree must be a relative path")
		}
		clean := filepath.Clean(opts.worktree)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return options{}, nil, fmt.Errorf("worktree must not escape repo root: %q", opts.worktree)
		}
	}

	cmd := fs.Args()
	if len(cmd) == 0 {
		return options{}, nil, errors.New("missing command")
	}

	return opts, cmd, nil
}
