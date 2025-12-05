package cli

import (
	"github.com/spf13/cobra"
)

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "wt",
		Short:         "Worktree swiss-army knife for multi-branch repositories",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runStatus,
	}

	cmd.AddCommand(
		newInitCommand(),
		newCloneCommand(),
		newNewCommand(),
		newActivateCommand(),
		newDoctorCommand(),
	)

	return cmd
}
