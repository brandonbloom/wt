package cli

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/brandonbloom/wt/internal/processes"
	"github.com/brandonbloom/wt/internal/project"
)

var (
	listProcesses     = processes.List
	currentProcessPID = os.Getpid()
	parentProcessPID  = os.Getppid()
)

func attachProcessesToStatuses(statuses []*worktreeStatus, worktrees []project.Worktree) error {
	processMap, supported, err := detectWorktreeProcesses(worktrees)
	if err != nil {
		return err
	}
	if !supported {
		return nil
	}
	for _, status := range statuses {
		if procs := processMap[canonicalizePath(status.Path)]; len(procs) > 0 {
			status.Processes = append([]processes.Process(nil), procs...)
		}
	}
	return nil
}

func attachProcessesToCandidates(candidates []*tidyCandidate) error {
	worktrees := make([]project.Worktree, len(candidates))
	for i, cand := range candidates {
		worktrees[i] = cand.Worktree
	}
	processMap, supported, err := detectWorktreeProcesses(worktrees)
	if err != nil {
		return err
	}
	if !supported {
		return nil
	}
	for _, cand := range candidates {
		key := canonicalizePath(cand.Worktree.Path)
		updateCandidateProcesses(cand, processMap[key])
	}
	return nil
}

func detectWorktreeProcesses(worktrees []project.Worktree) (map[string][]processes.Process, bool, error) {
	procs, err := listProcesses()
	if errors.Is(err, processes.ErrUnsupported) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}

	canonicalRoots := make([]string, len(worktrees))
	for i, wt := range worktrees {
		canonicalRoots[i] = canonicalizePath(wt.Path)
	}

	result := make(map[string][]processes.Process, len(worktrees))
	for _, proc := range procs {
		cwd := normalizeProcessCWD(proc.CWD)
		if cwd == "" {
			continue
		}
		for _, root := range canonicalRoots {
			if root == "" {
				continue
			}
			if isWithin(cwd, root) {
				key := root
				result[key] = append(result[key], proc)
			}
		}
	}

	for key, group := range result {
		group = pruneProcessList(group)
		if len(group) == 0 {
			delete(result, key)
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			ci := processCommandLabel(group[i].Command)
			cj := processCommandLabel(group[j].Command)
			if ci == cj {
				return group[i].PID < group[j].PID
			}
			return ci < cj
		})
		result[key] = group
	}

	return result, true, nil
}

func canonicalizePath(path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func normalizeProcessCWD(path string) string {
	path = strings.TrimSpace(path)
	return canonicalizePath(path)
}

func pruneProcessList(procs []processes.Process) []processes.Process {
	if len(procs) == 0 {
		return procs
	}
	filtered := make([]processes.Process, 0, len(procs))
	for _, proc := range procs {
		if proc.PID == currentProcessPID || proc.PID == parentProcessPID {
			continue
		}
		filtered = append(filtered, proc)
	}
	if len(filtered) == 0 {
		return filtered
	}

	pidIndex := make(map[int]int, len(filtered))
	for i, proc := range filtered {
		pidIndex[proc.PID] = i
	}
	keep := make([]bool, len(filtered))
	for i := range keep {
		keep[i] = true
	}
	for i, proc := range filtered {
		if parentIdx, ok := pidIndex[proc.PPID]; ok {
			parent := filtered[parentIdx]
			if strings.EqualFold(processCommandLabel(parent.Command), processCommandLabel(proc.Command)) {
				keep[i] = false
			}
		}
	}

	out := make([]processes.Process, 0, len(filtered))
	for i, proc := range filtered {
		if keep[i] {
			out = append(out, proc)
		}
	}
	return out
}

func updateCandidateProcesses(cand *tidyCandidate, procs []processes.Process) {
	if cand == nil {
		return
	}
	removeProcessGrayReason(cand)
	if len(procs) == 0 {
		cand.Processes = nil
		return
	}
	cand.Processes = append([]processes.Process(nil), procs...)
	if summary := summarizeProcesses(procs, defaultProcessSummaryLimit); summary != "-" {
		cand.extraGrayReasons = append(cand.extraGrayReasons, "processes running: "+summary)
	}
}

func removeProcessGrayReason(cand *tidyCandidate) {
	if cand == nil || len(cand.extraGrayReasons) == 0 {
		return
	}
	filtered := cand.extraGrayReasons[:0]
	for _, reason := range cand.extraGrayReasons {
		if strings.HasPrefix(reason, "processes running:") {
			continue
		}
		filtered = append(filtered, reason)
	}
	cand.extraGrayReasons = filtered
}
