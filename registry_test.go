package climux

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.hotsrc.dev/climux/ir"
)

// TestRegistryChains asserts that all three registration methods are
// variadic and return the registry, so a library contributes everything it
// exports in one statement and each kind arrives in the order given.
func TestRegistryChains(t *testing.T) {
	var tr tracer
	registry := new(Registry).
		FlagGroups(
			NewFlagGroup("first", "First options", String("one", "")),
			NewFlagGroup("second", "Second options", String("two", "")),
		).
		Middleware(tr.step("outer"), tr.step("inner")).
		Subcommands(
			NewCommand("alpha", "").HandleFunc(tr.handler("alpha", nil)),
			NewCommand("beta", ""),
		)
	app := NewCommand("app", "").Mount(registry)

	node, err := app.Compile()
	if err != nil {
		t.Fatal(err)
	}
	var groups, subs []string
	for _, group := range node.FlagGroups {
		groups = append(groups, group.Name)
	}
	for _, sub := range node.Subcommands {
		subs = append(subs, sub.Name)
	}
	assertString(t, "options first second", strings.Join(groups, " "))
	assertString(t, "alpha beta", strings.Join(subs, " "))

	if err := Dispatch(context.Background(), app, WithArgs("alpha")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "outer in, inner in, alpha, inner out, outer out", tr.String())
}

// TestMountResolvesAtParse asserts that a mounted registry is read when
// the tree is parsed rather than when Mount is called, so a group
// registered afterwards -- in a later var declaration, or an init function
// -- is still seen.
func TestMountResolvesAtParse(t *testing.T) {
	var s string
	registry := new(Registry)
	cmd := NewCommand("test", "").Mount(registry)
	registry.FlagGroups(NewFlagGroup("late", "Late options",
		String("name", "").Bind(&s),
	))
	if _, err := Parse(cmd, "--name=value"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "value", s)
}

// TestMountIsNotWrittenBack asserts that resolving a mounted registry
// leaves the command's own groups untouched: a repeated Parse, with a
// Describe between, must not see the mounted flags twice and report them
// as duplicates.
func TestMountIsNotWrittenBack(t *testing.T) {
	var s string
	registry := new(Registry)
	registry.FlagGroups(NewFlagGroup("lib", "Lib options",
		String("name", "").Bind(&s),
	))
	cmd := NewCommand("test", "").Mount(registry)
	for i := 1; i <= 2; i++ {
		if _, err := Parse(cmd, "--name=x"); err != nil {
			t.Fatalf("parse %d: %v", i, err)
		}
		if _, err := cmd.Compile(); err != nil {
			t.Fatalf("compile %d: %v", i, err)
		}
	}
}

// assertConfigError asserts that parsing cmd fails with a ConfigError,
// naming the invalid configuration under test in the failure message.
func assertConfigError(t *testing.T, cmd *Command, reason string) bool {
	t.Helper()
	_, err := Parse(cmd)
	if err == nil {
		t.Errorf("expected error for %s, got nil", reason)
		return false
	}
	var cfgErr *ir.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Errorf("expected ConfigError for %s, got %T: %v", reason, err, err)
		return false
	}
	return true
}

// TestMountDuplicateFlagNames asserts that a name collision involving
// mounted groups is the same configuration error as one between declared
// flags, whether the collision is with the command's own flag or with
// another mounted registry.
func TestMountDuplicateFlagNames(t *testing.T) {
	newRegistry := func() *Registry {
		registry := new(Registry)
		registry.FlagGroups(NewFlagGroup("lib", "Lib options",
			String("foo", ""),
		))
		return registry
	}
	t.Run("WithOwnFlag", func(t *testing.T) {
		assertConfigError(t, NewCommand("test", "").
			Flags(String("foo", "")).
			Mount(newRegistry()),
			"a mounted flag colliding with a declared flag")
	})
	t.Run("BetweenRegistries", func(t *testing.T) {
		assertConfigError(t, NewCommand("test", "").
			Mount(newRegistry(), newRegistry()),
			"the same flag name mounted from two registries")
	})
}

// TestMountUsage asserts that mounted groups appear in help output under
// their own headings, after the command's own groups.
func TestMountUsage(t *testing.T) {
	registry := new(Registry)
	registry.FlagGroups(NewFlagGroup("logging", "Logging options",
		String("log-level", "Set log verbosity"),
	))
	registry.FlagGroups(NewFlagGroup("metrics", "Metrics options",
		String("metrics-addr", "Metrics listen address"),
	))
	cmd := NewCommand("test", "").
		Flags(Bool("verbose", "Print more")).
		Mount(registry)

	node, err := cmd.Compile()
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := node.Usage(&buf); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"Usage: test [OPTIONS]",
		"",
		"Options:",
		"   --verbose  Print more",
		"",
		"Logging options:",
		"   --log-level  Set log verbosity",
		"",
		"Metrics options:",
		"   --metrics-addr  Metrics listen address",
		"",
	}, "\n")
	if got := buf.String(); got != want {
		t.Errorf("Usage = %q, want %q", got, want)
	}
}

// TestMountTwiceInOnePath asserts that a subcommand may mount the same
// registry as an ancestor. A command is often mounted somewhere its author
// did not choose, so two teams both reaching DefaultRegistry is ordinary
// rather than a name conflict; only a command's own declarations claim a
// name against its descendants.
func TestMountTwiceInOnePath(t *testing.T) {
	var level string
	registry := new(Registry)
	registry.FlagGroups(NewFlagGroup("telemetry", "Telemetry options",
		String("log-level", "").Bind(&level).Default("info"),
	))
	sub := NewCommand("sub", "").Mount(registry)
	cmd := NewCommand("test", "").Mount(registry).Subcommands(sub)

	if _, err := Parse(cmd, "sub", "--log-level=debug"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "debug", level)
}

// TestMountMiddlewareOrder asserts where a registered wrapper sits: inside
// what the mounting command inherited, outside what it declared itself,
// and around every command beneath it. A wrapper registered beside the
// flag it reads has to bound the handlers the mounting program wrote, or a
// program could declare its way out of it.
func TestMountMiddlewareOrder(t *testing.T) {
	var tr tracer
	registry := new(Registry)
	registry.Middleware(tr.step("registered"))

	sub := NewCommand("sub", "").
		Middleware(tr.step("sub")).
		HandleFunc(tr.handler("handler", nil))
	app := NewCommand("app", "").
		Middleware(tr.step("declared")).
		Mount(registry).
		Subcommands(sub)

	if err := Dispatch(context.Background(), app, WithArgs("sub")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "registered in, declared in, sub in, handler, sub out, declared out, registered out"
	assertString(t, want, tr.String())
}

// TestMountMiddlewareResolvesAtDispatch asserts that middleware registered
// after Mount still wraps, the way a flag group registered afterwards is
// still mounted: a registry is read when a command that mounts it runs.
func TestMountMiddlewareResolvesAtDispatch(t *testing.T) {
	var tr tracer
	registry := new(Registry)
	app := NewCommand("app", "").
		Mount(registry).
		HandleFunc(tr.handler("handler", nil))
	registry.Middleware(tr.step("late"))

	if err := Dispatch(context.Background(), app, WithArgs()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "late in, handler, late out", tr.String())
}

// TestMountNilMiddleware asserts that a nil wrapper in a registry is the
// same configuration error as a nil one declared on a command, rather than
// a panic when the tree is lowered.
func TestMountNilMiddleware(t *testing.T) {
	registry := new(Registry)
	registry.Middleware(nil)
	assertConfigError(t, NewCommand("test", "").
		Mount(registry).
		HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }),
		"a nil registered middleware")
}

// TestMountSubcommands asserts that a registered command is dispatchable
// as a child of whatever mounts the registry, and runs inside the
// registry's own middleware the way a declared subcommand does.
func TestMountSubcommands(t *testing.T) {
	var tr tracer
	registry := new(Registry)
	registry.Middleware(tr.step("registered"))
	registry.Subcommands(
		NewCommand("diag", "").HandleFunc(tr.handler("diag", nil)),
	)
	app := NewCommand("app", "").Mount(registry)

	if err := Dispatch(context.Background(), app, WithArgs("diag")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "registered in, diag, registered out", tr.String())
}

// TestMountSubcommandOrder asserts that registered commands follow the
// ones the mounting command declared, so the program's own tree reads
// first in help and a library cannot insert itself above it.
func TestMountSubcommandOrder(t *testing.T) {
	registry := new(Registry)
	registry.Subcommands(NewCommand("diag", "Diagnostics"))
	app := NewCommand("app", "").
		Mount(registry).
		Subcommands(NewCommand("deploy", "Ship it"))

	node, err := app.Compile()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, sub := range node.Subcommands {
		got = append(got, sub.Name)
	}
	assertString(t, "deploy diag", strings.Join(got, " "))
}

// TestMountSubcommandParented asserts that a command already mounted in a
// tree of its own must not also be registered. A registry claims no
// parent, so such a command would be lowered under two of them, and only
// the source *Command remembers which mount it accepted.
func TestMountSubcommandParented(t *testing.T) {
	owned := NewCommand("diag", "")
	NewCommand("other", "").Subcommands(owned)

	registry := new(Registry)
	registry.Subcommands(owned)
	assertConfigError(t, NewCommand("app", "").Mount(registry),
		"a registered command already parented elsewhere")
}

// TestMountSubcommandNameCollision asserts that a registered command may
// not take a name the mounting command declared. Registered children are
// appended after declared ones and dispatch resolves a name to the last
// match, so without the check a package a program merely links in would
// silently replace a command that program wrote.
func TestMountSubcommandNameCollision(t *testing.T) {
	registry := new(Registry)
	registry.Subcommands(NewCommand("diag", "from the library"))
	assertConfigError(t, NewCommand("app", "").
		Mount(registry).
		Subcommands(NewCommand("diag", "from the program").
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })),
		"a registered command taking a declared command's name")
}

// TestMountSharedByTwoTrees asserts the property that keeps a registry
// from being a Command: mounting one writes nothing back to it, so two
// programs -- or two runs of the same test -- may each mount the same
// registry and each lower its contributions under a root of their own.
func TestMountSharedByTwoTrees(t *testing.T) {
	registry := new(Registry)
	registry.Subcommands(NewCommand("diag", "Diagnostics"))

	for _, name := range []string{"first", "second"} {
		app := NewCommand(name, "").Mount(registry)
		node, err := app.Compile()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(node.Subcommands) != 1 {
			t.Fatalf("%s: got %d subcommands, want 1", name, len(node.Subcommands))
		}
		assertString(t, name+" diag", node.Subcommands[0].FullName)
	}
}
