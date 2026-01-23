// Delay parsing for `WT_TEST_GH_DELAY`, used to simulate slow `gh pr list` calls.
package main

import (
	"strconv"
	"time"
)

func parseDelay(raw string) time.Duration {
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(float64(time.Second) * f)
	}
	return 0
}
