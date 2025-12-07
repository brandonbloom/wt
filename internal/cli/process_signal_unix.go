//go:build !windows

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func parseSignal(spec string) (syscall.Signal, error) {
	if strings.TrimSpace(spec) == "" {
		return 0, fmt.Errorf("missing signal")
	}
	if n, err := strconv.Atoi(spec); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("signal must be positive (got %d)", n)
		}
		return syscall.Signal(n), nil
	}
	name := strings.ToUpper(strings.TrimSpace(spec))
	if !strings.HasPrefix(name, "SIG") {
		name = "SIG" + name
	}
	if sig := unix.SignalNum(name); sig != 0 {
		return sig, nil
	}
	return 0, fmt.Errorf("unknown signal %q", spec)
}

func describeSignal(sig syscall.Signal) string {
	name := unix.SignalName(sig)
	if name == "" {
		return fmt.Sprintf("signal %d", sig)
	}
	return fmt.Sprintf("%s (%d)", name, sig)
}
