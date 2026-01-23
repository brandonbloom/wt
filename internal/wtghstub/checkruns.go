// Check-runs implements the minimal JSON responses for `gh api .../check-runs`,
// backed by the pipe-delimited state file `WT_GH_CI_FILE`.
package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
)

type checkRunRecord struct {
	EntryType   string
	EntryID     string
	Name        string
	Status      string
	Conclusion  string
	URL         string
	StartedAt   string
	CompletedAt string
}

func emitCheckRuns(ciFile string, key string) string {
	fallback := ""
	switch {
	case strings.HasPrefix(key, "commit|"):
		fallback = "commit|*"
	case strings.HasPrefix(key, "pr|"):
		fallback = "pr|*"
	}
	if os.Getenv("WT_GH_DEBUG") != "" {
		appendLine(ciFile+".log", "ci_key="+key)
	}

	checkRuns := loadCheckRuns(ciFile)
	for _, rec := range checkRuns {
		entry := rec.EntryType + "|" + rec.EntryID
		if entry != key && (fallback == "" || entry != fallback) {
			continue
		}

		resp := map[string]any{
			"total_count": 1,
			"check_runs": []map[string]any{
				{
					"name":         rec.Name,
					"status":       rec.Status,
					"conclusion":   rec.Conclusion,
					"html_url":     rec.URL,
					"details_url":  rec.URL,
					"started_at":   rec.StartedAt,
					"completed_at": rec.CompletedAt,
				},
			},
		}
		b, _ := json.Marshal(resp)
		return string(b)
	}
	return `{"total_count":0,"check_runs":[]}`
}

func loadCheckRuns(path string) []checkRunRecord {
	lines, err := readLines(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return nil
	}
	out := make([]checkRunRecord, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 8 {
			continue
		}
		out = append(out, checkRunRecord{
			EntryType:   parts[0],
			EntryID:     parts[1],
			Name:        parts[2],
			Status:      parts[3],
			Conclusion:  parts[4],
			URL:         parts[5],
			StartedAt:   parts[6],
			CompletedAt: parts[7],
		})
	}
	return out
}
