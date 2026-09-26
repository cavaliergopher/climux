package climux

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// asInterrupt makes cmd an interrupt the way a library constructor such
// as VersionCommand does, moving its handler into the interrupt slot --
// the callback is the marker -- for a test that needs one shaped to its
// own scenario rather than reaching for VersionCommand itself.
func asInterrupt(cmd *Command) *Command {
	cmd.interrupt = cmd.handlerFunc
	cmd.handlerFunc = nil
	return cmd
}

// TestInterruptCommandSkipsAncestorRequiredFlag asserts that naming an
// interrupt subcommand answers even when an ancestor's Required flag is
// missing, the way an interrupt flag already does; see
// TestHelpWinsOverAnEarlierArgumentError in parser_test.go for the flag
// case this mirrors.
func TestInterruptCommandSkipsAncestorRequiredFlag(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		Flags(String("name", "").Required()).
		Subcommands(asInterrupt(NewCommand("version", "").HandleFunc(tr.handler("version", nil))))

	if err := Dispatch(context.Background(), app, WithArgs("version")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := tr.String(), "version"; got != want {
		t.Errorf("steps = %q, want %q", got, want)
	}
}

// TestInterruptCommandSkipsMiddleware asserts that middleware declared on
// an ancestor does not wrap an interrupt command's handler, the way none
// wraps a request for help.
func TestInterruptCommandSkipsMiddleware(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		Middleware(tr.step("root")).
		Subcommands(asInterrupt(NewCommand("version", "").HandleFunc(tr.handler("handler", nil))))

	if err := Dispatch(context.Background(), app, WithArgs("version")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := tr.String(), "handler"; got != want {
		t.Errorf("steps = %q, want %q", got, want)
	}
}

// TestNonInterruptSiblingStillEnforcesRules asserts that a sibling which
// is not an interrupt is unaffected by one that is: it still enforces an
// ancestor's Required flag and still runs the inherited middleware.
func TestNonInterruptSiblingStillEnforcesRules(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		Flags(String("name", "").Required()).
		Middleware(tr.step("root")).
		Subcommands(
			asInterrupt(NewCommand("version", "").HandleFunc(tr.handler("version", nil))),
			NewCommand("sibling", "").HandleFunc(tr.handler("sibling", nil)),
		)

	if err := Dispatch(context.Background(), app, WithArgs("version")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := tr.String(), "version"; got != want {
		t.Errorf("steps = %q, want %q", got, want)
	}

	tr.steps = nil
	if _, err := Parse(app, "sibling"); err == nil {
		t.Fatal("expected error, got nil")
	}

	tr.steps = nil
	if err := Dispatch(context.Background(), app, WithArgs("--name=x", "sibling")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := tr.String(), "root in, sibling, root out"; got != want {
		t.Errorf("steps = %q, want %q", got, want)
	}
}

// TestInterruptCommandAtTheRootReadsItsLine asserts that an interrupt
// which is itself the command the line was parsed against binds what
// followed it, the way any command does.
func TestInterruptCommandAtTheRootReadsItsLine(t *testing.T) {
	var topics []string
	var ran bool
	app := asInterrupt(NewCommand("app", "").
		Flags(Strings("topic", "").Bind(&topics).Positional()).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			ran = true
			return nil
		}))

	if err := Dispatch(context.Background(), app, WithArgs("foo", "bar")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := ran, true; got != want {
		t.Errorf("ran = %v, want %v", got, want)
	}
	if want := []string{"foo", "bar"}; !slices.Equal(topics, want) {
		t.Errorf("topics = %q, want %q", topics, want)
	}
}

// TestInterruptCommandCompilesIntoItsHandler asserts that an interrupt's
// callback is the compiled Handler, so running a command is one call
// however it answers. A second slot holding the real callback would
// leave Handler reporting the usage error a command with no handler of
// its own gets, which is not what the program declared.
func TestInterruptCommandCompilesIntoItsHandler(t *testing.T) {
	var ran bool
	app := NewCommand("app", "").
		Subcommands(asInterrupt(NewCommand("version", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				ran = true
				return nil
			})))

	node, err := app.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub := node.Subcommands[0]
	if got, want := sub.Interrupts, true; got != want {
		t.Errorf("Interrupts = %v, want %v", got, want)
	}
	if err := sub.Handler(context.Background(), &Invocation{Cmd: sub}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := ran, true; got != want {
		t.Errorf("interrupt callback ran = %v, want %v", got, want)
	}
}

// TestInterruptCommandDeclaresNoSubcommands asserts that a subcommand
// beneath an interrupt command is a configuration error, and that flags
// and positionals are not: a help topic is an argument, not a command.
func TestInterruptCommandDeclaresNoSubcommands(t *testing.T) {
	noop := func(ctx context.Context, inv *Invocation) error { return nil }
	withSub := NewCommand("app", "").Subcommands(
		InterruptCommand("help", "", noop).Subcommands(NewCommand("config-vars", "")))
	_, err := withSub.Compile()
	if err == nil {
		t.Fatal("Compile succeeded, want a configuration error")
	}
	if got, want := err.Error(), "an interrupt command declares no subcommands"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}

	withTopic := NewCommand("app", "").Subcommands(
		InterruptCommand("help", "", noop).
			Flags(String("TOPIC", "").Positional()))
	if _, err := withTopic.Compile(); err != nil {
		t.Errorf("Compile: unexpected error: %v", err)
	}
}
