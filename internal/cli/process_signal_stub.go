//go:build windows

package cli

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
)

func parseSignal(spec string) (syscall.Signal, error) {
	if strings.TrimSpace(spec) == "" {
		return 0, fmt.Errorf("missing signal")
	}
	if n, err := strconv.Atoi(spec); err == nil && n > 0 {
		return syscall.Signal(n), nil
	}
	return 0, fmt.Errorf("signal names unsupported on this platform; use a numeric value")
}

func describeSignal(sig syscall.Signal) string {
	return fmt.Sprintf("signal %d", sig)
}
