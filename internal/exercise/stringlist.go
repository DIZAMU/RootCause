package exercise

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// StringList is a YAML sequence whose scalar values are all kept as strings,
// whatever type they were written as. Variant values like port: [8080, 8081]
// are numbers in YAML but become RC_VAR_PORT=8080 environment variables, so we
// normalise everything to a string at load time.
type StringList []string

// UnmarshalYAML accepts a sequence of scalars (string, int, float, bool) and
// stores each one's string form. Anything else (a map, a nested list) is an error.
func (s *StringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected a list, got %s", kindName(node.Kind))
	}
	out := make(StringList, 0, len(node.Content))
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			return fmt.Errorf("list values must be plain scalars, got %s", kindName(item.Kind))
		}
		out = append(out, item.Value)
	}
	*s = out
	return nil
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a map"
	case yaml.ScalarNode:
		return "a scalar"
	default:
		return "something unexpected"
	}
}
