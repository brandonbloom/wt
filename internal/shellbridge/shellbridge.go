package shellbridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	envWrapper         = "WT_WRAPPER_ACTIVE"
	envInstructionFile = "WT_INSTRUCTION_FILE"
)

var (
	// ErrWrapperMissing indicates the shell function wrapper is not active.
	ErrWrapperMissing = errors.New("shell wrapper missing; add `eval \"$(wt activate)\"` to your shell rc")
)

// Active reports whether the shell wrapper marked itself as active.
func Active() bool {
	return os.Getenv(envWrapper) == "1"
}

// InstructionFile returns the path provided by the wrapper for directives.
func InstructionFile() string {
	return os.Getenv(envInstructionFile)
}

// Require ensures the wrapper is active, returning a helpful error otherwise.
func Require(feature string) error {
	if Active() && InstructionFile() != "" {
		return nil
	}
	if feature == "" {
		feature = "this command"
	}
	return fmt.Errorf("%s requires the wt shell wrapper; run `eval \"$(wt activate)\"` in your shell rc", feature)
}

// ChangeDirectory requests that the wrapper cd into the provided path.
func ChangeDirectory(path string) error {
	if err := Require("changing directories"); err != nil {
		return err
	}

	file := InstructionFile()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}

	return os.WriteFile(file, []byte(path), 0o644)
}
