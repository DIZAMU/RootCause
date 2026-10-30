package exercise

import (
	"fmt"
	"hash/fnv"
	"sort"
)

// SelectVariant picks one concrete value for each variant from a seed
// (ARCHITECTURE.md section 3: a variant is "one concrete version of an exercise, chosen by
// a seed"). The same seed always yields the same choice, so a session can be
// reproduced exactly. The result maps variant name to the chosen value, ready to
// be passed to break.sh as RC_VAR_<NAME> environment variables.
//
// Each variant is chosen independently with its own hash of (seed, name), so
// adding or removing one variant does not shift the others' choices.
func (e *Exercise) SelectVariant(seed int64) map[string]string {
	chosen := make(map[string]string, len(e.Variants))
	for _, name := range e.variantNames() {
		values := e.Variants[name]
		idx := int(hashSeed(seed, name) % uint64(len(values)))
		chosen[name] = values[idx]
	}
	return chosen
}

// variantNames returns the variant names in a stable, sorted order.
func (e *Exercise) variantNames() []string {
	names := make([]string, 0, len(e.Variants))
	for name := range e.Variants {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// hashSeed hashes a seed and a variant name into a stable 64-bit number.
func hashSeed(seed int64, name string) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d:%s", seed, name)
	return h.Sum64()
}
