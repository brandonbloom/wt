package cli

import (
	"context"
	"runtime/trace"
)

func withTraceRegion[T any](ctx context.Context, name string, fn func() (T, error)) (T, error) {
	var value T
	var err error
	trace.WithRegion(ctx, name, func() {
		value, err = fn()
	})
	return value, err
}

func withTraceRegionErr(ctx context.Context, name string, fn func() error) error {
	var err error
	trace.WithRegion(ctx, name, func() {
		err = fn()
	})
	return err
}
