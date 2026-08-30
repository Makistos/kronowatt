// Package scheduler runs collectors on a timer. It knows nothing about any
// specific collector or storage — callers pass a Job closure and are
// responsible for their own error handling/health reporting, so one
// collector's failure mode can never be scheduler-level logic that affects
// another (spec §2.1: a broken collector must not degrade the rest of the
// system).
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type Job func(ctx context.Context) error

// Run executes fn immediately, then every interval, until ctx is
// cancelled. A failed run is logged and the loop continues — a transient
// error on one tick must not stop future collection attempts.
func Run(ctx context.Context, name string, interval time.Duration, fn Job) {
	logger := slog.Default().With("collector", name)

	runOnce := func() {
		if err := fn(ctx); err != nil {
			logger.Error("collector run failed", "error", err)
		}
	}

	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
