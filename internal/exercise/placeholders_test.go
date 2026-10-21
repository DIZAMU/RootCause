package exercise

import (
	"reflect"
	"testing"
)

func TestUnfilledPlaceholders(t *testing.T) {
	var texts I18n
	texts.Ticket.Title = "The {site_name} is down"
	texts.Ticket.From = "Kari"
	texts.Ticket.Body = "Nobody can open {site_name}."
	texts.Hints = []string{"Look at the config.", "Run nginx -t", "Line {line} is broken."}
	texts.Walkthrough = "Fix line {line}; the site answers on {port}."
	texts.Feedback = map[string]string{"site_unreachable": "{site_name} does not answer on {prot}."}

	all := map[string]bool{"site_name": true, "port": true, "line": true}
	if got := UnfilledPlaceholders(&texts, all); !reflect.DeepEqual(got, []string{"prot"}) {
		t.Errorf("with every value available: got %v, want [prot] (the typo)", got)
	}

	noDerived := map[string]bool{"site_name": true, "port": true}
	if got := UnfilledPlaceholders(&texts, noDerived); !reflect.DeepEqual(got, []string{"line", "prot"}) {
		t.Errorf("without derived values: got %v, want [line prot]", got)
	}
}
