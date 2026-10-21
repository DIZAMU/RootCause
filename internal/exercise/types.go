// Package exercise loads and validates RootCause exercise definitions.
//
// An exercise is a folder (see spec section 4.1) holding an exercise.yaml, the
// scripts that build, break, check and solve a machine, and per-language text
// files. Load reads that folder, checks it against the schema, and returns a
// validated Exercise. The CLI, CI and the validate script all go through here,
// so the rules live in one place.
package exercise

// Exercise mirrors exercise.yaml (spec section 4.2).
type Exercise struct {
	ID               string    `yaml:"id"`
	Version          int       `yaml:"version"`
	Topic            string    `yaml:"topic"`
	Difficulty       int       `yaml:"difficulty"`
	EstimatedMinutes int       `yaml:"estimated_minutes"`
	Network          string    `yaml:"network"`
	Resources        Resources `yaml:"resources"`
	SessionTTLMin    int       `yaml:"session_ttl_minutes"`
	Check            Check     `yaml:"check"`

	// Capabilities are the Linux capabilities the sandbox adds back after
	// dropping all of them (spec section 13.1). Empty means none are needed.
	Capabilities []string `yaml:"capabilities"`

	// Variants maps a variant name (e.g. "port") to its allowed values.
	// Values may be written as strings or numbers in YAML; both are stored as
	// strings, because they end up as RC_VAR_<NAME> environment variables.
	Variants map[string]StringList `yaml:"variants"`

	// dir is the folder this exercise was loaded from. Not from YAML.
	dir string `yaml:"-"`
}

// Resources is the sandbox resource budget (spec section 17).
type Resources struct {
	Memory string  `yaml:"memory"`
	CPUs   float64 `yaml:"cpus"`
}

// Check controls how and when the grader runs (spec section 4.2).
type Check struct {
	RunsFrom string `yaml:"runs_from"` // sandbox | runner
	Before   string `yaml:"before"`    // none | restart
}

// Dir returns the folder the exercise was loaded from.
func (e *Exercise) Dir() string { return e.dir }

// I18n mirrors one language file, i18n/<lang>.yaml (spec section 4.4).
type I18n struct {
	Ticket struct {
		Title string `yaml:"title"`
		From  string `yaml:"from"`
		Body  string `yaml:"body"`
	} `yaml:"ticket"`
	Hints       []string          `yaml:"hints"`
	Walkthrough string            `yaml:"walkthrough"`
	Feedback    map[string]string `yaml:"feedback"`
}
