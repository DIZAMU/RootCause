// Command rootcause is the RootCause CLI (spec section 10).
//
// Phase 2 implements the local play loop on Docker: init, start, shell, ticket,
// hint, check, reset, giveup, status, list, lang. The developer commands
// validate and vars need no database. Later phases add next (selector) and
// serve (dashboard).
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DIZAMU/RootCause/internal/cli"
	"github.com/DIZAMU/RootCause/internal/exercise"
	"github.com/DIZAMU/RootCause/internal/runtime/docker"
	"github.com/DIZAMU/RootCause/internal/session"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, cli.Usage("en")) // no profile is loaded yet
		os.Exit(2)
	}
	ctx := context.Background()
	cmd, args := os.Args[1], os.Args[2:]

	switch cmd {
	// Developer/CI commands: no database needed.
	case "validate":
		os.Exit(cmdValidate(args))
	case "vars":
		os.Exit(cmdVars(args))
	case "selftest":
		os.Exit(cmdSelfTest(ctx, args))
	case "init":
		os.Exit(cmdInit(ctx))
	default:
		os.Exit(runApp(ctx, cmd, args))
	}
}

// runApp handles the database-backed play commands.
func runApp(ctx context.Context, cmd string, args []string) int {
	app, err := cli.NewApp(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer app.Close()

	var cmdErr error
	switch cmd {
	case "start":
		cmdErr = app.Start(ctx, args)
	case "ticket":
		cmdErr = app.Ticket(ctx)
	case "shell":
		cmdErr = app.Shell(ctx)
	case "hint":
		cmdErr = app.Hint(ctx)
	case "check":
		cmdErr = app.Check(ctx)
	case "reset":
		cmdErr = app.Reset(ctx)
	case "giveup":
		cmdErr = app.GiveUp(ctx)
	case "status":
		cmdErr = app.Status(ctx)
	case "list":
		cmdErr = app.List(ctx)
	case "lang":
		cmdErr = app.Lang(ctx, args)
	default:
		fmt.Fprint(os.Stderr, app.UnknownCommand(cmd))
		return 2
	}
	if cmdErr != nil {
		fmt.Fprintln(os.Stderr, app.Describe(cmdErr))
		return 1
	}
	return 0
}

func cmdInit(ctx context.Context) int {
	msg, _, err := cli.Init(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(msg)
	return 0
}

// --- developer commands (no database) --------------------------------------

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

// cmdSelfTest runs an exercise's unsolved and solved scenarios in real
// sandboxes, with the same flags a session uses. It needs Docker, not the
// database. scripts/validate-exercise.sh calls it.
func cmdSelfTest(ctx context.Context, args []string) int {
	seed, dir, ok := seedAndDir("selftest", args)
	if !ok {
		return 2
	}
	rt, err := docker.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := session.SelfTest(ctx, rt, rt, dir, seed, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		return 1
	}
	return 0
}

func cmdVars(args []string) int {
	seed, dir, ok := seedAndDir("vars", args)
	if !ok {
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

// seedAndDir parses "--seed <n> <exercise-dir>" for the developer commands.
func seedAndDir(cmd string, args []string) (int64, string, bool) {
	var seed int64
	var dir string
	seedSet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--seed":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "%s: --seed needs a value\n", cmd)
				return 0, "", false
			}
			i++
			if _, err := fmt.Sscan(args[i], &seed); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --seed must be a number, got %q\n", cmd, args[i])
				return 0, "", false
			}
			seedSet = true
		default:
			dir = args[i]
		}
	}
	if dir == "" || !seedSet {
		fmt.Fprintf(os.Stderr, "%s: usage: rootcause %s --seed <n> <exercise-dir>\n", cmd, cmd)
		return 0, "", false
	}
	return seed, dir, true
}

func envName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
