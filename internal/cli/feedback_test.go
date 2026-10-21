package cli

import (
	"testing"

	"github.com/DIZAMU/RootCause/internal/exercise"
)

func TestFeedbackText(t *testing.T) {
	texts := &exercise.I18n{Feedback: map[string]string{
		"site_unreachable": "The {site_name} site does not respond.",
		"blank":            "   ",
	}}
	vars := map[string]string{"site_name": "wiki"}

	cases := map[string]string{
		"site_unreachable": "The wiki site does not respond.",
		"no_such_key":      "",
		"":                 "",
		"blank":            "",
	}
	for key, want := range cases {
		if got := feedbackText(texts, key, vars); got != want {
			t.Errorf("feedbackText(%q) = %q, want %q", key, got, want)
		}
	}
}
