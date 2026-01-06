package version

import (
	"runtime/debug"
	"strings"
)

func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}

	version := info.Main.Version
	if version == "" || version == "(devel)" {
		return "(devel)"
	}
	if strings.Contains(version, "+dirty") || isPseudoVersion(version) {
		return "(devel)"
	}
	return version
}

func isPseudoVersion(version string) bool {
	version, _, _ = strings.Cut(version, "+")

	parts := strings.Split(version, "-")
	if len(parts) < 3 {
		return false
	}

	ts := parts[len(parts)-2]
	hash := parts[len(parts)-1]
	if len(ts) != 14 || !allDigits(ts) {
		return false
	}
	if len(hash) < 12 || !allHex(hash) {
		return false
	}
	return true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func allHex(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= '0' && b <= '9' {
			continue
		}
		if b >= 'a' && b <= 'f' {
			continue
		}
		if b >= 'A' && b <= 'F' {
			continue
		}
		return false
	}
	return true
}
