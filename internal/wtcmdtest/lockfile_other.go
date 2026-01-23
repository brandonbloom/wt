//go:build !unix

// File locking for `wtcmdtest` on non-unix platforms.
//
// The repo's transcript suite runs on unix today; this is a no-op fallback.
package main

import (
	"context"
	"time"
)

func acquireLockFile(_ context.Context, _ string, _ time.Duration) (func(), error) {
	return func() {}, nil
}
