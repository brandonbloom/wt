// wtghstub is a hermetic stub for the `gh` CLI used by transcript tests.
//
// Supported subcommands:
//   - `gh auth status` (always succeeds)
//   - `gh repo view` (prints "main")
//   - `gh pr list` / `gh pr close`
//   - `gh api graphql`, `gh api repos/.../commits/.../check-runs`, `gh api repos/.../actions/runs...`
//   - `gh run list` (prints "[]")
//
// State is read from `WT_GH_STATE_FILE` and `WT_GH_CI_FILE` (defaults are `.gh-prs` and `.gh-ci` in `$PWD`).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	stateFile := getenvDefault("WT_GH_STATE_FILE", filepath.Join(mustGetwd(), ".gh-prs"))
	ciFile := getenvDefault("WT_GH_CI_FILE", filepath.Join(mustGetwd(), ".gh-ci"))

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "gh stub: missing subcommand")
		os.Exit(1)
	}

	sub := os.Args[1]
	args := os.Args[2:]

	switch sub {
	case "auth":
		if len(args) >= 1 && args[0] == "status" {
			os.Exit(0)
		}
	case "repo":
		if len(args) >= 1 && args[0] == "view" {
			fmt.Fprintln(os.Stdout, "main")
			os.Exit(0)
		}
	case "pr":
		if len(args) >= 1 && args[0] == "list" {
			if delay := strings.TrimSpace(os.Getenv("WT_TEST_GH_DELAY")); delay != "" {
				time.Sleep(parseDelay(delay))
			}
			out, code := handlePRList(stateFile, args[1:])
			if out != "" {
				fmt.Fprintln(os.Stdout, out)
			}
			os.Exit(code)
		}
		if len(args) >= 1 && args[0] == "close" {
			out, err := handlePRClose(stateFile, args[1:])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, out)
			os.Exit(0)
		}
	case "api":
		out, code := handleAPI(stateFile, ciFile, args)
		if out != "" {
			fmt.Fprintln(os.Stdout, out)
		}
		os.Exit(code)
	case "run":
		if len(args) >= 1 && args[0] == "list" {
			fmt.Fprintln(os.Stdout, "[]")
			os.Exit(0)
		}
	}

	fmt.Fprintf(os.Stderr, "gh stub cannot handle: %s %s\n", sub, strings.Join(args, " "))
	os.Exit(1)
}
