//go:build unix

// File locking for `wtcmdtest` on unix.
//
// This prevents concurrent transcript runs from clobbering `/tmp/wt-transcripts/tmprepo`.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

func acquireLockFile(ctx context.Context, path string, timeout time.Duration) (func(), error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return nil, err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				_ = f.Close()
			}, nil
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			if time.Now().After(deadline) {
				_ = f.Close()
				return nil, fmt.Errorf("wtcmdtest: waiting on lock %s: %w", path, context.DeadlineExceeded)
			}
			select {
			case <-ctx.Done():
				_ = f.Close()
				return nil, fmt.Errorf("wtcmdtest: waiting on lock %s: %w", path, ctx.Err())
			case <-time.After(25 * time.Millisecond):
			}
			continue
		}
		_ = f.Close()
		return nil, err
	}
}
