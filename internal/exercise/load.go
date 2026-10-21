package exercise

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// requiredFiles must exist in every exercise folder (spec section 4.1).
var requiredFiles = []string{
	"Dockerfile",
	"break.sh",
	"check.sh",
	"solution.sh",
	filepath.Join("i18n", "en.yaml"),
	filepath.Join("i18n", "no.yaml"),
}

// languages are the language codes every exercise must provide (spec section 12).
var languages = []string{"en", "no"}

var (
	validTopics    = map[string]bool{"permissions": true, "services": true, "logs": true, "networking": true, "disk": true}
	validNetwork   = map[string]bool{"none": true, "internal": true}
	validRunsFrom  = map[string]bool{"sandbox": true, "runner": true}
	validBefore    = map[string]bool{"none": true, "restart": true}
	idPattern      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	memoryPattern  = regexp.MustCompile(`^[0-9]+(Mi|Gi)$`)
	capPattern     = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

// Load reads exercise.yaml from dir, then validates the schema, the required
// files and the two language files. It returns a fully validated Exercise or an
// error describing every problem found.
func Load(dir string) (*Exercise, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "exercise.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read exercise.yaml: %w", err)
	}

	var ex Exercise
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // reject unknown keys, so typos in the YAML fail loudly
	if err := dec.Decode(&ex); err != nil {
		return nil, fmt.Errorf("parse exercise.yaml: %w", err)
	}
	ex.dir = dir

	var problems []error
	if err := ex.validateSchema(); err != nil {
		problems = append(problems, err)
	}
	if err := ex.validateFiles(); err != nil {
		problems = append(problems, err)
	}
	if err := ex.validateI18n(); err != nil {
		problems = append(problems, err)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("exercise %q is invalid:\n%w", filepath.Base(dir), errors.Join(problems...))
	}
	return &ex, nil
}

// validateSchema checks the exercise.yaml fields alone, with no file access.
func (e *Exercise) validateSchema() error {
	var p []error
	add := func(format string, args ...any) { p = append(p, fmt.Errorf(format, args...)) }

	if !idPattern.MatchString(e.ID) {
		add("id %q must be lowercase words separated by single hyphens", e.ID)
	} else if e.dir != "" && e.ID != filepath.Base(e.dir) {
		add("id %q must match the folder name %q", e.ID, filepath.Base(e.dir))
	}
	if e.Version < 1 {
		add("version must be 1 or greater, got %d", e.Version)
	}
	if !validTopics[e.Topic] {
		add("topic %q must be one of permissions, services, logs, networking, disk", e.Topic)
	}
	if e.Difficulty < 1 || e.Difficulty > 3 {
		add("difficulty must be 1 to 3, got %d", e.Difficulty)
	}
	if e.EstimatedMinutes <= 0 {
		add("estimated_minutes must be greater than 0, got %d", e.EstimatedMinutes)
	}
	if !validNetwork[e.Network] {
		add("network %q must be none or internal", e.Network)
	}
	if !memoryPattern.MatchString(e.Resources.Memory) {
		add("resources.memory %q must look like 256Mi or 1Gi", e.Resources.Memory)
	}
	if e.Resources.CPUs <= 0 {
		add("resources.cpus must be greater than 0, got %v", e.Resources.CPUs)
	}
	if e.SessionTTLMin <= 0 {
		add("session_ttl_minutes must be greater than 0, got %d", e.SessionTTLMin)
	}
	if !validRunsFrom[e.Check.RunsFrom] {
		add("check.runs_from %q must be sandbox or runner", e.Check.RunsFrom)
	}
	if !validBefore[e.Check.Before] {
		add("check.before %q must be none or restart", e.Check.Before)
	}
	for name, values := range e.Variants {
		if len(values) == 0 {
			add("variant %q has no values", name)
		}
	}
	for _, c := range e.Capabilities {
		if !capPattern.MatchString(c) {
			add("capability %q must be an uppercase name like SETUID", c)
		}
	}
	return errors.Join(p...)
}

// validateFiles checks that every required file exists in the folder.
func (e *Exercise) validateFiles() error {
	if e.dir == "" {
		return nil
	}
	var p []error
	for _, name := range requiredFiles {
		if _, err := os.Stat(filepath.Join(e.dir, name)); err != nil {
			p = append(p, fmt.Errorf("missing required file %s", name))
		}
	}
	return errors.Join(p...)
}

// validateI18n loads both language files and checks that they define the same
// shape: the same ticket fields, the same number of hints, a walkthrough, and
// the same feedback keys (spec section 4.4).
func (e *Exercise) validateI18n() error {
	if e.dir == "" {
		return nil
	}
	texts := make(map[string]*I18n, len(languages))
	var p []error
	for _, lang := range languages {
		t, err := e.LoadI18n(lang)
		if err != nil {
			p = append(p, err)
			continue
		}
		texts[lang] = t
	}
	if len(texts) == len(languages) {
		if diff := shapeDiff(texts["en"], texts["no"]); diff != "" {
			p = append(p, fmt.Errorf("en and no text files differ: %s", diff))
		}
	}
	return errors.Join(p...)
}

// LoadI18n reads and parses one language file.
func (e *Exercise) LoadI18n(lang string) (*I18n, error) {
	path := filepath.Join(e.dir, "i18n", lang+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Join("i18n", lang+".yaml"), err)
	}
	var t I18n
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Join("i18n", lang+".yaml"), err)
	}
	return &t, nil
}

// shapeDiff returns a human-readable reason the two language files differ in
// structure, or "" if their shapes match.
func shapeDiff(a, b *I18n) string {
	if len(a.Hints) != len(b.Hints) {
		return fmt.Sprintf("en has %d hints, no has %d", len(a.Hints), len(b.Hints))
	}
	if keys := diffKeys(a.Feedback, b.Feedback); keys != "" {
		return "feedback keys: " + keys
	}
	return ""
}

// diffKeys reports the first key present in one map but not the other.
func diffKeys(a, b map[string]string) string {
	var missing []string
	for k := range a {
		if _, ok := b[k]; !ok {
			missing = append(missing, "no is missing "+k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			missing = append(missing, "en is missing "+k)
		}
	}
	sort.Strings(missing)
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", missing)
}
