package grading

import "testing"

func TestStars(t *testing.T) {
	cases := map[int]int{0: 3, 1: 2, 2: 2, 3: 1, 4: 1}
	for hints, want := range cases {
		if got := Stars(hints); got != want {
			t.Errorf("Stars(%d) = %d, want %d", hints, got, want)
		}
	}
}

func s(n int) *int { return &n }

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		stars    *int
		hadCheck bool
		want     Kind
	}{
		{"3-star solve", "solved", s(3), false, Strong},
		{"2-star solve", "solved", s(2), false, Strong},
		{"1-star solve", "solved", s(1), false, Weak},
		{"gave up", "gave_up", nil, false, Fail},
		{"voided", "voided", nil, false, Skip},
		{"expired with no check", "expired", nil, false, Skip},
		{"expired after a check", "expired", nil, true, Fail},
	}
	for _, c := range cases {
		if got := Classify(c.status, c.stars, c.hadCheck); got != c.want {
			t.Errorf("%s: Classify = %v, want %v", c.name, got, c.want)
		}
	}
}

// Outcome builders: strong, weak and failed sessions on an exercise of
// difficulty d, and a skipped one.
func st(d int) Outcome { return Outcome{Kind: Strong, Difficulty: d} }
func wk(d int) Outcome { return Outcome{Kind: Weak, Difficulty: d} }
func fl(d int) Outcome { return Outcome{Kind: Fail, Difficulty: d} }
func sk() Outcome      { return Outcome{Kind: Skip, Difficulty: 1} }

func TestLevel(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []Outcome
		want     int
	}{
		{"start at 1", nil, 1},
		{"one strong is not enough", []Outcome{st(1)}, 1},
		{"two strong level up", []Outcome{st(1), st(1)}, 2},
		{"difficulty-2 solves raise level 2 to 3", []Outcome{st(1), st(1), st(2), st(2)}, 3},
		{"level stays capped at 3", []Outcome{st(1), st(1), st(2), st(2), st(3), st(3)}, 3},
		{"weak breaks the up streak", []Outcome{st(1), wk(1), st(1)}, 1},
		{"skip does not break the streak", []Outcome{st(1), sk(), st(1)}, 2},
		{"two fails do not go below 1", []Outcome{fl(1), fl(1)}, 1},
		{"up then down nets back", []Outcome{st(1), st(1), fl(2), fl(2)}, 1},
		{"fail breaks the up streak", []Outcome{st(1), fl(1), st(1)}, 1},

		// Difficulty rule (owner decision for spec 7.1).
		{"two difficulty-1 solves at level 2 do not level up",
			[]Outcome{st(1), st(1), st(1), st(1)}, 2},
		{"a below-level solve between two at-level solves is neutral",
			[]Outcome{st(1), st(1), st(2), st(1), st(2)}, 3},
		{"a below-level solve does not break a fail streak",
			[]Outcome{st(1), st(1), fl(2), st(1), fl(2)}, 1},
		{"fails at an easier difficulty still count",
			[]Outcome{st(1), st(1), st(2), st(2), fl(1), fl(1)}, 2},
		{"a harder exercise counts at the current level",
			[]Outcome{st(3), st(3)}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Level(tc.outcomes); got != tc.want {
				t.Errorf("Level(%v) = %d, want %d", tc.outcomes, got, tc.want)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	tests := []struct {
		name          string
		outcomes      []Outcome
		level, streak int
	}{
		{"nothing yet", nil, 1, 0},
		{"one strong at level 1", []Outcome{st(1)}, 1, 1},
		{"a level up resets the streak", []Outcome{st(1), st(1)}, 2, 0},
		{"one strong at level 2", []Outcome{st(1), st(1), st(2)}, 2, 1},
		{"a below-level solve does not count", []Outcome{st(1), st(1), st(1)}, 2, 0},
		{"a weak solve resets it", []Outcome{st(1), wk(1)}, 1, 0},
		{"a fail resets it", []Outcome{st(1), fl(1)}, 1, 0},
		{"no streak at the top level", []Outcome{st(1), st(1), st(2), st(2), st(3)}, 3, 0},
	}
	for _, tt := range tests {
		level, streak := Progress(tt.outcomes)
		if level != tt.level || streak != tt.streak {
			t.Errorf("%s: Progress = (%d, %d), want (%d, %d)", tt.name, level, streak, tt.level, tt.streak)
		}
		if l := Level(tt.outcomes); l != level {
			t.Errorf("%s: Level = %d, Progress level = %d", tt.name, l, level)
		}
	}
}
