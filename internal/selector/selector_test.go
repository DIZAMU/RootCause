package selector

import (
	"errors"
	"maps"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func daysAgo(d int) time.Time { return now.Add(-time.Duration(d) * 24 * time.Hour) }

// catalog is a small catalog over three topics.
var catalog = []Exercise{
	{ID: "perm-01", Topic: "permissions", Difficulty: 1},
	{ID: "perm-02", Topic: "permissions", Difficulty: 2},
	{ID: "svc-01", Topic: "services", Difficulty: 1},
	{ID: "svc-02", Topic: "services", Difficulty: 2},
	{ID: "disk-01", Topic: "disk", Difficulty: 2},
}

func TestNext(t *testing.T) {
	cases := []struct {
		name       string
		in         Input
		wantID     string
		wantReason Reason
	}{
		{
			name: "new learner: all level 1, none practiced, first topic by name, closest difficulty",
			in:   Input{Exercises: catalog},
			// disk has only a difficulty-2 exercise, but topics tie on level and
			// practice, so the name decides: disk.
			wantID: "disk-01", wantReason: ReasonWeakest,
		},
		{
			name: "lowest level wins",
			in: Input{Exercises: catalog, Topics: map[string]TopicState{
				"disk": {Level: 2}, "permissions": {Level: 2}, "services": {Level: 1, LastPracticed: daysAgo(0)},
			}},
			wantID: "svc-01", wantReason: ReasonWeakest,
		},
		{
			name: "level tie: least recently practiced, never practiced first",
			in: Input{Exercises: catalog, Topics: map[string]TopicState{
				"disk":        {Level: 1, LastPracticed: daysAgo(1)},
				"permissions": {Level: 1, LastPracticed: daysAgo(5)},
				"services":    {Level: 1, LastPracticed: daysAgo(2)},
			}},
			wantID: "perm-01", wantReason: ReasonWeakest,
		},
		{
			name: "within the topic: difficulty closest to the level",
			in: Input{Exercises: catalog, Topics: map[string]TopicState{
				"disk": {Level: 3}, "services": {Level: 3}, "permissions": {Level: 2, LastPracticed: daysAgo(1)},
			}},
			wantID: "perm-02", wantReason: ReasonWeakest,
		},
		{
			name: "same difficulty distance: unseen first, then least recently attempted",
			in: Input{
				Exercises: []Exercise{
					{ID: "a", Topic: "logs", Difficulty: 1},
					{ID: "b", Topic: "logs", Difficulty: 1},
					{ID: "c", Topic: "logs", Difficulty: 1},
				},
				LastAttempt: map[string]time.Time{"a": daysAgo(1), "c": daysAgo(3)},
			},
			wantID: "b", wantReason: ReasonWeakest,
		},
		{
			name: "all seen: least recently attempted",
			in: Input{
				Exercises: []Exercise{
					{ID: "a", Topic: "logs", Difficulty: 1},
					{ID: "c", Topic: "logs", Difficulty: 1},
				},
				LastAttempt: map[string]time.Time{"a": daysAgo(1), "c": daysAgo(3)},
			},
			wantID: "c", wantReason: ReasonWeakest,
		},
		{
			name: "never the previous exercise",
			in:   Input{Exercises: catalog, Previous: "disk-01"},
			// disk has no candidate left, so the next topic by name: permissions.
			wantID: "perm-01", wantReason: ReasonWeakest,
		},
		{
			name: "review first, when due",
			in: Input{Exercises: catalog, Topics: map[string]TopicState{"services": {Level: 1}},
				Review: []Due{{ExerciseID: "svc-02", DueAt: daysAgo(1)}}},
			wantID: "svc-02", wantReason: ReasonReview,
		},
		{
			name:   "review exactly at its due time is due",
			in:     Input{Exercises: catalog, Review: []Due{{ExerciseID: "svc-02", DueAt: now}}},
			wantID: "svc-02", wantReason: ReasonReview,
		},
		{
			name:   "review not yet due is ignored",
			in:     Input{Exercises: catalog, Review: []Due{{ExerciseID: "svc-02", DueAt: now.Add(time.Minute)}}},
			wantID: "disk-01", wantReason: ReasonWeakest,
		},
		{
			name: "oldest due review first",
			in: Input{Exercises: catalog, Review: []Due{
				{ExerciseID: "svc-02", DueAt: daysAgo(1)},
				{ExerciseID: "perm-02", DueAt: daysAgo(2)},
			}},
			wantID: "perm-02", wantReason: ReasonReview,
		},
		{
			name: "rule 4 wins over review",
			in: Input{Exercises: catalog, Previous: "svc-02",
				Review: []Due{{ExerciseID: "svc-02", DueAt: daysAgo(1)}}},
			wantID: "disk-01", wantReason: ReasonWeakest,
		},
		{
			name:   "a review of an exercise not in the catalog is ignored",
			in:     Input{Exercises: catalog, Review: []Due{{ExerciseID: "gone-01", DueAt: daysAgo(1)}}},
			wantID: "disk-01", wantReason: ReasonWeakest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Next(c.in, now)
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if got.Exercise.ID != c.wantID || got.Reason != c.wantReason {
				t.Errorf("Next = %s (%s), want %s (%s)", got.Exercise.ID, got.Reason, c.wantID, c.wantReason)
			}
			if (got.Reason == ReasonReview) != (got.Due != nil) {
				t.Errorf("Due = %v for reason %s", got.Due, got.Reason)
			}
		})
	}
}

func TestNextNoCandidate(t *testing.T) {
	_, err := Next(Input{Exercises: catalog[:1], Previous: catalog[0].ID}, now)
	if !errors.Is(err, ErrNoCandidate) {
		t.Errorf("err = %v, want ErrNoCandidate", err)
	}
}

func TestReviewSeed(t *testing.T) {
	// Variant "port" is the seed modulo 3.
	variantOf := func(seed int64) map[string]string {
		return map[string]string{"port": []string{"80", "8080", "8443"}[seed%3]}
	}
	seq := func(seeds ...int64) func() int64 {
		return func() int64 { s := seeds[0]; seeds = seeds[1:]; return s }
	}
	last := Due{LastSeed: 4, LastVariant: variantOf(4)} // port 8080

	// Seeds 4 (the same seed) and 7 (same variant) are skipped; 5 differs.
	if got := ReviewSeed(variantOf, last, seq(4, 7, 5)); got != 5 {
		t.Errorf("ReviewSeed = %d, want 5", got)
	}
	if maps.Equal(variantOf(5), last.LastVariant) {
		t.Fatal("test setup: seed 5 should give a new variant")
	}

	// No variants: any seed but the last.
	none := func(int64) map[string]string { return map[string]string{} }
	seeds := make([]int64, 0, 22)
	for i := 0; i < 21; i++ {
		seeds = append(seeds, 4)
	}
	seeds = append(seeds, 9)
	if got := ReviewSeed(none, Due{LastSeed: 4, LastVariant: map[string]string{}}, seq(seeds...)); got != 9 {
		t.Errorf("ReviewSeed without variants = %d, want 9", got)
	}
}
