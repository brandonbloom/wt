package cli

import (
	"github.com/brandonbloom/wt/internal/version"
	"github.com/spf13/cobra"
)

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wt",
		Short:         "Brandon Bloom's experimental, opinionated, personal worktree manager.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.String(),
		RunE:          runStatus,
	}

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
