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

func TestLevel(t *testing.T) {
	strong := Classify("solved", s(3), false)
	weak := Classify("solved", s(1), false)
	fail := Classify("gave_up", nil, false)
	skip := Classify("voided", nil, false)
	expiredNoCheck := Classify("expired", nil, false)
	expiredCheck := Classify("expired", nil, true)

	if strong != Strong || weak != Weak || fail != Fail || skip != Skip {
		t.Fatalf("classify basics wrong: %v %v %v %v", strong, weak, fail, skip)
	}
	if expiredNoCheck != Skip {
		t.Errorf("expired with no check should Skip")
	}
	if expiredCheck != Fail {
		t.Errorf("expired after a check should be a Fail")
	}

	tests := []struct {
		name  string
		kinds []Kind
		want  int
	}{
		{"start at 1", nil, 1},
		{"one strong is not enough", []Kind{Strong}, 1},
		{"two strong level up", []Kind{Strong, Strong}, 2},
		{"four strong cap at 3", []Kind{Strong, Strong, Strong, Strong}, 3},
		{"five strong still cap at 3", []Kind{Strong, Strong, Strong, Strong, Strong}, 3},
		{"weak breaks the up streak", []Kind{Strong, Weak, Strong}, 1},
		{"skip does not break the streak", []Kind{Strong, Skip, Strong}, 2},
		{"two fails do not go below 1", []Kind{Fail, Fail}, 1},
		{"up then down nets back", []Kind{Strong, Strong, Fail, Fail}, 1},
		{"fail breaks up streak", []Kind{Strong, Fail, Strong}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Level(tc.kinds); got != tc.want {
				t.Errorf("Level(%v) = %d, want %d", tc.kinds, got, tc.want)
			}
		})
	}
}
