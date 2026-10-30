package session

import (
	"context"
	"regexp"
	"time"

	"github.com/DIZAMU/RootCause/internal/store"
)

// Reaper destroys sandboxes whose session TTL has passed and marks those
// sessions expired (ARCHITECTURE.md section 8.4 step 8). In v1 it is run once at the start
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

// ReapOnce expires every session past its TTL, then reclaims orphaned
// sandboxes. It returns how many sessions it expired.
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
	if _, err := r.ReclaimOrphans(ctx); err != nil {
		return len(due), err
	}
	return len(due), nil
}

// sessionIDPattern matches the ids newID makes.
var sessionIDPattern = regexp.MustCompile(`^s-[0-9a-f]{16}$`)

// ReclaimOrphans destroys running sandboxes whose session is no longer
// active, for example after a crash between ending a session and destroying
// its sandbox. It returns how many it destroyed. It only touches sandboxes
// that this database started, labelled with a session id in the newID format,
// and never a selftest sandbox; anything it does not recognise is left alone.
func (r *Reaper) ReclaimOrphans(ctx context.Context) (int, error) {
	active, err := r.st.ActiveSessionIDs(ctx)
	if err != nil {
		return 0, err
	}
	boxes, err := r.eng.rt.List(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range boxes {
		id := b.Labels[labelSession]
		if b.Labels[labelSelftest] == "true" ||
			b.Labels[labelDatabase] != r.st.Database() ||
			!sessionIDPattern.MatchString(id) ||
			active[id] {
			continue
		}
		if err := r.eng.rt.Destroy(ctx, b.ID); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
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
