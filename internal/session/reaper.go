package session

import (
	"context"
	"time"

	"github.com/DIZAMU/RootCause/internal/store"
)

// Reaper destroys sandboxes whose session TTL has passed and marks those
// sessions expired (spec section 8.4 step 8). In v1 it is run once at the start
// of each CLI command and, when the dashboard server exists, on a ticker.
type Reaper struct {
	eng      *Engine
	st       *store.Store
	interval time.Duration
}

// NewReaper builds a reaper. interval is only used by Run.
func NewReaper(eng *Engine, st *store.Store, interval time.Duration) *Reaper {
	return &Reaper{eng: eng, st: st, interval: interval}
}

// ReapOnce expires every session past its TTL and returns how many it expired.
func (r *Reaper) ReapOnce(ctx context.Context) (int, error) {
	due, err := r.st.DueSessions(ctx)
	if err != nil {
		return 0, err
	}
	for _, sess := range due {
		if _, err := r.eng.Expire(ctx, sess); err != nil {
			return 0, err
		}
	}
	return len(due), nil
}

// Run reaps on a ticker until the context is cancelled.
func (r *Reaper) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = r.ReapOnce(ctx)
		}
	}
}
