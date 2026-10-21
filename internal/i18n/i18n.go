// Package i18n loads the interface strings from locales/<lang>.json and fills
// {placeholder} values. No user-facing text is hard-coded in Go (spec section
// 12); exercise texts live separately in each exercise's i18n folder.
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// placeholderPattern matches a {name} token. The name must follow the brace
// directly, so text like "server {" or {"line": 5} is not a placeholder.
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

// Placeholders returns the distinct {name} tokens in s, in order of first use.
func Placeholders(s string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, m := range placeholderPattern.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// Languages supported in v1.
var Languages = []string{"en", "no"}

// Bundle holds the interface strings for every language.
type Bundle struct {
	msgs map[string]map[string]string // lang -> key -> template
}

// Load reads locales/en.json and locales/no.json from dir.
func Load(dir string) (*Bundle, error) {
	b := &Bundle{msgs: make(map[string]map[string]string)}
	for _, lang := range Languages {
		path := filepath.Join(dir, lang+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		b.msgs[lang] = m
	}
	return b, nil
}

// T returns the message for key in lang, with placeholders filled. It falls
// back to English, then to the key itself, so a missing string is visible but
// never crashes.
func (b *Bundle) T(lang, key string, vars map[string]string) string {
	msg, ok := b.msgs[lang][key]
	if !ok {
		if msg, ok = b.msgs["en"][key]; !ok {
			return key
		}
	}
	return Fill(msg, vars)
}

// Keys returns the message keys for a language (used by CI parity checks).
func (b *Bundle) Keys(lang string) []string {
	keys := make([]string, 0, len(b.msgs[lang]))
	for k := range b.msgs[lang] {
		keys = append(keys, k)
	}
	return keys
}

// Fill replaces {name} tokens in template with vars[name]. Unknown tokens are
// left as-is.
func Fill(template string, vars map[string]string) string {
	if len(vars) == 0 {
		return template
	}
	pairs := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		pairs = append(pairs, "{"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(template)
}
