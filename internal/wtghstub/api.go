// API implements the subset of `gh api ...` used by `wt` during transcript tests.
//
// Supported endpoints:
//   - `graphql` (see `graphql.go`)
//   - `repos/.../commits/.../check-runs` (see `checkruns.go`)
//   - `repos/.../actions/runs...` (returns no runs)
package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

func handleAPI(stateFile string, ciFile string, args []string) (string, int) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "gh stub: api requires a path")
		return "", 1
	}
	endpoint := args[0]
	apiArgs := args[1:]
	base := strings.SplitN(endpoint, "?", 2)[0]

	switch {
	case base == "graphql":
		out, err := handleGraphQL(stateFile, apiArgs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return "", 1
		}
		return out, 0
	case isCheckRuns(base):
		ref := extractCheckRunsRef(base)
		decodedRef := urlDecode(ref)
		if os.Getenv("WT_GH_DEBUG") != "" {
			appendLine(ciFile+".log", "decoded_ref="+decodedRef)
		}

		key := "commit|" + decodedRef
		if strings.HasPrefix(decodedRef, "refs/pull/") && strings.HasSuffix(decodedRef, "/merge") {
			prNum := strings.TrimSuffix(strings.TrimPrefix(decodedRef, "refs/pull/"), "/merge")
			if os.Getenv("WT_GH_DEBUG") != "" {
				appendLine(ciFile+".log", "pr_num="+prNum)
			}
			key = "pr|" + prNum
		}

		return emitCheckRuns(ciFile, key), 0
	case isActionsRuns(base):
		return `{"total_count":0,"workflow_runs":[]}`, 0
	default:
		return "", 1
	}
}

func urlDecode(s string) string {
	decoded, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

func isCheckRuns(path string) bool {
	return strings.HasPrefix(path, "repos/") && strings.Contains(path, "/commits/") && strings.HasSuffix(path, "/check-runs")
}

func extractCheckRunsRef(path string) string {
	idx := strings.Index(path, "/commits/")
	if idx == -1 {
		return ""
	}
	rest := path[idx+len("/commits/"):]
	return strings.TrimSuffix(rest, "/check-runs")
}

func isActionsRuns(path string) bool {
	return strings.HasPrefix(path, "repos/") && strings.Contains(path, "/actions/runs")
}
