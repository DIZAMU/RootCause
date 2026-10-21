// Package grading turns session outcomes into stars and topic levels (spec
// sections 6 and 7.1). It is pure: no database, no I/O, so the rules are easy
// to read and test. The engine feeds it ordered outcomes and stores the result.
package grading

// Stars scores a solved session by how many hints were revealed (spec section
// 6): no hints = 3, hint 1 or 2 = 2, hint 3 = 1.
func Stars(hintsRevealed int) int {
	switch {
	case hintsRevealed == 0:
		return 3
	case hintsRevealed <= 2:
		return 2
	default:
		return 1
	}
}

// Kind is how one finished session counts toward the topic level.
type Kind int

const (
	Skip   Kind = iota // does not count (voided, or expired with no check)
	Weak               // solved with 1 star: breaks streaks but neither up nor down
	Strong             // solved with 2+ stars: counts toward leveling up
	Fail               // gave up, or expired after a check: counts toward leveling down
)

// Classify maps a finished session to its Kind (spec sections 6 and 7.1).
func Classify(status string, stars *int, hadCheck bool) Kind {
	switch status {
	case "solved":
		if stars != nil && *stars >= 2 {
			return Strong
		}
		return Weak
	case "gave_up":
		return Fail
	case "expired":
		if hadCheck {
			return Fail
		}
		return Skip
	default: // voided, or anything not finished
		return Skip
	}
}

// Outcome is one finished session as the level rule sees it.
type Outcome struct {
	Kind       Kind
	Difficulty int // the exercise's difficulty, 1 to 3
}

// Level replays a topic's finished outcomes (oldest first) and returns the
// resulting level, 1 to 3. Rule (spec 7.1): start at 1; two consecutive strong
// solves at the current level level up; two consecutive fails level down; a
// level change resets the streaks.
//
// "At the current level" means the exercise's difficulty is at least the
// current level. A strong solve of an easier exercise is neutral: like a
// skipped outcome, it neither advances nor resets either streak. Fails count at
// any difficulty.
func Level(outcomes []Outcome) int {
	level, up, down := 1, 0, 0
	for _, o := range outcomes {
		switch o.Kind {
		case Strong:
			if o.Difficulty < level {
				continue
			}
			up++
			down = 0
		case Weak:
			up, down = 0, 0
		case Fail:
			down++
			up = 0
		case Skip:
			continue
		}
		switch {
		case up >= 2 && level < 3:
			level++
			up, down = 0, 0
		case down >= 2 && level > 1:
			level--
			up, down = 0, 0
		}
	}
	return level
}
