package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/brandonbloom/wt/internal/processes"
	"github.com/brandonbloom/wt/internal/project"
)

var (
	defaultKillSignal     = syscall.Signal(syscall.SIGTERM)
	errProcessUnsupported = errors.New("process detection unsupported on this platform")
)

type killSettings struct {
	Signal      syscall.Signal
	SignalLabel string
	Timeout     time.Duration
}

func resolveKillSettings(signalSpec string, timeoutSpec string, defaultTimeout time.Duration) (killSettings, error) {
	sig := defaultKillSignal
	label := describeSignal(sig)
	if signalSpec != "" && signalSpec != "true" {
		parsed, err := parseSignal(signalSpec)
		if err != nil {
			return killSettings{}, err
		}
		sig = parsed
		label = describeSignal(sig)
	}

	timeout := defaultTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	if strings.TrimSpace(timeoutSpec) != "" {
		dur, err := time.ParseDuration(timeoutSpec)
		if err != nil {
			return killSettings{}, fmt.Errorf("invalid --timeout value %q (examples: 1s, 500ms)", timeoutSpec)
		}
		if dur <= 0 {
			return killSettings{}, fmt.Errorf("timeout must be positive")
		}
		timeout = dur
	}

	return killSettings{
		Signal:      sig,
		SignalLabel: label,
		Timeout:     timeout,
	}, nil
}

type processTerminator interface {
	Terminate(proc processes.Process, sig syscall.Signal) error
}

type realProcessTerminator struct{}

func (realProcessTerminator) Terminate(proc processes.Process, sig syscall.Signal) error {
	p, err := os.FindProcess(proc.PID)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

type testProcessTerminator struct {
	path string
	mu   sync.Mutex
}

func (t *testProcessTerminator) Terminate(proc processes.Process, sig syscall.Signal) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := os.ReadFile(t.path)
	if err != nil {
		return err
	}
	var procs []processes.Process
	if len(data) > 0 {
		if err := json.Unmarshal(data, &procs); err != nil {
			return err
		}
	}
	filtered := make([]processes.Process, 0, len(procs))
	for _, p := range procs {
		if p.PID == proc.PID {
			continue
		}
		filtered = append(filtered, p)
	}
	updated, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, updated, 0o644)
}

func newProcessTerminator() processTerminator {
	if path := processes.TestDataFilePath(); path != "" {
		return &testProcessTerminator{path: path}
	}
	return realProcessTerminator{}
}

func terminateWorktreeProcesses(ctx context.Context, wt project.Worktree, procs []processes.Process, settings killSettings, term processTerminator) error {
	var errs error
	for _, proc := range procs {
		if err := term.Terminate(proc, settings.Signal); err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s (%d): %w", processCommandLabel(proc.Command), proc.PID, err))
		}
	}
	if errs != nil {
		return errs
	}
	remaining, err := waitForProcessExit(ctx, wt, settings.Timeout)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		summary := summarizeProcesses(remaining, defaultProcessSummaryLimit)
		if summary == "-" {
			summary = fmt.Sprintf("%d process(es)", len(remaining))
		}
		return fmt.Errorf("processes still running after %s: %s", settings.Timeout, summary)
	}
	return nil
}

func waitForProcessExit(ctx context.Context, wt project.Worktree, timeout time.Duration) ([]processes.Process, error) {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		current, supported, err := detectWorktreeProcesses([]project.Worktree{wt})
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, errProcessUnsupported
		}
		list := current[canonicalizePath(wt.Path)]
		if len(list) == 0 {
			return nil, nil
		}
		if time.Now().After(deadline) {
			return list, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}
