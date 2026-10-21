package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DIZAMU/RootCause/internal/i18n"
	"github.com/DIZAMU/RootCause/internal/session"
	"github.com/DIZAMU/RootCause/locales"
)

func loadBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b, err := i18n.Load(locales.FS)
	if err != nil {
		t.Fatalf("load locales: %v", err)
	}
	return b
}

func TestMessage(t *testing.T) {
	b := loadBundle(t)
	cases := []struct {
		name string
		lang string
		err  error
		want []string // substrings the message must contain
	}{
		{"known key, en", "en", fail("error.no_root", nil), []string{"Cannot find the RootCause folder"}},
		{"known key, no", "no", fail("error.no_root", nil), []string{"Finner ikke RootCause-mappen"}},
		{"cause becomes details", "en", fail("error.compose_failed", errors.New("exit status 1")),
			[]string{"Could not start Postgres", "Details: exit status 1"}},
		{"key with values", "en", failWith("start.unknown_exercise", map[string]string{"id": "svc-99"}, nil),
			[]string{"svc-99", "rootcause list"}},
		{"wrapped engine sentinel", "en", fmt.Errorf("give up: %w", session.ErrGiveUpTooEarly),
			[]string{"hint 3"}},
		{"unknown error, generic heading", "no", errors.New("boom"), []string{"Noe gikk galt: boom"}},
	}
	for _, c := range cases {
		got := Message(b, c.lang, c.err)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: Message = %q, want it to contain %q", c.name, got, w)
			}
		}
	}
}

// TestUsedKeysExist fails if this package asks for a locale key that does not
// exist. Bundle.T falls back to printing the raw key, so a typo would
// otherwise only show up on screen.
func TestUsedKeysExist(t *testing.T) {
	b := loadBundle(t)
	known := make(map[string]bool)
	for _, k := range b.Keys("en") {
		known[k] = true
	}
	calls := regexp.MustCompile(`(?:\.say|\.t|\bfail|\bfailWith)\("([a-z_]+\.[a-z_]+)"|\.T\([^,()]+, "([a-z_]+\.[a-z_]+)"`)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range calls.FindAllStringSubmatch(string(src), -1) {
			key := m[1] + m[2]
			found++
			if !known[key] {
				t.Errorf("%s uses locale key %q, which is not in locales/en.json", f, key)
			}
		}
	}
	if found < 20 {
		t.Errorf("found only %d key uses; the pattern has probably stopped matching", found)
	}
}
