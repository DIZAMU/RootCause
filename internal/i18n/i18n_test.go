package i18n

import (
	"reflect"
	"testing"
)

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
