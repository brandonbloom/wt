package cli

import (
	"fmt"
	"os"

	"github.com/brandonbloom/wt/internal/version"
	"github.com/spf13/cobra"
)

func Execute() error {
	return newRootCommand().Execute()
}

type rootOptions struct {
	directories []string
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
			return applyDirectoryFlags(opts.directories)
		},
		RunE:          runStatus,
	}

	cmd.PersistentFlags().StringArrayVarP(&opts.directories, "directory", "C", nil, "change to directory before doing anything")

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

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the wt status dashboard",
		RunE:  runStatus,
	}
}

func applyDirectoryFlags(directories []string) error {
	for _, dir := range directories {
		if dir == "" {
			return fmt.Errorf("chdir: empty directory")
		}
		if err := os.Chdir(dir); err != nil {
			return fmt.Errorf("chdir to %q: %w", dir, err)
		}
	}
	return nil
}
