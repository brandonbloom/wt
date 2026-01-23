// wtcmdtest is a small internal harness for transcript tests.
//
// It provisions a disposable wt project under `/tmp/wt-transcripts/tmprepo-<id>`,
// installs a hermetic `gh` stub, optionally simulates the `wt` shell wrapper,
// then runs an arbitrary command inside the repo and returns the command's exit code.
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	tool, err := newToolFromExecutable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(tool.runCLI(context.Background(), os.Args[1:]))
}
