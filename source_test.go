package climux

import (
	"context"
	"testing"

	"go.hotsrc.dev/climux/ir"
)

// TestSource covers the three sources a flag's value can have, and the
// precedence between them: argv beats the environment, and the
// environment beats the default a flag was constructed with.
func TestSource(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		env    string
		want   Source
		wantOn string
	}{
		{name: "defaulted", want: SourceDefault, wantOn: "json"},
		{name: "from argv", args: []string{"--output", "yaml"}, want: SourceArgs, wantOn: "yaml"},
		{name: "from the environment", env: "wide", want: SourceEnv, wantOn: "wide"},
		{
			name: "argv over the environment",
			args: []string{"--output", "yaml"}, env: "wide",
			want: SourceArgs, wantOn: "yaml",
		},
		{
			// The value is the default, but it was still typed.
			name: "argv restating the default",
			args: []string{"--output", "json"},
			want: SourceArgs, wantOn: "json",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("APP_OUTPUT", tt.env)
			}
			var output string
			cmd := NewCommand("app", "").Flags(
				String(&output, "output", "json", "").Env("APP_OUTPUT"),
			)
			inv, err := Parse(cmd, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := inv.Source("output"), tt.want; got != want {
				t.Errorf("Source(%q) = %v, want %v", "output", got, want)
			}
			if got, want := inv.IsSet("output"), tt.want != SourceDefault; got != want {
				t.Errorf("IsSet(%q) = %v, want %v", "output", got, want)
			}
			assertString(t, tt.wantOn, output)
		})
	}
}

// TestSourceUndeclared covers the one name that reports SourceDefault
// without a flag behind it, which is the case Lookup exists to tell
// apart.
func TestSourceUndeclared(t *testing.T) {
	var verbose bool
	cmd := NewCommand("app", "").Flags(Bool(&verbose, "verbose", false, ""))
	inv, err := Parse(cmd, "--verbose")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Source("nonesuch"), SourceDefault; got != want {
		t.Errorf("Source(%q) = %v, want %v", "nonesuch", got, want)
	}
	if got := inv.Lookup("nonesuch"); got != nil {
		t.Errorf("Lookup(%q) = %v, want nil", "nonesuch", got)
	}
	if got := inv.Lookup("verbose"); got == nil {
		t.Fatalf("Lookup(%q) = nil, want the flag", "verbose")
	}
	if got, want := inv.Sources[inv.Lookup("verbose")], SourceArgs; got != want {
		t.Errorf("Sources[verbose] = %v, want %v", got, want)
	}
}

// TestSourceScope covers a flag an ancestor declared, which is in scope
// for the subcommand the command line reached and answers there.
func TestSourceScope(t *testing.T) {
	var verbose bool
	var force bool
	sub := NewCommand("deploy", "").
		Flags(Bool(&force, "force", false, "")).
		HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	cmd := NewCommand("app", "").
		Flags(Bool(&verbose, "verbose", false, "")).
		Subcommands(sub)

	inv, err := Parse(cmd, "--verbose", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Cmd.Name, "deploy"; got != want {
		t.Fatalf("Cmd.Name = %q, want %q", got, want)
	}
	if got, want := inv.Source("verbose"), SourceArgs; got != want {
		t.Errorf("Source(%q) = %v, want %v", "verbose", got, want)
	}
	if got, want := inv.Source("force"), SourceDefault; got != want {
		t.Errorf("Source(%q) = %v, want %v", "force", got, want)
	}
}

// TestSourcePositional covers an operand, which has a source like any
// other flag: this is how an optional positional tells "not given" from
// "given as the empty string".
func TestSourcePositional(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want Source
	}{
		{name: "omitted", want: SourceDefault},
		{name: "given", args: []string{"prod"}, want: SourceArgs},
		{name: "given empty", args: []string{""}, want: SourceArgs},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var target string
			cmd := NewCommand("app", "").Flags(
				String(&target, "TARGET", "", "").Positional(),
			)
			inv, err := Parse(cmd, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := inv.Source("TARGET"), tt.want; got != want {
				t.Errorf("Source(%q) = %v, want %v", "TARGET", got, want)
			}
		})
	}
}

// TestSourceRepeated covers a flag given more than once, which has one
// source however many occurrences it accumulated.
func TestSourceRepeated(t *testing.T) {
	var tags []string
	cmd := NewCommand("app", "").Flags(
		Strings(&tags, "tag", nil, "").NArgs(0, 0),
	)
	inv, err := Parse(cmd, "--tag", "a", "--tag", "b")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Source("tag"), SourceArgs; got != want {
		t.Errorf("Source(%q) = %v, want %v", "tag", got, want)
	}
	assertStrings(t, []string{"a", "b"}, tags)
}

// TestSourceInterrupt covers a command line an interrupt cut short: the
// flags given before it were applied and are recorded as such, and the
// environment, which an interrupt never reads, is not.
func TestSourceInterrupt(t *testing.T) {
	t.Setenv("APP_OUTPUT", "wide")
	var verbose bool
	var output string
	cmd := NewCommand("app", "").
		Flags(
			Bool(&verbose, "verbose", false, ""),
			String(&output, "output", "json", "").Env("APP_OUTPUT"),
		).
		HelpFlag()

	inv, err := Parse(cmd, "--verbose", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Interrupt == nil {
		t.Fatal("expected the invocation to name an interrupt")
	}
	if got, want := inv.Source("verbose"), SourceArgs; got != want {
		t.Errorf("Source(%q) = %v, want %v", "verbose", got, want)
	}
	if got, want := inv.Source("output"), SourceDefault; got != want {
		t.Errorf("Source(%q) = %v, want %v", "output", got, want)
	}
}

// TestSourceString covers the words a Source is written as, which a
// program reporting where a value came from prints.
func TestSourceString(t *testing.T) {
	for _, tt := range []struct {
		src  Source
		want string
	}{
		{SourceDefault, "default"},
		{SourceEnv, "env"},
		{SourceArgs, "args"},
		{Source(99), "unknown"},
	} {
		if got, want := tt.src.String(), tt.want; got != want {
			t.Errorf("Source(%d).String() = %q, want %q", tt.src, got, want)
		}
	}
}

// TestSourceInHandle covers the typed form of the same three answers,
// asked through the declaration rather than through its name.
func TestSourceInHandle(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		env  string
		want Source
	}{
		{name: "defaulted", want: SourceDefault},
		{name: "from argv", args: []string{"--output", "yaml"}, want: SourceArgs},
		{name: "from the environment", env: "wide", want: SourceEnv},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("APP_OUTPUT", tt.env)
			}
			var output string
			outputFlag := String(&output, "output", "json", "").Env("APP_OUTPUT")
			cmd := NewCommand("app", "").Flags(outputFlag)

			inv, err := Parse(cmd, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := outputFlag.SourceIn(inv), tt.want; got != want {
				t.Errorf("SourceIn = %v, want %v", got, want)
			}
			if got, want := outputFlag.IsSetIn(inv), tt.want != SourceDefault; got != want {
				t.Errorf("IsSetIn = %v, want %v", got, want)
			}
			if got, want := outputFlag.InScope(inv), true; got != want {
				t.Errorf("InScope = %v, want %v", got, want)
			}
		})
	}
}

// TestSourceInOtherSubtree is why the handle exists. Two sibling
// commands may both declare "force", so the name form answers about
// whichever one is in scope -- which is the wrong flag for a program
// holding the other declaration.
func TestSourceInOtherSubtree(t *testing.T) {
	var deployForce, pushForce bool
	deployFlag := Bool(&deployForce, "force", false, "")
	pushFlag := Bool(&pushForce, "force", false, "")
	cmd := NewCommand("app", "").Subcommands(
		NewCommand("deploy", "").Flags(deployFlag).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }),
		NewCommand("push", "").Flags(pushFlag).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }),
	)

	inv, err := Parse(cmd, "deploy", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := deployFlag.SourceIn(inv), SourceArgs; got != want {
		t.Errorf("deploy --force SourceIn = %v, want %v", got, want)
	}
	if got, want := pushFlag.IsSetIn(inv), false; got != want {
		t.Errorf("push --force IsSetIn = %v, want %v", got, want)
	}
	if got, want := pushFlag.InScope(inv), false; got != want {
		t.Errorf("push --force InScope = %v, want %v", got, want)
	}
	// The name form cannot tell them apart, which is the whole point.
	if got, want := inv.IsSet("force"), true; got != want {
		t.Errorf("IsSet(%q) = %v, want %v", "force", got, want)
	}
}

// TestSourceInAncestor covers a handle held for a flag an ancestor
// declared, which is in scope for every command beneath it.
func TestSourceInAncestor(t *testing.T) {
	var verbose bool
	verboseFlag := Bool(&verbose, "verbose", false, "")
	cmd := NewCommand("app", "").
		Flags(verboseFlag).
		Subcommands(NewCommand("deploy", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }))

	inv, err := Parse(cmd, "--verbose", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := verboseFlag.SourceIn(inv), SourceArgs; got != want {
		t.Errorf("SourceIn = %v, want %v", got, want)
	}
}

// TestSourceInMountedTwice covers one declaration reaching two subtrees
// through a registry, which lowers it to a compiled flag per command.
// The handle resolves to whichever of them was in scope.
func TestSourceInMountedTwice(t *testing.T) {
	var dryRun bool
	dryRunFlag := Bool(&dryRun, "dry-run", false, "")
	registry := &Registry{}
	registry.FlagGroups(NewFlagGroup("shared", "Shared", dryRunFlag))

	sub := func(name string) *Command {
		return NewCommand(name, "").Mount(registry).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	}
	cmd := NewCommand("app", "").Subcommands(sub("deploy"), sub("push"))

	inv, err := Parse(cmd, "deploy", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dryRunFlag.SourceIn(inv), SourceArgs; got != want {
		t.Errorf("SourceIn = %v, want %v", got, want)
	}

	// A second tree, a second compile, and the same declaration still
	// resolves -- against the flag that tree lowered, not the first's.
	inv, err = Parse(cmd, "push")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dryRunFlag.SourceIn(inv), SourceDefault; got != want {
		t.Errorf("SourceIn = %v, want %v", got, want)
	}
	if got, want := dryRunFlag.InScope(inv), true; got != want {
		t.Errorf("InScope = %v, want %v", got, want)
	}
}

// TestSourceInInterrupt covers the flag that ended the parse. It binds
// no value, so nothing Set it, but the command line named it and that is
// what a program asks about.
func TestSourceInInterrupt(t *testing.T) {
	helpFlag := HelpFlag()
	cmd := NewCommand("app", "").Flags(helpFlag)

	inv, err := Parse(cmd, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Interrupt == nil {
		t.Fatal("expected the invocation to name an interrupt")
	}
	if got, want := helpFlag.IsSetIn(inv), true; got != want {
		t.Errorf("IsSetIn = %v, want %v", got, want)
	}
	if got, want := inv.IsSet("help"), true; got != want {
		t.Errorf("IsSet(%q) = %v, want %v", "help", got, want)
	}

	inv, err = Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := helpFlag.IsSetIn(inv), false; got != want {
		t.Errorf("IsSetIn = %v, want %v", got, want)
	}
}

// TestOriginsAreDistinct guards the whole of what an Origin promises:
// that no two declarations share one, and that none is the zero an
// unstamped flag carries.
func TestOriginsAreDistinct(t *testing.T) {
	seen := make(map[ir.Origin]int)
	for i := range 64 {
		seen[ir.NewOrigin()] = i
	}
	if got, want := len(seen), 64; got != want {
		t.Errorf("distinct origins = %d, want %d", got, want)
	}
	for o := range seen {
		if o == 0 {
			t.Error("NewOrigin returned the zero Origin, which belongs to no declaration")
		}
	}
}

// TestResolveZeroOrigin covers a flag no declaration was lowered into,
// which is what a tree assembled by hand out of ir types holds.
func TestResolveZeroOrigin(t *testing.T) {
	cmd := NewCommand("app", "").Flags(Bool(new(bool), "verbose", false, ""))
	inv, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.Resolve(0); got != nil {
		t.Errorf("Resolve(0) = %v, want nil", got)
	}
	if got := inv.Resolve(ir.NewOrigin()); got != nil {
		t.Errorf("Resolve(unstamped) = %v, want nil", got)
	}
}
