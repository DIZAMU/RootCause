package selector

import (
	"context"
	"fmt"

	"github.com/DIZAMU/RootCause/internal/store"
)

// Source says where Load took the levels and the review queue from.
type Source string

const (
	SourceMarts    Source = "marts"    // the dbt marts
	SourceFallback Source = "fallback" // topic levels from the engine, no review (ARCHITECTURE.md section 9.3)
)

// Deps are what Load reads from.
type Deps struct {
	Store *store.Store
	// Level is the engine's topic level (ARCHITECTURE.md section 7.1), used when the
	// marts are missing or stale.
	Level func(ctx context.Context, learnerID int, topic string) (int, error)
	// Refresh rebuilds the marts (runs dbt). It is called at most once, when
	// the marts are missing or stale. nil means never refresh.
	Refresh func(ctx context.Context) error
}

// Loaded is the input Load gathered, and how.
type Loaded struct {
	Input      Input
	Source     Source
	RefreshErr error // set if a refresh was tried and failed
}

// Load gathers the selector's input for a learner. If the marts are missing
// or stale it refreshes them once; if they are still not fresh it falls back
// to the engine's topic levels and skips the review queue, so the learner
// always gets a pick.
func Load(ctx context.Context, d Deps, learnerID int, exercises []Exercise) (Loaded, error) {
	var out Loaded
	fresh, err := d.Store.MartsFresh(ctx, learnerID)
	if err != nil {
		return out, err
	}
	if !fresh && d.Refresh != nil {
		if out.RefreshErr = d.Refresh(ctx); out.RefreshErr == nil {
			if fresh, err = d.Store.MartsFresh(ctx, learnerID); err != nil {
				return out, err
			}
		}
	}

	in := Input{Exercises: exercises, Topics: make(map[string]TopicState)}
	if in.LastAttempt, in.Previous, err = d.Store.AttemptHistory(ctx, learnerID); err != nil {
		return out, err
	}

	if fresh {
		levels, err := d.Store.MartTopicLevels(ctx, learnerID)
		if err != nil {
			return out, err
		}
		for topic, tl := range levels {
			ts := TopicState{Level: tl.Level}
			if tl.LastPracticed != nil {
				ts.LastPracticed = *tl.LastPracticed
			}
			in.Topics[topic] = ts
		}
		queue, err := d.Store.MartReviewQueue(ctx, learnerID)
		if err != nil {
			return out, err
		}
		for _, r := range queue {
			in.Review = append(in.Review, Due{
				ExerciseID: r.ExerciseID, DueAt: r.DueAt, LastSeed: r.LastSeed, LastVariant: r.LastVariant,
			})
		}
		out.Input, out.Source = in, SourceMarts
		return out, nil
	}

	practiced, err := d.Store.LastPracticed(ctx, learnerID)
	if err != nil {
		return out, err
	}
	for _, ex := range exercises {
		if _, done := in.Topics[ex.Topic]; done {
			continue
		}
		level, err := d.Level(ctx, learnerID, ex.Topic)
		if err != nil {
			return out, fmt.Errorf("topic level %s: %w", ex.Topic, err)
		}
		in.Topics[ex.Topic] = TopicState{Level: level, LastPracticed: practiced[ex.Topic]}
	}
	out.Input, out.Source = in, SourceFallback
	return out, nil
}
