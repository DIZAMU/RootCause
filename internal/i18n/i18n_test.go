package i18n

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/DIZAMU/RootCause/locales"
)

// TestLocaleParity fails if a key exists in one language and not the other,
// or if any string is empty (ARCHITECTURE.md section 12).
func TestLocaleParity(t *testing.T) {
	b, err := Load(locales.FS)
	if err != nil {
		t.Fatalf("load locales: %v", err)
	}
	en, no := b.Keys("en"), b.Keys("no")
	sort.Strings(en)
	sort.Strings(no)
	if !reflect.DeepEqual(en, no) {
		t.Errorf("en and no keys differ:\n  only en: %v\n  only no: %v", minus(en, no), minus(no, en))
	}
	for _, lang := range Languages {
		for _, k := range b.Keys(lang) {
			if strings.TrimSpace(b.T(lang, k, nil)) == "" {
				t.Errorf("%s: %s is empty", lang, k)
			}
		}
	}
}

// minus returns the items in a that are not in b.
func minus(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, s := range b {
		in[s] = true
	}
	var out []string
	for _, s := range a {
		if !in[s] {
			out = append(out, s)
		}
	}
	return out
}

func TestPlaceholders(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"no placeholders here", nil},
		{"Line {line} of {site_name}", []string{"line", "site_name"}},
		{"{port} and {port} again", []string{"port"}},
		{"server { listen 80; }", nil},
		{`prints {"line": 5}`, nil},
	}
	for _, c := range cases {
		if got := Placeholders(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Placeholders(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
