// Command rootcause is the RootCause CLI entry point.
//
// Phase 1 implements only the developer/CI subcommands needed to validate
// exercises. Phase 2 grows this into the full player CLI (init, start, shell,
// ticket, hint, check, reset, giveup, status, list; spec section 10).
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DIZAMU/RootCause/internal/exercise"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		os.Exit(cmdValidate(os.Args[2:]))
	case "vars":
		os.Exit(cmdVars(os.Args[2:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  rootcause validate <exercise-dir>
        Load and validate an exercise. Exit 0 if valid, 1 if not.

  rootcause vars --seed <n> <exercise-dir>
        Validate, then print the variant values chosen for that seed as
        RC_VAR_<NAME>=<value> lines, for a shell to eval.
`)
}

// cmdValidate loads and validates an exercise folder.
func cmdValidate(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "validate: expected exactly one exercise directory")
		return 2
	}
	ex, err := exercise.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("ok: %s\n", ex.ID)
	return 0
}

// cmdVars validates an exercise, then prints the variant choices for a seed.
func cmdVars(args []string) int {
	var seed int64
	var dir string
	seedSet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--seed":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "vars: --seed needs a value")
				return 2
			}
			i++
			if _, err := fmt.Sscan(args[i], &seed); err != nil {
				fmt.Fprintf(os.Stderr, "vars: --seed must be a number, got %q\n", args[i])
				return 2
			}
			seedSet = true
		default:
			dir = args[i]
		}
	}
	if dir == "" || !seedSet {
		fmt.Fprintln(os.Stderr, "vars: usage: rootcause vars --seed <n> <exercise-dir>")
		return 2
	}
	ex, err := exercise.Load(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for name, value := range ex.SelectVariant(seed) {
		fmt.Printf("RC_VAR_%s=%s\n", envName(name), shellQuote(value))
	}
	return 0
}

// envName turns a variant name into the RC_VAR_<NAME> suffix: upper case, with
// hyphens becoming underscores (site_name -> SITE_NAME).
func envName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// shellQuote single-quotes a value so a shell evals it literally.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
