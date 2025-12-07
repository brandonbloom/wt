package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/brandonbloom/wt/internal/processes"
)

const (
	defaultProcessSummaryLimit = 80
	minProcessSummaryEntries   = 3
)

func summarizeProcesses(procs []processes.Process, limit int) string {
	procs = pruneProcessList(procs)
	if len(procs) == 0 {
		return "-"
	}
	if limit <= 0 {
		limit = defaultProcessSummaryLimit
	}

	sorted := append([]processes.Process(nil), procs...)
	sort.Slice(sorted, func(i, j int) bool {
		ci := processCommandLabel(sorted[i].Command)
		cj := processCommandLabel(sorted[j].Command)
		if ci == cj {
			return sorted[i].PID < sorted[j].PID
		}
		return ci < cj
	})

	entries := make([]string, len(sorted))
	for i, proc := range sorted {
		entries[i] = fmt.Sprintf("%s (%d)", processCommandLabel(proc.Command), proc.PID)
	}

	required := minProcessSummaryEntries
	if required > len(entries) {
		required = len(entries)
	}

	var b strings.Builder
	shown := 0
	for idx, entry := range entries {
		sep := ""
		if shown > 0 {
			sep = ", "
		}
		projected := b.Len() + len(sep) + len(entry)
		forced := shown < required
		if !forced && limit > 0 && projected > limit {
			break
		}
		b.WriteString(sep)
		b.WriteString(entry)
		shown++
		if idx == len(entries)-1 {
			break
		}
	}

	remaining := len(entries) - shown
	if remaining > 0 {
		sep := ""
		if shown > 0 {
			sep = ", "
		}
		more := fmt.Sprintf("+ %d more", remaining)
		b.WriteString(sep)
		b.WriteString(more)
	}

	return b.String()
}

func processCommandLabel(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "process"
	}
	fields := strings.Fields(cmd)
	cmd = fields[0]
	cmd = filepath.Base(cmd)
	return cmd
}
