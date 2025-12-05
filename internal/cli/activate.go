package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newActivateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate",
		Short: "Print the shell wrapper that enables wt to change your cwd",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), wrapperScript)
			return nil
		},
	}
	return cmd
}

const wrapperScript = `# wt shell integration
wt() {
  local _wt_tmp
  _wt_tmp="$(mktemp "${TMPDIR:-/tmp}/wt.XXXXXX")" || return 1
  WT_WRAPPER_ACTIVE=1 WT_INSTRUCTION_FILE="$_wt_tmp" command wt "$@"
  local _wt_status=$?
  if [ -f "$_wt_tmp" ]; then
    local _wt_target
    _wt_target="$(cat "$_wt_tmp")"
    rm -f "$_wt_tmp"
    if [ $_wt_status -eq 0 ] && [ -n "$_wt_target" ]; then
      builtin cd "$_wt_target"
    fi
  fi
  return $_wt_status
}
`
