package exercise

import (
	"strings"
	"testing"
)

func TestLoadValidFixture(t *testing.T) {
	ex, err := Load("testdata/valid")
	if err != nil {
		t.Fatalf("Load returned an error for a valid exercise: %v", err)
	}
	if ex.ID != "valid" {
		t.Errorf("ID = %q, want %q", ex.ID, "valid")
	}
	if ex.Topic != "services" {
		t.Errorf("Topic = %q, want %q", ex.Topic, "services")
	}
	if got := ex.Variants["port"]; len(got) != 3 || got[0] != "8080" {
		t.Errorf("port variant = %v, want [8080 8081 8088] as strings", got)
	}
}

func TestValidateSchemaRejectsBadFields(t *testing.T) {
	good := func() *Exercise {
		return &Exercise{
			ID: "ex", Version: 1, Topic: "services", Difficulty: 1,
			EstimatedMinutes: 10, Network: "none",
			Resources:     Resources{Memory: "256Mi", CPUs: 0.5},
			SessionTTLMin: 60,
			Check:         Check{RunsFrom: "sandbox", Before: "none"},
		}
	}
	if err := good().validateSchema(); err != nil {
		t.Fatalf("baseline exercise should be valid, got: %v", err)
	}

	cases := []struct {
		name string
		want string
		mut  func(*Exercise)
	}{
		{"bad topic", "topic", func(e *Exercise) { e.Topic = "frobnication" }},
		{"difficulty too high", "difficulty", func(e *Exercise) { e.Difficulty = 4 }},
		{"zero version", "version", func(e *Exercise) { e.Version = 0 }},
		{"bad network", "network", func(e *Exercise) { e.Network = "host" }},
		{"bad memory", "memory", func(e *Exercise) { e.Resources.Memory = "256" }},
		{"bad runs_from", "runs_from", func(e *Exercise) { e.Check.RunsFrom = "nowhere" }},
		{"runner without internal network", "needs network internal", func(e *Exercise) { e.Check.RunsFrom = "runner" }},
		{"runner with systemd", "not supported", func(e *Exercise) {
			e.Init, e.Network, e.Check.RunsFrom = "systemd", "internal", "runner"
		}},
		{"bad init", "init", func(e *Exercise) { e.Init = "openrc" }},
		{"relative tmpfs", "absolute", func(e *Exercise) { e.Tmpfs = []Tmpfs{{Path: "srv/data", Size: "32Mi"}} }},
		{"root tmpfs", "absolute", func(e *Exercise) { e.Tmpfs = []Tmpfs{{Path: "/", Size: "32Mi"}} }},
		{"tmpfs size unit", "32Mi", func(e *Exercise) { e.Tmpfs = []Tmpfs{{Path: "/srv/data", Size: "32m"}} }},
		{"tmpfs over memory", "below resources.memory", func(e *Exercise) {
			e.Tmpfs = []Tmpfs{{Path: "/a", Size: "200Mi"}, {Path: "/b", Size: "56Mi"}}
		}},
		{"bad id", "id", func(e *Exercise) { e.ID = "Bad_ID" }},
		{"empty variant", "no values", func(e *Exercise) { e.Variants = map[string]StringList{"x": {}} }},
		{"disallowed capability", "not allowed", func(e *Exercise) { e.Capabilities = []string{"SYS_ADMIN"} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := good()
			c.mut(e)
			err := e.validateSchema()
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got none", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q should mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestAllowedCapabilityPasses(t *testing.T) {
	e := &Exercise{
		ID: "ex", Version: 1, Topic: "services", Difficulty: 1,
		EstimatedMinutes: 10, Network: "none",
		Resources:     Resources{Memory: "256Mi", CPUs: 0.5},
		SessionTTLMin: 60,
		Check:         Check{RunsFrom: "sandbox", Before: "none"},
		Capabilities:  []string{"SETUID", "SETGID", "CHOWN"},
	}
	if err := e.validateSchema(); err != nil {
		t.Fatalf("allowed capabilities rejected: %v", err)
	}
}

func TestCompletenessFindsGaps(t *testing.T) {
	var full I18n
	full.Ticket.Title, full.Ticket.From, full.Ticket.Body = "t", "f", "b"
	full.Walkthrough = "w"
	full.Hints = []string{"1", "2", "3"}
	if p := completeness("en", &full); len(p) != 0 {
		t.Fatalf("complete file reported problems: %v", p)
	}

	gaps := full
	gaps.Ticket.Title = " "
	gaps.Hints = []string{"1", "2"}
	p := completeness("no", &gaps)
	if len(p) != 2 {
		t.Fatalf("want 2 problems (empty title, 2 hints), got %d: %v", len(p), p)
	}
}

func TestSelectVariantIsDeterministic(t *testing.T) {
	ex, err := Load("testdata/valid")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const seed = int64(42)
	first := ex.SelectVariant(seed)
	second := ex.SelectVariant(seed)
	if len(first) != len(ex.Variants) {
		t.Fatalf("selected %d variants, want %d", len(first), len(ex.Variants))
	}
	for name, v := range first {
		if second[name] != v {
			t.Errorf("variant %q not deterministic: %q then %q", name, v, second[name])
		}
		if !contains(ex.Variants[name], v) {
			t.Errorf("variant %q chose %q, not in allowed values %v", name, v, ex.Variants[name])
		}
	}
}

func contains(list StringList, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
