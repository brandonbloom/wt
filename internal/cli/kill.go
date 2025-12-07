package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/brandonbloom/wt/internal/processes"
	"github.com/brandonbloom/wt/internal/project"
	"github.com/spf13/cobra"
)

type killOptions struct {
	dryRun      bool
	signalFlag  string
	timeoutFlag string
	sig9        bool
}

func newKillCommand() *cobra.Command {
	opts := &killOptions{}
	cmd := &cobra.Command{
		Use:   "kill <worktrees...>",
		Short: "Terminate processes running inside worktrees",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(cmd, opts, args)
		},
	}
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "show which processes would be terminated")
	cmd.Flags().StringVarP(&opts.signalFlag, "signal", "s", "", "signal to send (numeric or name like TERM, HUP)")
	cmd.Flags().StringVar(&opts.timeoutFlag, "timeout", "", "time to wait for processes to exit (e.g. 3s)")
	cmd.Flags().BoolVarP(&opts.sig9, "sigkill", "9", false, "shorthand for --signal=9")
	_ = cmd.Flags().MarkHidden("sigkill")
	return cmd
}

func runKill(cmd *cobra.Command, opts *killOptions, args []string) error {
	proj, err := loadProjectFromWD()
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	worktrees, err := project.ListWorktrees(proj.Root)
	if err != nil {
		return err
	}
	targets, err := resolveWorktreeArgs(worktrees, args, wd)
	if err != nil {
		return err
	}

	signalSpec := opts.signalFlag
	if signalSpec == "" && opts.sig9 {
		signalSpec = "9"
	}
	settings, err := resolveKillSettings(signalSpec, opts.timeoutFlag, proj.Config.Process.KillTimeoutDuration())
	if err != nil {
		return err
	}

	processMap, supported, err := detectWorktreeProcesses(targets)
	if err != nil {
		return err
	}
	if !supported {
		return errors.New("process detection unsupported on this platform")
	}

	terminator := newProcessTerminator()
	out := cmd.OutOrStdout()
	var combined error

	for i, target := range targets {
		key := canonicalizePath(target.Path)
		procs := append([]processes.Process(nil), processMap[key]...)

		fmt.Fprintf(out, "%s:\n", target.Name)
		if len(procs) == 0 {
			fmt.Fprintln(out, "  nothing to kill")
			if i < len(targets)-1 {
				fmt.Fprintln(out)
			}
			continue
		}

		for _, proc := range procs {
			fmt.Fprintf(out, "  - %s (%d)\n", processCommandLabel(proc.Command), proc.PID)
		}
		action := fmt.Sprintf("%s to %d %s", settings.SignalLabel, len(procs), pluralizeProcess(len(procs)))
		if opts.dryRun {
			fmt.Fprintf(out, "  would send %s\n", action)
		} else {
			fmt.Fprintf(out, "  sending %s\n", action)
			if err := terminateWorktreeProcesses(cmd.Context(), target, procs, settings, terminator); err != nil {
				fmt.Fprintf(out, "  error: %s\n", singleLineError(err))
				combined = errors.Join(combined, fmt.Errorf("%s: %w", target.Name, err))
			} else {
				fmt.Fprintln(out, "  cleared")
			}
		}

		if i < len(targets)-1 {
			fmt.Fprintln(out)
		}
	}

	return combined
}

func pluralizeProcess(count int) string {
	if count == 1 {
		return "process"
	}
	return "processes"
}
