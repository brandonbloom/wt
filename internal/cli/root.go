package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"

	"github.com/brandonbloom/wt/internal/version"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func Execute() error {
	return newRootCommand().Execute()
}

type rootOptions struct {
	tracePath string
	traceFile *os.File
	traceTask *trace.Task
}

func newRootCommand() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:           "wt",
		Short:         "Brandon Bloom's experimental, opinionated, personal worktree manager.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return applyPreRunFlags(cmd, opts)
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			stopRuntimeTrace(opts)
			return nil
		},
		RunE: runStatus,
	}

	cmd.PersistentFlags().StringArrayP("directory", "C", nil, "change to directory before doing anything")
	cmd.PersistentFlags().StringVar(&opts.tracePath, "trace", "", "write a Go execution trace to file (relative to current dir after any earlier -C; view with `go tool trace` or Perfetto)")

	cmd.AddCommand(
		newVersionCommand(),
		newInitCommand(),
		newCloneCommand(),
		newNewCommand(),
		newBootstrapCommand(),
		newStatusCommand(),
		newActivateCommand(),
		newDoctorCommand(),
		newTidyCommand(),
		newRmCommand(),
		newKillCommand(),
	)

	return cmd
}

func startRuntimeTrace(cmd *cobra.Command, opts *rootOptions) error {
	if opts.traceFile != nil {
		return nil
	}

	f, err := os.Create(opts.tracePath)
	if err != nil {
		return fmt.Errorf("open trace file %q: %w", opts.tracePath, err)
	}
	if err := trace.Start(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("start trace: %w", err)
	}
	opts.traceFile = f

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, task := trace.NewTask(ctx, cmd.CommandPath())
	opts.traceTask = task
	cmd.SetContext(ctx)

	trace.Logf(ctx, "wt", "trace enabled: %s", opts.tracePath)

	return nil
}

func stopRuntimeTrace(opts *rootOptions) {
	if opts.traceFile == nil {
		return
	}

	if opts.traceTask != nil {
		opts.traceTask.End()
		opts.traceTask = nil
	}

	trace.Stop()
	_ = opts.traceFile.Close()
	opts.traceFile = nil
}

func applyPreRunFlags(cmd *cobra.Command, opts *rootOptions) error {
	flagSet := pflag.NewFlagSet("wt-prerun", pflag.ContinueOnError)
	flagSet.ParseErrorsAllowlist.UnknownFlags = true
	flagSet.SetInterspersed(true)
	flagSet.StringArrayP("directory", "C", nil, "")
	flagSet.String("trace", "", "")

	traceStarted := false
	err := flagSet.ParseAll(os.Args[1:], func(flag *pflag.Flag, value string) error {
		switch flag.Name {
		case "directory":
			if value == "" {
				return fmt.Errorf("chdir: empty directory")
			}
			if err := os.Chdir(value); err != nil {
				return fmt.Errorf("chdir to %q: %w", value, err)
			}
			return nil
		case "trace":
			if traceStarted {
				return fmt.Errorf("trace: multiple --trace flags are not supported")
			}
			if value == "" {
				return fmt.Errorf("trace: empty path")
			}
			traceStarted = true

			tracePath := value
			if !filepath.IsAbs(tracePath) {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				tracePath = filepath.Join(wd, tracePath)
			}
			opts.tracePath = tracePath
			if err := startRuntimeTrace(cmd, opts); err != nil {
				return err
			}
			return nil
		default:
			return nil
		}
	})
	return err
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the wt status dashboard",
		RunE:  runStatus,
	}
}
