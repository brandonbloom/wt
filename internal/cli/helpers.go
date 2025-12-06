package cli

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func isWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func currentTimeOverride() time.Time {
	if override := os.Getenv("WT_NOW"); override != "" {
		if t, err := time.Parse(time.RFC3339, override); err == nil {
			return t
		}
	}
	return time.Now()
}
