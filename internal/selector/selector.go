// Package selector picks the learner's next exercise (ARCHITECTURE.md section 7.2). Next
// is a pure function of its Input; Load gathers that input from the dbt marts,
// or from the source tables when the marts are missing or stale (ARCHITECTURE.md section
// 9.3).
package selector

import (
	"errors"
	"maps"
	"sort"
	"time"
)

// Exercise is one exercise the selector may pick.
type Exercise struct {
	ID         string
	Topic      string
	Difficulty int
}

// TopicState is the learner's standing in one topic.
type TopicState struct {
	Level         int
	LastPracticed time.Time // zero if never practiced
}

// Due is one exercise in the review queue.
type Due struct {
	ExerciseID  string
	DueAt       time.Time
	LastSeed    int64
	LastVariant map[string]string
}

// Input is everything Next needs.
type Input struct {
	Exercises   []Exercise
	Topics      map[string]TopicState // missing topics are at level 1, never practiced
	Review      []Due                 // empty when the marts are not used
	LastAttempt map[string]time.Time  // per exercise: start of its latest session that was not voided
	Previous    string                // exercise of the learner's most recent session, "" if none
}

// Reason says which rule of ARCHITECTURE.md section 7.2 made the pick.
type Reason string

const (
	ReasonReview  Reason = "review"        // rule 1
	ReasonWeakest Reason = "weakest_topic" // rules 2 and 3
)

// Pick is the selector's choice.
type Pick struct {
	Exercise Exercise
	Reason   Reason
	Due      *Due // set for a review pick
}

// ErrNoCandidate means every exercise is ruled out, which happens only when
// the catalog holds nothing but the previous exercise.
var ErrNoCandidate = errors.New("no exercise to pick")

// Next applies ARCHITECTURE.md section 7.2 in order:
//
//  1. Review first: the due exercise with the oldest due time, ties by id.
//  2. Otherwise the weakest topic: lowest level, ties broken by least recently
//     practiced (never practiced first), then by topic name.
//  3. Within the topic: difficulty closest to the level, then unseen first,
//     then least recently attempted, then by id.
//  4. Never the previous exercise, in any rule.
func Next(in Input, now time.Time) (Pick, error) {
	catalog := make(map[string]Exercise, len(in.Exercises))
	for _, ex := range in.Exercises {
		if ex.ID != in.Previous {
			catalog[ex.ID] = ex
		}
	}
	if len(catalog) == 0 {
		return Pick{}, ErrNoCandidate
	}

	// Rule 1.
	var due []Due
	for _, d := range in.Review {
		if _, ok := catalog[d.ExerciseID]; ok && !d.DueAt.After(now) {
			due = append(due, d)
		}
	}
	if len(due) > 0 {
		sort.Slice(due, func(i, j int) bool {
			if !due[i].DueAt.Equal(due[j].DueAt) {
				return due[i].DueAt.Before(due[j].DueAt)
			}
			return due[i].ExerciseID < due[j].ExerciseID
		})
		d := due[0]
		return Pick{Exercise: catalog[d.ExerciseID], Reason: ReasonReview, Due: &d}, nil
	}

	// Rule 2, over the topics that still have a candidate.
	byTopic := make(map[string][]Exercise)
	for _, ex := range catalog {
		byTopic[ex.Topic] = append(byTopic[ex.Topic], ex)
	}
	state := func(topic string) TopicState {
		s, ok := in.Topics[topic]
		if !ok || s.Level == 0 {
			s.Level = 1
		}
		return s
	}
	topics := sortedKeys(byTopic)
	sort.SliceStable(topics, func(i, j int) bool {
		a, b := state(topics[i]), state(topics[j])
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.LastPracticed.Before(b.LastPracticed)
	})
	topic := topics[0]
	level := state(topic).Level

	// Rule 3.
	cands := byTopic[topic]
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if da, db := distance(a.Difficulty, level), distance(b.Difficulty, level); da != db {
			return da < db
		}
		ta, seenA := in.LastAttempt[a.ID]
		tb, seenB := in.LastAttempt[b.ID]
		if seenA != seenB {
			return !seenA
		}
		if !ta.Equal(tb) {
			return ta.Before(tb)
		}
		return a.ID < b.ID
	})
	return Pick{Exercise: cands[0], Reason: ReasonWeakest}, nil
}

// variantTries bounds the search for a seed with a new variant.
const variantTries = 20

// ReviewSeed returns a seed for a review pick, so the exercise "returns as a
// new variant" (ARCHITECTURE.md section 7.2): the first of up to 20 random seeds whose
// variant values differ from the last session's. If none does (the exercise
// has no variants, or only one), it returns a random seed that differs from
// the last one. variantOf is the exercise's own variant selection.
func ReviewSeed(variantOf func(seed int64) map[string]string, d Due, random func() int64) int64 {
	seed := random()
	for i := 0; i < variantTries; i++ {
		if seed != d.LastSeed && !maps.Equal(variantOf(seed), d.LastVariant) {
			return seed
		}
		seed = random()
	}
	for seed == d.LastSeed {
		seed = random()
	}
	return seed
}

func distance(difficulty, level int) int {
	if difficulty > level {
		return difficulty - level
	}
	return level - difficulty
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
