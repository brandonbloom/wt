// PR implements the subset of `gh pr ...` used by `wt` during transcript tests.
//
// Backed by `WT_GH_STATE_FILE` (pipe-delimited records):
//
//	branch|number|state|isDraft|updatedAt|url
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

type prRecord struct {
	Branch    string
	Number    int
	State     string
	IsDraft   bool
	UpdatedAt string
	URL       string
}

func handlePRList(stateFile string, args []string) (string, int) {
	branch := ""
	limit := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--head":
			if i+1 >= len(args) {
				return "", 1
			}
			branch = args[i+1]
			i++
		case "--limit":
			if i+1 >= len(args) {
				return "", 1
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return "", 1
			}
			limit = n
			i++
		case "--state", "--json":
			i++
		default:
		}
	}

	prs := loadPRs(stateFile)
	out := make([]map[string]any, 0, len(prs))
	for _, pr := range prs {
		if branch != "" && pr.Branch != branch {
			continue
		}
		out = append(out, map[string]any{
			"number":    pr.Number,
			"state":     pr.State,
			"isDraft":   pr.IsDraft,
			"updatedAt": pr.UpdatedAt,
			"url":       pr.URL,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}

	b, _ := json.Marshal(out)
	return string(b), 0
}

func handlePRClose(stateFile string, args []string) (string, error) {
	if len(args) < 1 {
		return "", errors.New("gh stub: pr close requires a number")
	}
	number := args[0]
	lines, err := readLines(stateFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}

	found := false
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		if parts[1] == number && !found {
			parts[2] = "CLOSED"
			found = true
		}
		out = append(out, strings.Join(parts[:6], "|"))
	}

	if !found {
		return "", fmt.Errorf("gh stub: pull request #%s not found", number)
	}
	if err := os.WriteFile(stateFile, []byte(strings.Join(out, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("Closed PR #%s", number), nil
}

func loadPRs(path string) []prRecord {
	lines, err := readLines(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return nil
	}
	out := make([]prRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		isDraft := parts[3] == "true"
		out = append(out, prRecord{
			Branch:    parts[0],
			Number:    n,
			State:     parts[2],
			IsDraft:   isDraft,
			UpdatedAt: parts[4],
			URL:       parts[5],
		})
	}
	return out
}
