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

	type summaryEntry struct {
		label string
		pids  []int
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

	grouped := make([]summaryEntry, 0, len(sorted))
	for _, proc := range sorted {
		label := processCommandLabel(proc.Command)
		if len(grouped) > 0 && grouped[len(grouped)-1].label == label {
			grouped[len(grouped)-1].pids = append(grouped[len(grouped)-1].pids, proc.PID)
			continue
		}
		grouped = append(grouped, summaryEntry{
			label: label,
			pids:  []int{proc.PID},
		})
	}

	entries := make([]string, len(grouped))
	for i, entry := range grouped {
		entries[i] = fmt.Sprintf("%s (%s)", entry.label, joinPIDs(entry.pids))
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

func joinPIDs(pids []int) string {
	var b strings.Builder
	for i, pid := range pids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%d", pid))
	}
	return b.String()
}
