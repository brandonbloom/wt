package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the wt version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", root.DisplayName(), root.Version)
			return err
		},
	}
}
