package climux

import (
	"context"
	"testing"
)

// TestSource covers the three sources a flag's value can have, and the
// precedence between them: argv beats the environment, and the
// environment beats the declared default. The flag reports all three
// itself.
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
			output := String("output", "").Default("json").Env("APP_OUTPUT").State()
			if _, err := Parse(NewCommand("app", "").Flags(output), tt.args...); err != nil {
				t.Fatal(err)
			}
			if got, want := output.Source(), tt.want; got != want {
				t.Errorf("Source() = %v, want %v", got, want)
			}
			if got, want := output.IsSet(), tt.want != SourceDefault; got != want {
				t.Errorf("IsSet() = %v, want %v", got, want)
			}
			assertString(t, tt.wantOn, output.Value())
		})
	}
}

// TestSourceBound covers the same answers for a flag bound to a variable
// of the program's own: the variable and Value never disagree.
func TestSourceBound(t *testing.T) {
	var output string
	outputFlag := String("output", "").Default("json").Bind(&output)
	if _, err := Parse(NewCommand("app", "").Flags(outputFlag), "--output", "yaml"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "yaml", output)
	assertString(t, "yaml", outputFlag.State().Value())
	if got, want := outputFlag.State().Source(), SourceArgs; got != want {
		t.Errorf("Source() = %v, want %v", got, want)
	}
}

// TestSourceScope covers a flag an ancestor declared, which the line set
// before it dispatched: its state says so, and a sibling's flag the line
// never reached says the opposite.
func TestSourceScope(t *testing.T) {
	verbose := Bool("verbose", "").State()
	force := Bool("force", "").State()
	sub := NewCommand("deploy", "").
		Flags(force).
		HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	cmd := NewCommand("app", "").
		Flags(verbose).
		Subcommands(sub)

	inv, err := Parse(cmd, "--verbose", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Cmd.Name, "deploy"; got != want {
		t.Fatalf("Cmd.Name = %q, want %q", got, want)
	}
	if got, want := verbose.Source(), SourceArgs; got != want {
		t.Errorf("verbose.Source() = %v, want %v", got, want)
	}
	if got, want := force.Source(), SourceDefault; got != want {
		t.Errorf("force.Source() = %v, want %v", got, want)
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
			target := String("TARGET", "").Positional().State()
			if _, err := Parse(NewCommand("app", "").Flags(target), tt.args...); err != nil {
				t.Fatal(err)
			}
			if got, want := target.Source(), tt.want; got != want {
				t.Errorf("Source() = %v, want %v", got, want)
			}
		})
	}
}

// TestSourceRepeated covers a flag given more than once, which has one
// source however many occurrences it accumulated, and counts them.
func TestSourceRepeated(t *testing.T) {
	tags := Strings("tag", "").NArgs(0, 0).State()
	if _, err := Parse(NewCommand("app", "").Flags(tags), "--tag", "a", "--tag", "b"); err != nil {
		t.Fatal(err)
	}
	if got, want := tags.Source(), SourceArgs; got != want {
		t.Errorf("Source() = %v, want %v", got, want)
	}
	if got, want := tags.Count(), 2; got != want {
		t.Errorf("Count() = %d, want %d", got, want)
	}
	assertStrings(t, []string{"a", "b"}, tags.Value())
}

// TestSourceInterrupt covers a command line carrying an interrupt: the
// line is read as usual, so a flag given beside it and one the
// environment supplies are both recorded where they came from, and the
// interrupt itself reports that it was given.
func TestSourceInterrupt(t *testing.T) {
	t.Setenv("APP_OUTPUT", "wide")
	verbose := Bool("verbose", "").State()
	output := String("output", "").Default("json").Env("APP_OUTPUT").State()
	help := HelpFlag().State()
	cmd := NewCommand("app", "").Flags(verbose, output, help)

	inv, err := Parse(cmd, "--verbose", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if inv.Interrupt == nil {
		t.Fatal("expected the invocation to name an interrupt")
	}
	if got, want := verbose.Source(), SourceArgs; got != want {
		t.Errorf("verbose.Source() = %v, want %v", got, want)
	}
	if got, want := output.Source(), SourceEnv; got != want {
		t.Errorf("output.Source() = %v, want %v", got, want)
	}
	assertString(t, "wide", output.Value())
	if got, want := help.IsSet(), true; got != want {
		t.Errorf("help.IsSet() = %v, want %v", got, want)
	}

	if _, err := Parse(cmd); err != nil {
		t.Fatal(err)
	}
	if got, want := help.IsSet(), false; got != want {
		t.Errorf("help.IsSet() after a line without it = %v, want %v", got, want)
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

// TestSourceOtherSubtree is why a flag answers for itself. Two sibling
// commands may both declare "force", and each state answers for its own
// declaration and no other.
func TestSourceOtherSubtree(t *testing.T) {
	deployForce := Bool("force", "").State()
	pushForce := Bool("force", "").State()
	cmd := NewCommand("app", "").Subcommands(
		NewCommand("deploy", "").Flags(deployForce).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }),
		NewCommand("push", "").Flags(pushForce).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil }),
	)

	if _, err := Parse(cmd, "deploy", "--force"); err != nil {
		t.Fatal(err)
	}
	if got, want := deployForce.Source(), SourceArgs; got != want {
		t.Errorf("deploy --force Source() = %v, want %v", got, want)
	}
	if got, want := pushForce.IsSet(), false; got != want {
		t.Errorf("push --force IsSet() = %v, want %v", got, want)
	}
}

// TestSourceMountedTwice covers one declaration reaching two subtrees
// through a registry, which lowers it to a compiled flag per command.
// Both point at one state, so the declaration answers wherever the line
// reached it, and a second parse of the tree starts it afresh.
func TestSourceMountedTwice(t *testing.T) {
	dryRun := Bool("dry-run", "").State()
	registry := &Registry{}
	registry.FlagGroups(NewFlagGroup("shared", "Shared", dryRun))

	sub := func(name string) *Command {
		return NewCommand(name, "").Mount(registry).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	}
	cmd := NewCommand("app", "").Subcommands(sub("deploy"), sub("push"))

	if _, err := Parse(cmd, "deploy", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if got, want := dryRun.Source(), SourceArgs; got != want {
		t.Errorf("Source() = %v, want %v", got, want)
	}
	if _, err := Parse(cmd, "push"); err != nil {
		t.Fatal(err)
	}
	if got, want := dryRun.Source(), SourceDefault; got != want {
		t.Errorf("Source() after a line without it = %v, want %v", got, want)
	}
}
