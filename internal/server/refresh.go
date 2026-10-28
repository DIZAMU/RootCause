package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// refresher rebuilds the marts for the dashboard (plan decision D6). Builds
// run one at a time. A caller that arrives during a build waits for it and
// then checks freshness again, so a build that already covered the latest
// session is not repeated.
type refresher struct {
	ctx   context.Context // background builds run under it
	build func(ctx context.Context) error
	fresh func(ctx context.Context) (bool, error)

	sem     chan struct{} // holds one token while a build runs
	pending atomic.Int32  // callers waiting for or running a build
	wg      sync.WaitGroup

	mu      sync.Mutex
	lastErr error // the last build's error, nil after a success
}

func newRefresher(ctx context.Context, build func(context.Context) error, fresh func(context.Context) (bool, error)) *refresher {
	return &refresher{ctx: ctx, build: build, fresh: fresh, sem: make(chan struct{}, 1)}
}

// run rebuilds the marts unless they are already fresh. It returns the
// build's error, or ctx's error if ctx ends while it waits for another build.
func (r *refresher) run(ctx context.Context) error {
	r.pending.Add(1)
	defer r.pending.Add(-1)
	select {
	case r.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-r.sem }()

	if fresh, err := r.fresh(ctx); err == nil && fresh {
		return nil
	}
	return r.buildLocked(ctx)
}

// buildLocked runs the build and records its error. The caller holds sem.
func (r *refresher) buildLocked(ctx context.Context) error {
	err := r.build(ctx)
	r.mu.Lock()
	r.lastErr = err
	r.mu.Unlock()
	return err
}

// errBusy means a rebuild is already running or waiting to run.
var errBusy = errors.New("a statistics rebuild is already running")

// force starts a rebuild in the background even if the marts are fresh, the
// learner's "rebuild now". Only one runs at a time: if one is running or
// waiting, force starts nothing and returns errBusy.
func (r *refresher) force() error {
	select {
	case r.sem <- struct{}{}:
	default:
		return errBusy
	}
	r.pending.Add(1)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer r.pending.Add(-1)
		defer func() { <-r.sem }()
		_ = r.buildLocked(r.ctx)
	}()
	return nil
}

// trigger runs a rebuild in the background and returns at once.
func (r *refresher) trigger() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		_ = r.run(r.ctx)
	}()
}

// busy reports whether a rebuild is running or waiting to run.
func (r *refresher) busy() bool { return r.pending.Load() > 0 }

// err returns the last build's error, or nil.
func (r *refresher) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

// wait blocks until every background rebuild has returned. Cancel the
// refresher's context first to make them stop early.
func (r *refresher) wait() { r.wg.Wait() }
