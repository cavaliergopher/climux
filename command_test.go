package climux

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"go.hotsrc.dev/climux/ir"
)

func TestSubcommands(t *testing.T) {
	// ranCommands is a bit mask to identify which subcommand handlers were
	// invoked
	var ranCommands uint64
	var setFlags uint64

	// newCommand is a function to recursively create subcommands
	var newCommand func(n, of uint64) *Command
	newCommand = func(n, of uint64) *Command {
		c := NewCommand(fmt.Sprintf("command%02d", n), "").
			Flags(
				BitField(
					&setFlags,
					uint64(1)<<(n-1),
					fmt.Sprintf("x%02d", n),

					""),
			).
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				ranCommands |= 1 << (n - 1)
				return nil
			})
		if n < of {
			c.Subcommands(newCommand(n+1, of))
		}
		return c
	}

	// call each subcommand
	cmdDepth := uint64(64)
	cmd := NewCommand("test", "").
		Subcommands(newCommand(1, cmdDepth))
	for i := uint64(0); i < cmdDepth; i++ {
		// build args to call subcommand i
		ranCommands = 0
		args := make([]string, 0)
		for j := uint64(0); j < i+1; j++ {
			args = append(
				args,
				fmt.Sprintf("command%02d", j+1), fmt.Sprintf("--x%02d", j+1),
			)
		}

		// invoke the subcommand handler
		if err := Dispatch(context.Background(), cmd, WithArgs(args...)); err != nil {
			t.Error(err)
			return
		}

		// check which commands run and flags were set
		assertUint64(t, 1<<i, ranCommands)
		expectFlags := uint64(0)
		for j := uint64(0); j < i+1; j++ {
			expectFlags |= 1 << j
		}
		assertUint64(t, expectFlags, setFlags)
	}
}

// TestPosFlagOrdering enforces the rule that no positional arguments may be
// specified after another variable length positional argument as this would
// create ambiguity as to which flag a given argument belongs to. Fixed length
// positional arguments do not exhibit this problem.
func TestPosFlagOrdering(t *testing.T) {
	var sink string
	getFixture := func(flags ...Flag) *Command {
		return NewCommand("test", "").Flags(flags...)
	}
	successCases := []*Command{
		getFixture(
			String(&sink, "one", "").Positional(),
		),
		getFixture(
			String(&sink, "one", "").Positional(),
			String(&sink, "two", "").Positional(),
		),
		getFixture(
			String(&sink, "one", "").Positional().NArgs(0, 1),
			String(&sink, "two", "").Positional(),
		),
		getFixture(
			String(&sink, "one", "").Positional().NArgs(1, 1),
			String(&sink, "two", "").Positional(),
		),
		getFixture(
			String(&sink, "one", "").Positional().NArgs(1, 1),
			String(&sink, "two", "").Positional().NArgs(2, 2),
			String(&sink, "three", "").Positional().NArgs(3, 3),
			String(&sink, "four", "").Positional(),
		),
	}
	for i, cmd := range successCases {
		t.Run(fmt.Sprintf("SuccessCase%02d", i+1), func(t *testing.T) {
			if err := cmd.validate(); err != nil {
				t.Errorf("expected nil error, got: %v", err)
			}
		})
	}
	errorCases := []*Command{
		getFixture(
			String(&sink, "one", "").Positional().NArgs(0, 0),
			String(&sink, "two", "").Positional(),
		),
		getFixture(
			String(&sink, "one", "").Positional().NArgs(1, 0),
			String(&sink, "two", "").Positional(),
		),
	}
	for i, cmd := range errorCases {
		t.Run(fmt.Sprintf("ErrorCase%02d", i+1), func(t *testing.T) {
			if err := cmd.validate(); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestPositionalFlags(t *testing.T) {
	var foo, bar string
	var baz, qux []string
	cmd := NewCommand("test", "").Flags(
		String(&foo, "foo", "").Positional().Required(),
		String(&bar, "bar", "").Positional().Required(),
		Strings(&baz, "baz", "").Positional().NArgs(2, 2),
		Strings(&qux, "qux", "").Positional().NArgs(0, 0),
	)
	_, err := Parse(cmd, "one", "two", "three", "four", "five", "six")
	if err != nil {
		t.Error(err)
		return
	}
	assertString(t, "one", foo)
	assertString(t, "two", bar)
	assertStrings(t, []string{"three", "four"}, baz)
	assertStrings(t, []string{"five", "six"}, qux)
}

func TestFromFlagSet(t *testing.T) {
	var foo, bar string
	var baz, qux bool
	flagSet := flag.NewFlagSet("native", flag.ContinueOnError)
	flagSet.StringVar(&foo, "foo", "", "")
	flagSet.BoolVar(&baz, "baz", false, "")
	c := NewCommand("test", "").
		Flags(
			String(&bar, "bar", ""),
			Bool(&qux, "qux", ""),
		).
		FlagGroups(FromFlagSet("native", "Native options", flagSet))
	_, err := Parse(c, "--foo", "foo", "--bar", "bar", "--baz", "--qux")
	if err != nil {
		t.Fatal(err)
	}
	assertString(t, "foo", foo)
	assertString(t, "bar", bar)
	assertBool(t, true, baz)
	assertBool(t, true, qux)
}

// TestFromFlagSetIsPersistent asserts that an imported flag stays valid
// beneath the command it is mounted on, since a flag set is written for a
// whole program rather than for one command.
func TestFromFlagSetIsPersistent(t *testing.T) {
	var foo string
	flagSet := flag.NewFlagSet("native", flag.ContinueOnError)
	flagSet.StringVar(&foo, "foo", "", "")
	c := NewCommand("test", "").
		FlagGroups(FromFlagSet("native", "Native options", flagSet)).
		Subcommands(NewCommand("sub", ""))
	if _, err := Parse(c, "sub", "--foo", "foo"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "foo", foo)
}

// opaqueFlagValue implements flag.Value but not flag.Getter, the way a
// hand-written stdlib flag often does, so FromFlagSet has no concrete
// type to recover a narrower Kind from.
type opaqueFlagValue struct{ s string }

func (v *opaqueFlagValue) String() string     { return v.s }
func (v *opaqueFlagValue) Set(s string) error { v.s = s; return nil }

// TestFromFlagSetRecoversKind asserts that a flag imported from a
// flag.FlagSet is described as precisely as a native one: its Kind is
// recovered from the concrete type its Value's Get returns, and a Value
// that does not implement flag.Getter at all compiles to ir.KindOpaque.
func TestFromFlagSetRecoversKind(t *testing.T) {
	var s string
	var b bool
	var i int
	var i64 int64
	var u uint
	var u64 uint64
	var f float64
	var d time.Duration
	flagSet := flag.NewFlagSet("native", flag.ContinueOnError)
	var txt big.Float
	flagSet.BoolVar(&b, "b", false, "")
	flagSet.BoolFunc("bf", "", func(string) error { return nil })
	flagSet.DurationVar(&d, "d", 0, "")
	flagSet.Float64Var(&f, "f", 0, "")
	flagSet.Func("fn", "", func(string) error { return nil })
	flagSet.IntVar(&i, "i", 0, "")
	flagSet.Int64Var(&i64, "i64", 0, "")
	flagSet.Var(&opaqueFlagValue{}, "opaque", "")
	flagSet.StringVar(&s, "s", "", "")
	flagSet.TextVar(&txt, "txt", &big.Float{}, "")
	flagSet.UintVar(&u, "u", 0, "")
	flagSet.Uint64Var(&u64, "u64", 0, "")

	cmd := NewCommand("test", "").FlagGroups(FromFlagSet("native", "Native options", flagSet))
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The implicit "options" group is first, empty here; the mounted
	// group follows. VisitAll, which FromFlagSet reads, visits in
	// lexicographical order.
	for i, want := range []ir.Kind{
		ir.KindBool, ir.KindBool, ir.KindDuration, ir.KindFloat,
		ir.KindOpaque, ir.KindInt, ir.KindInt, ir.KindOpaque,
		ir.KindString, ir.KindOpaque, ir.KindUint, ir.KindUint,
	} {
		if got := node.FlagGroups[1].Flags[i].Kind; got != want {
			t.Errorf("Flags[%d].Kind = %q, want %q", i, got, want)
		}
	}
}

func TestCommandLineage(t *testing.T) {
	a, b, c := NewCommand("a", ""), NewCommand("b", ""), NewCommand("c", "")
	a.Subcommands(b)
	b.Subcommands(c)
	assertString(t, "a", a.name)
	assertString(t, "b", a.subcommands[0].name)
	assertString(t, "a", a.subcommands[0].parent.name)
	assertString(t, "c", a.subcommands[0].subcommands[0].name)
	assertString(t, "b", a.subcommands[0].subcommands[0].parent.name)
}

// TestSubcommandAlreadyParented asserts that Subcommands does not steal an
// already-parented command -- such as one a library exports and two trees
// both mount -- and that the mismatch is reported as a ConfigError rather
// than silently corrupting the original relationship.
func TestSubcommandAlreadyParented(t *testing.T) {
	a, b, shared := NewCommand("a", ""), NewCommand("b", ""), NewCommand("shared", "")
	a.Subcommands(shared)
	b.Subcommands(shared)

	assertString(t, "a", shared.parent.name)
	if _, err := Parse(a); err != nil {
		t.Errorf("a.Parse: expected nil error, got: %v", err)
	}
	assertConfigError(t, b, "a subcommand already parented elsewhere")
}

// TestSubcommandDuplicateName asserts that two children answering to one
// word are a configuration error. Dispatch resolves a name to a single
// command, so without the check the second is unreachable and nothing
// says which was meant.
func TestSubcommandDuplicateName(t *testing.T) {
	handle := func(ctx context.Context, inv *Invocation) error { return nil }
	assertConfigError(t, NewCommand("app", "").Subcommands(
		NewCommand("x", "first").HandleFunc(handle),
		NewCommand("x", "second").HandleFunc(handle),
	), "two subcommands sharing a name")
}

// TestSubcommandCycle asserts that a command tree that leads back into
// itself is reported as a ConfigError rather than walked forever. Every
// shape here wedged the process before the tree could be validated: the
// first three walking parent links to find a root that is not there, the
// last two descending subcommand links that lead back up.
func TestSubcommandCycle(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *Command
	}{
		{
			// A command mounted under itself, which Subcommands accepts
			// because its parent is nil at the time.
			name: "Self",
			cmd: func() *Command {
				a := NewCommand("a", "")
				return a.Subcommands(a)
			},
		},
		{
			name: "Mutual",
			cmd: func() *Command {
				a, b := NewCommand("a", ""), NewCommand("b", "")
				a.Subcommands(b)
				b.Subcommands(a)
				return a
			},
		},
		{
			name: "Deep",
			cmd: func() *Command {
				a, b, c := NewCommand("a", ""), NewCommand("b", ""), NewCommand("c", "")
				a.Subcommands(b)
				b.Subcommands(c)
				c.Subcommands(a)
				return a
			},
		},
		{
			// The parent links are acyclic here -- Subcommands leaves b's
			// parent alone, since a already claimed it -- so only the
			// descent through subcommands leads back up.
			name: "SubcommandsOnly",
			cmd: func() *Command {
				a, b, c := NewCommand("a", ""), NewCommand("b", ""), NewCommand("c", "")
				a.Subcommands(b)
				b.Subcommands(c)
				c.Subcommands(b)
				return a
			},
		},
		{
			name: "MountedTwice",
			cmd: func() *Command {
				a, b := NewCommand("a", ""), NewCommand("b", "")
				return a.Subcommands(b, b)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertConfigError(t, tt.cmd(), "a cycle in the command tree")
		})
	}
}

// TestSubcommandCycleFromDescendant asserts that the cycle is reported
// wherever Compile is called from, not only from the command that closes
// it.
func TestSubcommandCycleFromDescendant(t *testing.T) {
	a, b := NewCommand("a", ""), NewCommand("b", "")
	a.Subcommands(b)
	b.Subcommands(a)
	assertConfigError(t, b, "a cycle reached from a descendant")
}

func ExampleCommand_FlagGroups() {
	var n int
	var rightToLeft bool
	var endcoding string

	cmd := NewCommand("helloworld", "").
		HelpFlag().
		// n flag defines how many times to print "Hello, World!".
		Flags(Int(&n, "n", "Print n times").Default(1)).

		// Mount a flag group for language-related flags.
		FlagGroups(NewFlagGroup(
			"language",
			"Language options",
			String(&endcoding, "encoding", "Text encoding").Default("utf-8"),
			Bool(&rightToLeft, "rtl", "Print right-to-left"),
		))

	// Print the help page
	Run(context.Background(), cmd, WithArgs("--help"))
	// Output:
	// Usage: helloworld [OPTIONS]
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//   -n          Print n times
	//
	// Language options:
	//    --encoding  Text encoding
	//    --rtl       Print right-to-left
}

func ExampleFromFlagSet() {
	// create a Go-native flag set
	flagSet := flag.NewFlagSet("native", flag.ExitOnError)
	message := flagSet.String("m", "Hello, World!", "Message to print")

	// import the flagset into an climux command as a flag group
	cmd := NewCommand("helloworld", "").
		HelpFlag().
		FlagGroups(FromFlagSet("native", "Native options", flagSet)).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Println(*message)
			return nil
		})

	ctx := context.Background()

	// Print the help page
	fmt.Println("+ helloworld --help")
	Run(ctx, cmd, WithArgs("--help"))

	// Run the command
	fmt.Println()
	fmt.Println("+ helloworld")
	Run(ctx, cmd, WithArgs())
	// Output:
	// + helloworld --help
	// Usage: helloworld [OPTIONS]
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//
	// Native options:
	//   -m   Message to print
	//
	// + helloworld
	// Hello, World!
}

func ExampleCommand_Subcommands() {
	var n int

	// configure a "create" subcommand
	create := NewCommand("create", "Make new widgets").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Printf("Created %d widget(s)\n", n)
			return nil
		})

	// configure a "destroy" subcommand
	destroy := NewCommand("destroy", "Destroy widgets").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Printf("Destroyed %d widget(s)\n", n)
			return nil
		})

	// configure the main command with two subcommands and a persistent
	// "n" flag, so it can be given after either of them.
	cmd := NewCommand("widgets", "").
		HelpFlag().
		Flags(Int(&n, "n", "Affect n widgets").Default(1).Persistent()).
		Subcommands(create, destroy)

	ctx := context.Background()

	// Print the help page
	fmt.Println("+ widgets --help")
	Run(ctx, cmd, WithArgs("--help"))

	// Invoke the "create" subcommand
	fmt.Println()
	fmt.Println("+ widgets create -n=3")
	Run(ctx, cmd, WithArgs("create", "-n=3"))
	// Output:
	// + widgets --help
	// Usage: widgets [OPTIONS] COMMAND
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//   -n          Affect n widgets
	//
	// Commands:
	//   create   Make new widgets
	//   destroy  Destroy widgets
	//
	// + widgets create -n=3
	// Created 3 widget(s)
}

func ExampleCommand_Description() {
	var n int
	cmd := NewCommand("helloworld", "Say \"Hello, World!\"").
		HelpFlag().
		// Configure a description to print detailed information on the help
		// page.
		Description(
			"This utility prints \"Hello, World!\" to the standard output.\n" +
				"Print more than once with -n.",
		).
		Flags(Int(&n, "n", "Print n times").Default(1))

	// Print the help page
	Run(context.Background(), cmd, WithArgs("--help"))
	// Output:
	// Usage: helloworld [OPTIONS]
	//
	// Say "Hello, World!"
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//   -n          Print n times
	//
	// This utility prints "Hello, World!" to the standard output.
	// Print more than once with -n.
}

func ExampleFlagBuilder_EndOfOptions() {
	var verbose bool
	var args []string

	// create a command that hands its arguments to another program
	cmd := NewCommand("echo_wrapper", "wraps the echo command").
		Flags(
			Bool(&verbose, "v", "Print verbose output"),
			// once the first argument is taken, options have ended, so
			// echo's own options reach it rather than echo_wrapper
			Strings(&args, "arg", "Arguments to pass to echo").
				Positional().
				EndOfOptions(),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			if verbose {
				fmt.Printf("+ echo %s\n", strings.Join(args, " "))
			}
			fmt.Println(strings.Join(args, " "))
			return nil
		})

	// -v is echo_wrapper's; -n comes after the first argument, so it is echo's
	Run(context.Background(), cmd, WithArgs("-v", "Hello,", "World!", "-n"))
	// Output:
	// + echo Hello, World! -n
	// Hello, World! -n
}

func TestCompileRoot(t *testing.T) {
	sub := NewCommand("sub", "Sub command summary")
	root := NewCommand("root", "Root command summary").
		Description("Root description").
		Subcommands(sub)

	node, err := root.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := node.Name, "root"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := node.Summary, "Root command summary"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
	if got, want := node.Description, "Root description"; got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
	if got, want := len(node.Ancestry), 1; got != want {
		t.Errorf("len(Ancestry) = %d, want %d for a root", got, want)
	}
	if got, want := len(node.Subcommands), 1; got != want {
		t.Fatalf("len(Subcommands) = %d, want %d", got, want)
	}
	if got, want := node.Subcommands[0].Name, "sub"; got != want {
		t.Errorf("Subcommands[0].Name = %q, want %q", got, want)
	}
	subNode := node.Subcommands[0]
	if got, want := subNode.Ancestry, []*ir.Command{node, subNode}; !slices.Equal(got, want) {
		t.Errorf("Subcommands[0].Ancestry = %v, want %v", got, want)
	}
}

func TestCompileSubcommand(t *testing.T) {
	foo := NewCommand("foo", "Foo summary")
	bar := NewCommand("bar", "Bar summary")
	NewCommand("root", "Root summary").Subcommands(foo, bar)

	node, err := foo.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := node.Name, "foo"; got != want {
		t.Errorf("Name = %q, want %q", got, want)
	}
	if got, want := len(node.Ancestry), 2; got != want {
		t.Fatalf("len(Ancestry) = %d, want %d", got, want)
	}
	root := node.Ancestry[0]
	if got, want := root.Name, "root"; got != want {
		t.Errorf("Ancestry[0].Name = %q, want %q", got, want)
	}
	if got, want := root, node.Root; got != want {
		t.Errorf("Ancestry[0] = %v, want it to be Root %v", got, want)
	}
	var names []string
	for _, c := range root.Subcommands {
		names = append(names, c.Name)
	}
	assertStrings(t, []string{"foo", "bar"}, names)
}

// TestCompileValidationError asserts that Compile returns the same
// configuration error that Parse would for a misconfigured tree.
func TestCompileValidationError(t *testing.T) {
	var a, b string
	cmd := NewCommand("test", "").Flags(
		String(&a, "foo", ""),
		String(&b, "foo", ""),
	)

	_, compileErr := cmd.Compile()
	if compileErr == nil {
		t.Fatal("expected error from Compile for duplicate flag name, got nil")
	}

	_, parseErr := Parse(cmd)
	if parseErr == nil {
		t.Fatal("expected error from Parse for duplicate flag name, got nil")
	}
	if got, want := compileErr.Error(), parseErr.Error(); got != want {
		t.Errorf("Compile error %q, want the Parse error %q", got, want)
	}
}

// TestCompileIsPure asserts that Compile does not mutate the command tree
// or the variables flags are bound to: it must reflect neither a Parse that
// ran before it, nor any bookkeeping of its own.
func TestCompileIsPure(t *testing.T) {
	var s string
	cmd := NewCommand("test", "").Flags(
		String(&s, "name", "").Default("default-value").NArgs(0, 1),
	)

	if _, err := Parse(cmd, "--name=parsed-value"); err != nil {
		t.Fatalf("unexpected error from Parse: %v", err)
	}
	if got, want := s, "parsed-value"; got != want {
		t.Fatalf("s = %q, want %q after Parse", got, want)
	}

	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error from Compile: %v", err)
	}

	// The bound variable must still hold the parsed value: Compile must
	// not have written back to it.
	if got, want := s, "parsed-value"; got != want {
		t.Errorf("s = %q, want %q after Compile", got, want)
	}
	// The projected default must still show the value captured at
	// construction, not the live/parsed value.
	df := node.FlagGroups[0].Flags[0]
	if got, want := df.Default, "default-value"; got != want {
		t.Errorf("Default = %q, want %q", got, want)
	}
}

// assertParseError asserts that parsing cmd fails, naming the invalid
// configuration under test in the failure message.
func assertParseError(t *testing.T, cmd *Command, reason string) bool {
	t.Helper()
	if _, err := Parse(cmd); err == nil {
		t.Errorf("expected error for %s, got nil", reason)
		return false
	}
	return true
}

func TestValidateDuplicateFlagName(t *testing.T) {
	var a, b string
	assertParseError(t, NewCommand("test", "").Flags(
		String(&a, "foo", ""),
		String(&b, "foo", ""),
	), "duplicate flag name")
}

func TestValidateDuplicateShortName(t *testing.T) {
	var a, b string
	assertParseError(t, NewCommand("test", "").Flags(
		String(&a, "x", ""),
		String(&b, "x", ""),
	), "duplicate short name")
}

// TestValidateDuplicatePositionalName asserts that a duplicate name
// between two positional flags is reported as a duplicate operand, in
// the vocabulary a user of the command line would recognize, rather than
// the "flag" wording that fits an option.
func TestValidateDuplicatePositionalName(t *testing.T) {
	var a, b string
	_, err := Parse(NewCommand("test", "").Flags(
		String(&a, "file", "").Positional(),
		String(&b, "file", "").Positional(),
	))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err), "test: operand declared more than once: FILE"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestConfigErrorNamesGrandchildByPath asserts that a ConfigError on a
// deep subcommand reports where it lives: the bare name "add" could be
// any command called "add", but "app remote add" is not.
func TestConfigErrorNamesGrandchildByPath(t *testing.T) {
	var a, b string
	add := NewCommand("add", "").Flags(
		String(&a, "name", ""),
		String(&b, "name", ""),
	)
	remote := NewCommand("remote", "").Subcommands(add)
	app := NewCommand("app", "").Subcommands(remote)

	_, err := Parse(app)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := `app remote add: flag declared more than once: --name`
	if got := humanMessage(err); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestValidateAncestorShadowing asserts that a persistent flag's options
// may not be claimed again beneath it, by either spelling, however far up
// the path the ancestor is: both would be writable after the descendant is
// named. See docs/adr/flags-are-local-by-default.md.
//
// The error names both commands, since ancestry is what tells a reader
// which end to change, and neither is called the offender: which was
// declared first is an accident of mount order. The command the error is
// reported against is still named by its full path, since a bare "sub" or
// "leaf" would not say which among possibly many.
func TestValidateAncestorShadowing(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  *Command
		want string
	}{
		{
			name: "LongName",
			cmd: NewCommand("root", "").
				Flags(Bool(new(bool), "force", "").Persistent()).
				Subcommands(NewCommand("sub", "").Flags(
					Bool(new(bool), "force", ""),
				)),
			want: `root sub: flag declared on both "root" and "sub": --force`,
		},
		{
			name: "ShortName",
			cmd: NewCommand("root", "").
				Flags(String(new(string), "file", "").Aliases("f").Persistent()).
				Subcommands(NewCommand("sub", "").Flags(
					String(new(string), "output", "").Aliases("f"),
				)),
			want: `root sub: flag declared on both "root" and "sub": -f`,
		},
		{
			name: "GrandparentClaim",
			cmd: NewCommand("root", "").
				Flags(Bool(new(bool), "force", "").Persistent()).
				Subcommands(NewCommand("mid", "").Subcommands(
					NewCommand("leaf", "").Flags(
						Bool(new(bool), "force", ""),
					),
				)),
			want: `root mid leaf: flag declared on both "root" and "leaf": --force`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.cmd)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got, want := humanMessage(err), tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

// TestSiblingFlagReuse asserts that commands in different subtrees may declare the same names, and each
// spelling binds the variable of whichever sibling was invoked.
func TestSiblingFlagReuse(t *testing.T) {
	var deleteForce, pushForce bool
	app := NewCommand("app", "").Subcommands(
		NewCommand("delete", "").Flags(
			Bool(&deleteForce, "force", "").Aliases("f"),
		),
		NewCommand("push", "").Flags(
			Bool(&pushForce, "force", "").Aliases("f"),
		),
	)

	inv, err := Parse(app, "delete", "--force")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Cmd.Name, "delete"; got != want {
		t.Errorf("Cmd = %q, want %q", got, want)
	}
	assertBool(t, true, deleteForce)
	assertBool(t, false, pushForce)

	inv, err = Parse(app, "push", "-f")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := inv.Cmd.Name, "push"; got != want {
		t.Errorf("Cmd = %q, want %q", got, want)
	}
	assertBool(t, true, pushForce)
}

// newRemoteTree returns git's "remote" shape: a command with a handler of
// its own and an "add" subcommand, each declaring a --verbose of its own.
// remote's is persistent when persistent is set, and add's is omitted
// then, since the name would collide.
func newRemoteTree(remoteVerbose, addVerbose *bool, persistent bool) *Command {
	verbose := Bool(remoteVerbose, "verbose", "")
	add := NewCommand("add", "")
	if persistent {
		verbose.Persistent()
	} else {
		add.Flags(Bool(addVerbose, "verbose", ""))
	}
	return NewCommand("git", "").Subcommands(
		NewCommand("remote", "").Flags(verbose).Subcommands(add),
	)
}

// TestLocalFlagScope asserts that a local flag is valid from its own
// command's name until the line dispatches, and unknown after that, while
// a persistent one stays valid beneath its command. See
// docs/adr/flags-are-local-by-default.md.
func TestLocalFlagScope(t *testing.T) {
	t.Run("WrittenBeforeDispatch", func(t *testing.T) {
		var remoteVerbose, addVerbose bool
		inv, err := Parse(newRemoteTree(&remoteVerbose, &addVerbose, false),
			"remote", "--verbose", "add")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := inv.Cmd.Name, "add"; got != want {
			t.Errorf("Cmd = %q, want %q", got, want)
		}
		assertBool(t, true, remoteVerbose)
		assertBool(t, false, addVerbose)
	})

	t.Run("ShadowedAfterDispatch", func(t *testing.T) {
		var remoteVerbose, addVerbose bool
		inv, err := Parse(newRemoteTree(&remoteVerbose, &addVerbose, false),
			"remote", "--verbose", "add", "--verbose")
		if err != nil {
			t.Fatal(err)
		}
		assertBool(t, true, remoteVerbose)
		assertBool(t, true, addVerbose)
		// The nearest declaration answers for a reused name.
		if got, want := inv.Lookup("verbose").Origin, inv.Cmd.FlagGroups[0].Flags[0].Origin; got != want {
			t.Errorf("Lookup found remote's --verbose, want add's")
		}
	})

	t.Run("UnknownAfterDispatch", func(t *testing.T) {
		tree := NewCommand("git", "").Subcommands(
			NewCommand("remote", "").
				Flags(Bool(new(bool), "verbose", "")).
				Subcommands(NewCommand("add", "")),
		)
		_, err := Parse(tree, "remote", "add", "--verbose")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got, want := humanMessage(err),
			`unrecognized option: --verbose (an option of "git remote")`; got != want {
			t.Errorf("message = %q, want %q", got, want)
		}
	})

	t.Run("PersistentAfterDispatch", func(t *testing.T) {
		var remoteVerbose bool
		if _, err := Parse(newRemoteTree(&remoteVerbose, nil, true),
			"remote", "add", "--verbose"); err != nil {
			t.Fatal(err)
		}
		assertBool(t, true, remoteVerbose)
	})

	// A word naming a subcommand is data while the command's positionals
	// are still filling, so the line has not dispatched and the command's
	// local flags stay valid.
	t.Run("PositionalDoesNotDispatch", func(t *testing.T) {
		var files []string
		var verbose bool
		app := NewCommand("app", "").
			Flags(
				Strings(&files, "file", "").Positional(),
				Bool(&verbose, "verbose", ""),
			).
			Subcommands(NewCommand("run", ""))
		inv, err := Parse(app, "x", "run", "--verbose")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := inv.Cmd.Name, "app"; got != want {
			t.Errorf("Cmd = %q, want %q", got, want)
		}
		assertStrings(t, []string{"x", "run"}, files)
		assertBool(t, true, verbose)
	})
}

// TestPersistentPositional asserts that a positional argument cannot be
// persistent: it is filled before the line dispatches, so no descendant
// could write it.
func TestPersistentPositional(t *testing.T) {
	app := NewCommand("app", "").
		Flags(String(new(string), "file", "").Positional().Persistent())
	_, err := Parse(app)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := err.Error(), "positional argument cannot be persistent"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}
}

// TestHelpListsPersistentAncestorFlags asserts that a subcommand's help
// lists its ancestors' persistent flags after its own, under their own
// groups' headings, and none of their local ones, which is exactly what
// it accepts.
func TestHelpListsPersistentAncestorFlags(t *testing.T) {
	app := NewCommand("app", "").
		HelpFlag().
		Flags(
			Bool(new(bool), "local", "Only for app"),
			Bool(new(bool), "global", "Everywhere").Persistent(),
		).
		Subcommands(NewCommand("sub", "").
			Flags(Bool(new(bool), "own", "Only for sub")))

	code, stdout, stderr := runCaptured(app, "sub", "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	want := `Usage: app sub [OPTIONS]

Options:
   --own  Only for sub

Options:
  -h, --help    Show this help message and exit
      --global  Everywhere
`
	if got := stdout; got != want {
		t.Errorf("help =\n%s\nwant:\n%s", got, want)
	}
}

// TestFirstOperandDecides asserts how a command that declares both
// subcommands and positionals reads its operands. The first chooses: a
// word naming a subcommand dispatches, and any other binds the first
// positional. Once one has bound, a word naming a subcommand is data
// until every positional is full. Dispatch starts the choice over, so a
// subcommand's first operand chooses again.
func TestFirstOperandDecides(t *testing.T) {
	var tr tracer
	var region, plugin, target string
	var rest []string
	noArgs := func() {
		tr.steps, region, plugin, target, rest = nil, "", "", "", nil
	}
	// docker's shape: its own commands beside a plugin catch whose
	// unbounded tail never fills.
	docker := func() *Command {
		return NewCommand("docker", "").
			Flags(
				String(&plugin, "PLUGIN", "").Positional().EndOfOptions(),
				Strings(&rest, "ARG", "").Positional(),
			).
			Subcommands(NewCommand("run", "").HandleFunc(tr.handler("run", nil))).
			HandleFunc(tr.handler("docker", nil))
	}
	// A bounded positional ahead of a subcommand, whose own first operand
	// chooses again after dispatch.
	app := func() *Command {
		return NewCommand("app", "").
			Flags(String(&region, "REGION", "").Positional()).
			Subcommands(NewCommand("deploy", "").
				Flags(String(&target, "TARGET", "").Positional()).
				Subcommands(NewCommand("canary", "").HandleFunc(tr.handler("canary", nil))).
				HandleFunc(tr.handler("deploy", nil))).
			HandleFunc(tr.handler("app", nil))
	}

	for _, tt := range []struct {
		name  string
		cmd   func() *Command
		args  []string
		steps string
		check func(t *testing.T)
	}{
		{"FirstNamesASubcommand", docker, []string{"run"}, "run", nil},
		{"FirstBindsThenANameIsData", docker, []string{"compose", "run", "web"}, "docker",
			func(t *testing.T) {
				assertString(t, "compose", plugin)
				assertStrings(t, []string{"run", "web"}, rest)
			}},
		{"TerminatorLeavesTheChoice", docker, []string{"--", "run"}, "run", nil},
		{"LookupResumesWhenFull", app, []string{"us-east-1", "deploy"}, "deploy",
			func(t *testing.T) { assertString(t, "us-east-1", region) }},
		{"DispatchStartsTheChoiceOver", app, []string{"us-east-1", "deploy", "canary"}, "canary", nil},
		{"SubcommandFirstOperandBinds", app, []string{"deploy", "prod"}, "deploy",
			func(t *testing.T) { assertString(t, "prod", target) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			noArgs()
			if err := Dispatch(context.Background(), tt.cmd(), WithArgs(tt.args...)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := tr.String(), tt.steps; got != want {
				t.Errorf("steps = %q, want %q", got, want)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}

// TestEndOfOptionsIsTheAuthorsTerminator asserts that an argument which
// ends option processing does what a user's "--" does, from the point it
// takes its token, and that a user may still write one earlier.
func TestEndOfOptionsIsTheAuthorsTerminator(t *testing.T) {
	var verbose bool
	var image, command string
	var args []string
	build := func() *Command {
		verbose, image, command, args = false, "", "", nil
		return NewCommand("run", "").
			Flags(
				Bool(&verbose, "verbose", "").Aliases("v"),
				String(&image, "IMAGE", "").Positional().Required().EndOfOptions(),
				String(&command, "COMMAND", "").Positional(),
				Strings(&args, "ARG", "").Positional(),
			).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	}

	for _, tt := range []struct {
		name        string
		args        []string
		wantVerbose bool
		wantImage   string
		wantCommand string
		wantArgs    []string
	}{
		{"BeforeTheBoundary", []string{"-v", "alpine", "ls"}, true, "alpine", "ls", nil},
		{"DashedTokenPastIt", []string{"alpine", "ls", "-la"}, false, "alpine", "ls", []string{"-la"}},
		{"ParentsOwnFlagPastIt", []string{"alpine", "-v"}, false, "alpine", "-v", nil},
		{"TerminatorPastItIsData", []string{"alpine", "--", "-v"}, false, "alpine", "--", []string{"-v"}},
		{"UserMayEndItEarlier", []string{"--", "-v", "ls"}, false, "-v", "ls", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := Dispatch(context.Background(), build(), WithArgs(tt.args...)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := verbose, tt.wantVerbose; got != want {
				t.Errorf("verbose = %v, want %v", got, want)
			}
			if got, want := image, tt.wantImage; got != want {
				t.Errorf("IMAGE = %q, want %q", got, want)
			}
			if got, want := command, tt.wantCommand; got != want {
				t.Errorf("COMMAND = %q, want %q", got, want)
			}
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("ARG = %q, want %q", args, tt.wantArgs)
			}
		})
	}
}

// TestUnbound asserts what a flag bound to no value is: given by name
// alone, with no negated spelling and no attached value, and recorded as
// given even when nothing is chained onto it, so a handler can ask.
func TestUnbound(t *testing.T) {
	dryRun := Unbound("dry-run", "")
	build := func() *Command {
		return NewCommand("test", "").
			Flags(dryRun, Unbound("end-of-options", "").EndOfOptions()).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	}

	inv, err := Parse(build(), "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := dryRun.IsSetIn(inv), true; got != want {
		t.Errorf("IsSetIn = %v, want %v", got, want)
	}
	inv, err = Parse(build())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := dryRun.IsSetIn(inv), false; got != want {
		t.Errorf("IsSetIn with nothing given = %v, want %v", got, want)
	}

	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--no-end-of-options"}, "unrecognized option: --no-end-of-options"},
		{[]string{"--dry-run=true"}, "option takes no argument: --dry-run"},
	} {
		_, err := Parse(build(), tt.args...)
		if got := humanMessage(err); got != tt.want {
			t.Errorf("Parse(%q) error = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// TestUnboundCannotBeSet asserts the two ways a flag bound to no value
// could be asked to hold one are configuration errors.
func TestUnboundCannotBeSet(t *testing.T) {
	assertParseError(t, NewCommand("test", "").Flags(Unbound("word", "").Positional()),
		"positional argument must be bound to a value")
	assertParseError(t, NewCommand("test", "").Flags(Unbound("dry-run", "").Env("DRY_RUN")),
		"flag bound to no value reads no environment variable")
}

// TestEndOfOptionsOnAnOption asserts that an option may end option
// processing as a positional does, from where it is given: an unbound one
// is a second spelling of "--", git's --end-of-options, and one taking a
// value starts the next program's arguments the way find's -exec does.
func TestEndOfOptionsOnAnOption(t *testing.T) {
	var verbose bool
	var exec string
	var args []string
	build := func() *Command {
		verbose, exec, args = false, "", nil
		return NewCommand("test", "").
			Flags(
				Bool(&verbose, "verbose", ""),
				Unbound("end-of-options", "").EndOfOptions(),
				String(&exec, "exec", "").EndOfOptions(),
				Strings(&args, "ARG", "").Positional(),
			).
			HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })
	}
	for _, tt := range []struct {
		name     string
		args     []string
		wantExec string
		wantArgs []string
	}{
		{"SecondSpellingOfTerminator", []string{"--end-of-options", "--verbose", "-x"}, "", []string{"--verbose", "-x"}},
		{"ValueStartsTheTail", []string{"--exec", "ls", "-la", "--verbose"}, "ls", []string{"-la", "--verbose"}},
		{"AttachedValueStartsTheTail", []string{"--exec=ls", "-la"}, "ls", []string{"-la"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := Dispatch(context.Background(), build(), WithArgs(tt.args...)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := verbose, false; got != want {
				t.Errorf("verbose = %v, want %v", got, want)
			}
			assertString(t, tt.wantExec, exec)
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("ARG = %q, want %q", args, tt.wantArgs)
			}
		})
	}
}

func TestValidatePositionalAfterUnbounded(t *testing.T) {
	var a, b string
	assertParseError(t, NewCommand("test", "").Flags(
		String(&a, "one", "").Positional().NArgs(0, 0),
		String(&b, "two", "").Positional(),
	), "positional after unbounded positional")
}

// TestArgumentErrorNamesTheFlag asserts that every parse error a user can
// provoke names the flag it is about. The flag is carried on the error either
// way, but a human reading stderr only sees Message.
func TestArgumentErrorNamesTheFlag(t *testing.T) {
	// Each case builds its own command, since validateNArgs reports the
	// first unsatisfied flag and a shared one would let cases mask each
	// other.
	for _, tt := range []struct {
		name string
		flag Flag
		args []string
		want string
	}{
		{
			"MissingRequired",
			String(new(string), "req", "").Required(),
			nil,
			"missing required argument: --req",
		},
		{
			"TooFewExactCount",
			Strings(&[]string{}, "pair", "").NArgs(2, 2),
			[]string{"--pair", "a"},
			"expected 2 arguments, got 1: --pair",
		},
		{
			"TooFewAtLeast",
			Strings(&[]string{}, "least", "").NArgs(2, 0),
			[]string{"--least", "a"},
			"expected at least 2 arguments, got 1: --least",
		},
		{
			"TooManyOccurrences",
			Strings(&[]string{}, "many", "").NArgs(0, 2),
			[]string{"--many", "a", "--many", "b", "--many", "c"},
			"argument specified too many times: --many",
		},
		{
			"OptionNeedsValue",
			String(new(string), "opt", ""),
			[]string{"--opt"},
			"option requires an argument: --opt",
		},
		{
			"UnrecognizedOption",
			String(new(string), "opt", ""),
			[]string{"--nope"},
			"unrecognized option: --nope",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(NewCommand("test", "").Flags(tt.flag), tt.args...)
			if err == nil {
				t.Fatalf("expected error for %v, got nil", tt.args)
			}
			if got, want := humanMessage(err), tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

// TestArgumentErrorNamesPositional asserts the same for a positional, which
// renders as its upper-cased name rather than with a leading dash.
func TestArgumentErrorNamesPositional(t *testing.T) {
	var files []string
	cmd := NewCommand("test", "").Flags(
		Strings(&files, "file", "").Positional().NArgs(1, 0),
	)
	_, err := Parse(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err), "missing required argument: FILE"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestValidateInvalidNArgs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		min, max int
		want     string
	}{
		{"MinExceedsMax", 2, 1, "minimum count 2 exceeds maximum count 1"},
		{"NegativeMin", -1, 1, "minimum count must not be negative: -1"},
		{"NegativeMax", 0, -1, "maximum count must not be negative: -1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var a string
			cmd := NewCommand("test", "").Flags(
				String(&a, "foo", "").NArgs(tt.min, tt.max),
			)
			_, err := Parse(cmd)
			if err == nil {
				t.Fatalf("NArgs(%d, %d): expected error, got nil", tt.min, tt.max)
			}
			if got, want := humanMessage(err), "--foo: "+tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

// TestValidateUnboundedMaxIsNotExceeded asserts that a max of 0 means
// unbounded rather than a ceiling the min can breach, so required-and-
// repeatable is a valid configuration.
func TestValidateUnboundedMaxIsNotExceeded(t *testing.T) {
	var a []string
	cmd := NewCommand("test", "").Flags(
		Strings(&a, "foo", "").NArgs(1, 0),
	)
	if _, err := Parse(cmd, "--foo", "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateShortName asserts POSIX guideline 3: a short name is one
// character from [A-Za-z0-9].
func TestValidateShortName(t *testing.T) {
	for _, shortName := range []string{
		"!", // outside the portable character set
		"=", // ... and this one the parser reads as a delimiter
		"-",
		" ",
		"é", // one character, but not one byte, and still not portable
	} {
		t.Run(shortName, func(t *testing.T) {
			var a string
			assertParseError(t, NewCommand("test", "").Flags(
				String(&a, "foo", "").Aliases(shortName),
			), "illegal short name")
		})
	}
	for _, shortName := range []string{"x", "X", "0"} {
		t.Run(shortName, func(t *testing.T) {
			var a string
			cmd := NewCommand("test", "").Flags(
				String(&a, "foo", "").Aliases(shortName),
			)
			if _, err := Parse(cmd); err != nil {
				t.Errorf("expected %q to be a legal short name: %v", shortName, err)
			}
		})
	}
}

// TestValidateCollectsAllErrors asserts that a malformed tree reports
// every configuration error in one run -- they surface in a batch at
// startup -- and that Run prints each on its own prefixed line.
func TestValidateCollectsAllErrors(t *testing.T) {
	var a, b, c string
	cmd := NewCommand("test", "").Flags(
		String(&a, "foo", ""),
		String(&b, "foo", ""),
		String(&c, "bar", "").Aliases("!"), // illegal short name
	)
	_, err := Parse(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cfgErr *ir.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected a *ConfigError in %v", err)
	}
	var code int
	stderr := captureStderr(t, func() {
		code = Run(context.Background(), cmd, WithArgs())
	})
	if got, want := code, 2; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	// Model errors precede spelling errors: Compile runs ir's validation,
	// which reads each flag on its own terms, before argv's, which reads
	// the spellings they render to. Order within a batch is not part of
	// the contract; that every error appears exactly once is.
	want := "Program error: --bar: short name must be one character from [A-Za-z0-9]: \"!\"\n" +
		"Program error: test: flag declared more than once: --foo\n"
	if got := stderr; got != want {
		t.Errorf("os.Stderr = %q, want %q", got, want)
	}
}

// TestConfigErrorReportsOnRunsStderr asserts that a tree which fails to
// compile reports on the stderr its Run call was given. The tree is what
// failed, so nothing it says about itself is worth trusting; the caller's
// stderr is.
func TestConfigErrorReportsOnRunsStderr(t *testing.T) {
	sub := NewCommand("sub", "").
		Flags(
			String(new(string), "foo", ""),
			String(new(string), "foo", ""),
		)
	cmd := NewCommand("test", "").Subcommands(sub)

	var stderr strings.Builder
	code := Run(context.Background(), cmd, WithArgs("sub"), WithStderr(&stderr))
	if got, want := code, 2; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	if got := stderr.String(); !strings.Contains(got, "foo") {
		t.Errorf("stderr = %q, want it to name the duplicate flag", got)
	}
}

// TestArgumentErrorWrapsArgumentErrorOnce asserts that an ArgumentError
// wrapping another, such as Choices reporting a bad value, prints its
// wrapped message plain: Error() tags it "climux: " for a Go caller, and
// that tag must not leak into the sentence Run prints for a human.
func TestArgumentErrorWrapsArgumentErrorOnce(t *testing.T) {
	cmd := NewCommand("test", "").Flags(
		String(new(string), "foo", "").Choices("a", "b"),
	)
	code, _, stderr := runCaptured(cmd, "--foo=c")
	if got, want := code, 2; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	want := "Argument error: --foo: expected one of: a, b\n" +
		"Usage: test [OPTIONS]\n" +
		"\n" +
		"Options:\n" +
		"   --foo  \n"
	if got := stderr; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// TestHandlerJoinedErrorReportsWhole asserts that only the batches
// validation collects are split into one line per error: a handler's own
// joined error reports whole, keeping the wrapper text a per-line split
// would drop.
func TestHandlerJoinedErrorReportsWhole(t *testing.T) {
	errA := errors.New("a is stale")
	errB := errors.New("b is stale")
	cmd := NewCommand("test", "").HandleFunc(
		func(ctx context.Context, inv *Invocation) error {
			return fmt.Errorf("syncing a and b failed: %w, %w", errA, errB)
		},
	)
	code, _, stderr := runCaptured(cmd)
	if got, want := code, 1; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	if got, want := stderr, "Error: syncing a and b failed: a is stale, b is stale\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// TestValidateFlagName asserts that a long name containing "=" or
// whitespace is rejected -- both break parsing -- while names that merely
// look unusual, such as a hyphenated one, remain legal.
func TestValidateFlagName(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
	}{
		{"foo=bar", `flag name must not contain '=': "foo=bar"`},
		{"foo bar", `flag name must not contain whitespace: "foo bar"`},
		{"foo\tbar", `flag name must not contain whitespace: "foo\tbar"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var a string
			cmd := NewCommand("test", "").Flags(String(&a, tt.name, ""))
			_, err := Parse(cmd)
			if err == nil {
				t.Fatalf("expected error for flag name %q, got nil", tt.name)
			}
			if got, want := humanMessage(err), "--"+tt.name+": "+tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
	for _, name := range []string{"dry-run", "helper"} {
		t.Run(name, func(t *testing.T) {
			var a string
			cmd := NewCommand("test", "").Flags(String(&a, name, ""))
			if _, err := Parse(cmd); err != nil {
				t.Errorf("expected %q to be a legal flag name: %v", name, err)
			}
		})
	}
}

// TestHelpNamesCollideOnlyWhenMounted asserts that "-h" and "--help" are
// not reserved. A command that mounts the help flag and then declares
// either name collides with it through the ordinary check, and a command
// that mounts no help flag may name them whatever it likes.
func TestHelpNamesCollideOnlyWhenMounted(t *testing.T) {
	var a string
	for _, tt := range []struct {
		name string
		flag Flag
		want string
	}{
		{
			"LongHelp",
			String(&a, "help", ""),
			"test: flag declared more than once: --help",
		},
		{
			"ShortH",
			String(&a, "foo", "").Aliases("h"),
			"test: flag declared more than once: -h",
		},
		{
			"ShortOnlyH", // a one-character name is spelled with one dash
			String(&a, "h", ""),
			"test: flag declared more than once: -h",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(NewCommand("test", "").HelpFlag().Flags(tt.flag))
			if err == nil {
				t.Fatal("expected a collision with the help flag, got nil")
			}
			if got, want := humanMessage(err), tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})

		t.Run(tt.name+"LegalUnmounted", func(t *testing.T) {
			if _, err := Parse(NewCommand("test", "").Flags(tt.flag)); err != nil {
				t.Errorf("expected the name to be free: %v", err)
			}
		})
	}
	// A collision is exact: "H" is not "h", "helper" is not "help", and
	// "--no-help" belongs to nobody, since an interrupt has no negation.
	t.Run("Legal", func(t *testing.T) {
		var b string
		cmd := NewCommand("test", "").
			HelpFlag().
			Flags(
				String(&a, "helper", "").Aliases("H"),
				String(&b, "no-help", ""),
			)
		if _, err := Parse(cmd); err != nil {
			t.Errorf("expected nearby names to remain legal: %v", err)
		}
	})
}

// TestValidateErrorsSurfaceAtParse asserts that a misconfigured tree does not
// error, or panic, at construction time -- only once Parse is called.
func TestValidateErrorsSurfaceAtParse(t *testing.T) {
	var a, b string
	cmd := NewCommand("test", "").Flags(
		String(&a, "foo", ""),
		String(&b, "foo", ""),
	)
	if cmd == nil {
		t.Fatal("expected non-nil command from construction")
	}
	assertParseError(t, cmd, "a tree only validated at Parse")
}

// TestValidateRunsOverSubcommands asserts that validation walks the whole
// tree: an invalid flag on a subcommand must fail a Parse issued on the
// root.
func TestValidateRunsOverSubcommands(t *testing.T) {
	var a, b string
	sub := NewCommand("sub", "").Flags(
		String(&a, "foo", ""),
		String(&b, "foo", ""),
	)
	root := NewCommand("root", "").Subcommands(sub)
	assertParseError(t, root, "an invalid subcommand reached from the root")
}

// *exec.ExitError implements ExitCoder without any help from this package,
// so a handler that shells out can return its error unchanged.
var _ ExitCoder = (*exec.ExitError)(nil)

// exitErrorHelperEnv asks the test binary re-executed by
// TestRunExitsWithChildProcessCode to exit with a known code rather than
// run the test suite.
const exitErrorHelperEnv = "XFLAGS_TEST_EXIT_HELPER"

// TestRunExitsWithChildProcessCode asserts the claim ExitCoder documents,
// against a real child process rather than the type assertion above: a
// handler that shells out can return the *exec.ExitError unchanged, and Run
// terminates with the child's exit code.
func TestRunExitsWithChildProcessCode(t *testing.T) {
	if os.Getenv(exitErrorHelperEnv) != "" {
		os.Exit(3) // this process is the child; see below
	}
	child := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	child.Env = append(os.Environ(), exitErrorHelperEnv+"=1")
	childErr := child.Run()
	var exitErr *exec.ExitError
	if !errors.As(childErr, &exitErr) {
		t.Fatalf("child process error = %v, want an *exec.ExitError", childErr)
	}

	cmd := NewCommand("test", "").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			return childErr
		})
	code, stdout, stderr := runCaptured(cmd)
	if got, want := code, 3; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	assertOutput(t, "stdout", stdout, "")
	assertOutput(t, "stderr", stderr, "Error: exit status 3\n")
}

// TestInvocationPath asserts that Parse reports the whole path of commands
// named by the arguments, root first, and the command they reached.
func TestInvocationPath(t *testing.T) {
	leaf := NewCommand("leaf", "")
	branch := NewCommand("branch", "").Subcommands(leaf)
	root := NewCommand("root", "").Subcommands(branch)

	inv, err := Parse(root, "branch", "leaf")
	if err != nil {
		t.Fatal(err)
	}
	assertString(t, "root branch leaf", inv.Cmd.FullName)
	if got, want := inv.Cmd.String(), leaf.String(); got != want {
		t.Errorf("Cmd = %v, want %v", got, want)
	}
}

// TestParseIsNotWrittenBack asserts that a parse leaves nothing behind on
// the command tree. Parsing one tree twice is what exposes a write-back,
// so this does deliberately what a program must not.
func TestParseIsNotWrittenBack(t *testing.T) {
	cmd := NewCommand("test", "").
		Flags(String(new(string), "name", ""))
	first, err := Parse(cmd, "--name=one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Parse(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(first.Sources), 1; got != want {
		t.Errorf("first parse sources = %d, want %d", got, want)
	}
	if got, want := len(second.Sources), 0; got != want {
		t.Errorf("second parse sources = %d, want %d", got, want)
	}
}

// TestRunExitCodes asserts the contract Run documents: 0 for success or
// help, 1 for a handler that failed, 2 for a command line that was wrong,
// and whatever an ExitCoder asks for. It also asserts which stream each
// outcome is reported on -- help on stdout, everything else on stderr.
func TestRunExitCodes(t *testing.T) {
	// handles returns a command whose handler returns err.
	handles := func(err error) *Command {
		return NewCommand("test", "").
			HelpFlag().
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				return err
			})
	}
	tests := []struct {
		name     string
		cmd      *Command
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "Success",
			cmd:      handles(nil),
			wantCode: 0,
		},
		{
			name:     "Help",
			cmd:      handles(nil),
			args:     []string{"--help"},
			wantCode: 0,
			wantOut:  "Usage: test [OPTIONS]\n",
		},
		{
			name:     "HandlerError",
			cmd:      handles(errors.New("boom")),
			wantCode: 1,
			wantErr:  "Error: boom\n",
		},
		{
			name:     "UnrecognizedArgument",
			cmd:      handles(nil),
			args:     []string{"--nope"},
			wantCode: 2,
			wantErr:  "Argument error: unrecognized option: --nope\nUsage: test [OPTIONS]\n",
		},
		{
			name:     "NoHandler",
			cmd:      NewCommand("test", ""),
			wantCode: 2,
			wantErr:  "Argument error: missing subcommand\nUsage: test\n",
		},
		{
			name: "ConfigError",
			cmd: NewCommand("test", "").
				Flags(
					String(new(string), "foo", ""),
					String(new(string), "foo", ""),
				),
			wantCode: 2,
			wantErr:  "Program error: test: flag declared more than once: --foo\n",
		},
		{
			// A tree that leads back into itself reports like any other
			// malformed tree, rather than wedging with no output at all.
			name: "CycleConfigError",
			cmd: func() *Command {
				cmd, sub := NewCommand("test", ""), NewCommand("sub", "")
				cmd.Subcommands(sub)
				sub.Subcommands(cmd)
				return cmd
			}(),
			wantCode: 2,
			wantErr:  "Program error: \"test\" is its own ancestor\n",
		},
		{
			name:     "Exit",
			cmd:      handles(Exit(3, errors.New("boom"))),
			wantCode: 3,
			wantErr:  "Error: boom\n",
		},
		{
			name:     "ExitWithoutError",
			cmd:      handles(Exit(3, nil)),
			wantCode: 3,
			wantErr:  "Error: exit status 3\n",
		},
		{
			name:     "Exitf",
			cmd:      handles(Exitf(3, "boom: %w", errors.New("kaboom"))),
			wantCode: 3,
			wantErr:  "Error: boom: kaboom\n",
		},
		{
			name:     "UsageError",
			cmd:      handles(Exitf(ExitCodeUsage, "--foo and --bar are exclusive")),
			wantCode: 2,
			wantErr:  "Error: --foo and --bar are exclusive\n",
		},
		{
			name:     "WrappedExitCoder",
			cmd:      handles(fmt.Errorf("child failed: %w", Exit(7, nil))),
			wantCode: 7,
			wantErr:  "Error: child failed: exit status 7\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runCaptured(tt.cmd, tt.args...)
			if got, want := code, tt.wantCode; got != want {
				t.Errorf("exit code = %d, want %d", got, want)
			}
			assertOutput(t, "stdout", stdout, tt.wantOut)
			assertOutput(t, "stderr", stderr, tt.wantErr)
		})
	}
}

// assertOutput asserts that a captured stream starts with want, or is empty
// if want is empty. Only the first line of a help message is worth
// asserting here; usage_test covers the rest.
func assertOutput(t *testing.T, name, got, want string) bool {
	t.Helper()
	if want == "" {
		if got != "" {
			t.Errorf("%s = %q, want nothing", name, got)
			return false
		}
		return true
	}
	if !strings.HasPrefix(got, want) {
		t.Errorf("%s = %q, want it to start with %q", name, got, want)
		return false
	}
	return true
}

// TestArgumentErrorsPrintUsage asserts that a wrong command line is
// reported with the usage of the command that the error names, on the same
// stderr stream, error line first. Help is not error reporting: it still
// prints on stdout alone and exits 0. See
// docs/adr/argument-errors-print-usage.md.
func TestArgumentErrorsPrintUsage(t *testing.T) {
	newCmd := func() *Command {
		sub := NewCommand("sub", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				return nil
			})
		return NewCommand("test", "").HelpFlag().Subcommands(sub)
	}
	t.Run("BadFlag", func(t *testing.T) {
		code, stdout, stderr := runCaptured(newCmd(), "sub", "--nope")
		if got, want := code, 2; got != want {
			t.Errorf("exit code = %d, want %d", got, want)
		}
		// The usage is the subcommand's, where the error happened, not the
		// root's, where Run was called.
		assertOutput(t, "stderr", stderr,
			"Argument error: unrecognized option: --nope\nUsage: test sub [OPTIONS]\n")
		assertOutput(t, "stdout", stdout, "")
	})
	t.Run("NoHandler", func(t *testing.T) {
		code, stdout, stderr := runCaptured(newCmd())
		if got, want := code, 2; got != want {
			t.Errorf("exit code = %d, want %d", got, want)
		}
		assertOutput(t, "stderr", stderr,
			"Argument error: missing subcommand\nUsage: test [OPTIONS] COMMAND\n")
		assertOutput(t, "stdout", stdout, "")
	})
	t.Run("Help", func(t *testing.T) {
		code, stdout, stderr := runCaptured(newCmd(), "--help")
		if got, want := code, 0; got != want {
			t.Errorf("exit code = %d, want %d", got, want)
		}
		assertOutput(t, "stdout", stdout, "Usage: test [OPTIONS] COMMAND\n")
		assertOutput(t, "stderr", stderr, "")
	})
	// The other two error classes stay one line: a handler error means the
	// command line was right, and a config error means the usage message
	// cannot be trusted.
	t.Run("HandlerError", func(t *testing.T) {
		cmd := NewCommand("test", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				return errors.New("boom")
			})
		code, _, stderr := runCaptured(cmd)
		if got, want := code, 1; got != want {
			t.Errorf("exit code = %d, want %d", got, want)
		}
		if got, want := stderr, "Error: boom\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
	t.Run("ConfigError", func(t *testing.T) {
		cmd := NewCommand("test", "").Flags(
			String(new(string), "foo", ""),
			String(new(string), "foo", ""),
		)
		code, _, stderr := runCaptured(cmd)
		if got, want := code, 2; got != want {
			t.Errorf("exit code = %d, want %d", got, want)
		}
		// A malformed tree prints no usage: it cannot describe itself.
		want := "Program error: test: flag declared more than once: --foo\n"
		if got := stderr; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
}

// TestDispatchReturnsRawError asserts Dispatch's half of the split from
// Run: the error comes back raw, with its Cmd naming the command that
// produced it, and no error text is printed. Help is the exception,
// because it is not error reporting: usage goes to stdout and Dispatch
// returns nil.
func TestDispatchReturnsRawError(t *testing.T) {
	newCmd := func() *Command {
		sub := NewCommand("sub", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				return nil
			})
		return NewCommand("test", "").HelpFlag().Subcommands(sub)
	}
	// dispatchCaptured is runCaptured for Dispatch.
	dispatchCaptured := func(cmd *Command, args ...string) (err error, stdout, stderr string) {
		var out, errOut strings.Builder
		err = Dispatch(context.Background(), cmd, WithArgs(args...),
			WithStdout(&out), WithStderr(&errOut))
		return err, out.String(), errOut.String()
	}
	// asArgumentError asserts that err carries an *ArgumentError naming the
	// command called wantCmd.
	asArgumentError := func(t *testing.T, err error, wantCmd string) {
		t.Helper()
		var argErr *ir.ArgumentError
		if !errors.As(err, &argErr) {
			t.Fatalf("err = %v, want *ArgumentError", err)
		}
		if argErr.Cmd == nil {
			t.Fatal("err.Cmd = nil, want the command that produced it")
		}
		if got, want := argErr.Cmd.String(), wantCmd; got != want {
			t.Errorf("err.Cmd = %q, want %q", got, want)
		}
	}
	t.Run("BadFlag", func(t *testing.T) {
		err, stdout, stderr := dispatchCaptured(newCmd(), "sub", "--nope")
		asArgumentError(t, err, "sub")
		assertOutput(t, "stdout", stdout, "")
		assertOutput(t, "stderr", stderr, "")
	})
	t.Run("NoHandler", func(t *testing.T) {
		err, stdout, stderr := dispatchCaptured(newCmd())
		asArgumentError(t, err, "test")
		assertOutput(t, "stdout", stdout, "")
		assertOutput(t, "stderr", stderr, "")
	})
	t.Run("Help", func(t *testing.T) {
		err, stdout, stderr := dispatchCaptured(newCmd(), "--help")
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
		assertOutput(t, "stdout", stdout, "Usage: test [OPTIONS] COMMAND\n")
		assertOutput(t, "stderr", stderr, "")
	})
	t.Run("HandlerError", func(t *testing.T) {
		boom := errors.New("boom")
		cmd := NewCommand("test", "").
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				return boom
			})
		err, stdout, stderr := dispatchCaptured(cmd)
		if got, want := err, boom; got != want {
			t.Errorf("err = %v, want %v", got, want)
		}
		assertOutput(t, "stdout", stdout, "")
		assertOutput(t, "stderr", stderr, "")
	})
}

// TestRunIgnoresWriteFailures asserts that a stream which cannot be
// written to costs the exit code and nothing else: there is nowhere left
// to report that reporting failed, so nothing is reported, as the flag
// package also does.
func TestRunIgnoresWriteFailures(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  *Command
		args []string
	}{
		{
			name: "Help",
			cmd: NewCommand("test", "").
				HelpFlag().
				HandleFunc(func(ctx context.Context, inv *Invocation) error {
					return nil
				}),
			args: []string{"--help"},
		},
		{
			name: "NoHandler",
			cmd:  NewCommand("test", ""),
		},
		{
			name: "UsageAfterAnError",
			cmd:  NewCommand("test", ""),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			procErr := captureStderr(t, func() {
				code = Run(context.Background(), tt.cmd, WithArgs(tt.args...),
					WithStdout(errWriter{}), WithStderr(errWriter{}))
			})
			if code == 0 {
				t.Errorf("exit code = 0, want non-zero")
			}
			if procErr != "" {
				t.Errorf("os.Stderr = %q, want nothing", procErr)
			}
		})
	}
}

// TestHandlerReceivesInvocation asserts that a handler is told how it was
// called: which command ran, and the path it was reached by.
func TestHandlerReceivesInvocation(t *testing.T) {
	var got *Invocation
	var remotes []string
	add := NewCommand("add", "").
		Flags(Strings(&remotes, "remote", "").Positional()).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			got = inv
			return nil
		})
	app := NewCommand("myapp", "").
		Subcommands(NewCommand("remote", "").Subcommands(add))

	args := []string{"remote", "add", "origin"}
	if code := Run(context.Background(), app, WithArgs(args...)); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got == nil {
		t.Fatal("handler was not called")
	}
	assertString(t, "myapp remote add", got.Cmd.FullName)
	assertStrings(t, []string{"origin"}, remotes)
	if want := add.String(); got.Cmd.String() != want {
		t.Errorf("Cmd = %v, want %v", got.Cmd, want)
	}
}

// TestParseReportsHelpAsAnInterrupt asserts that asking for help is reported
// on the Invocation rather than as an error, naming both the flag that asked
// and the subcommand whose help was asked for, so a caller doing its own
// dispatch can tell an interrupt apart from a failure.
func TestParseReportsHelpAsAnInterrupt(t *testing.T) {
	add := NewCommand("add", "")
	app := NewCommand("myapp", "").HelpFlag().Subcommands(add)

	inv, err := Parse(app, "add", "--help")
	if err != nil {
		t.Fatalf("Parse() = %v, want no error", err)
	}
	if inv.Interrupt == nil {
		t.Fatal("Interrupt = nil, want the help flag")
	}
	if got, want := inv.Interrupt.String(), "--help"; got != want {
		t.Errorf("Interrupt = %v, want %v", got, want)
	}
	if want := add.String(); inv.Cmd.String() != want {
		t.Errorf("Cmd = %v, want %v", inv.Cmd, want)
	}
}

// TestInterruptRunsInPlaceOfTheHandler asserts that naming an interrupt
// runs it rather than the command the arguments reached, and that the
// invocation it is given names that command -- a persistent interrupt
// reports on whichever command it was written after, not on the one that
// declared it.
func TestInterruptRunsInPlaceOfTheHandler(t *testing.T) {
	var ran string
	sub := NewCommand("sub", "").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			ran = "handler"
			return nil
		})
	cmd := NewCommand("test", "").
		Flags(Unbound("where", "").Interrupt(func(ctx context.Context, inv *Invocation) error {
			ran = inv.Cmd.FullName
			return nil
		}).Persistent()).
		Subcommands(sub)

	if code, _, stderr := runCaptured(cmd, "sub", "--where"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if got, want := ran, "test sub"; got != want {
		t.Errorf("ran = %q, want %q", got, want)
	}
}

// TestInterruptTakesNoArgument asserts that an interrupt is given by name
// alone: it binds no value, so an attached one is a malformed token rather
// than something to set, and it has no negated spelling to answer to.
func TestInterruptTakesNoArgument(t *testing.T) {
	newCmd := func() *Command {
		return NewCommand("test", "").
			Flags(Unbound("where", "").Interrupt(func(ctx context.Context, inv *Invocation) error {
				return nil
			}))
	}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"Attached", []string{"--where=x"}, "option takes no argument: --where"},
		{"Negated", []string{"--no-where"}, "unrecognized option: --no-where"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(newCmd(), tt.args...)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if got := humanMessage(err); got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHelpFlagNames asserts what Command.HelpFlag mounts: the two usual
// names when given none, and only what it was given otherwise, which is how
// a program keeps "-h" for something of its own.
func TestHelpFlagNames(t *testing.T) {
	t.Run("DefaultsToHelpAndH", func(t *testing.T) {
		for _, arg := range []string{"--help", "-h"} {
			inv, err := Parse(NewCommand("test", "").HelpFlag(), arg)
			if err != nil {
				t.Fatalf("Parse(%q) = %v, want no error", arg, err)
			}
			if inv.Interrupt == nil {
				t.Errorf("Parse(%q): Interrupt = nil, want the help flag", arg)
			}
		}
	})
	t.Run("NamedLongOnly", func(t *testing.T) {
		var host string
		cmd := NewCommand("test", "").
			HelpFlag("help").
			Flags(String(&host, "host", "").Aliases("h"))

		if _, err := Parse(cmd, "-h", "example.com"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := host, "example.com"; got != want {
			t.Errorf("host = %q, want %q", got, want)
		}
	})
	t.Run("NoneUnlessMounted", func(t *testing.T) {
		_, err := Parse(NewCommand("test", ""), "--help")
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if got, want := humanMessage(err), "unrecognized option: --help"; got != want {
			t.Errorf("message = %q, want %q", got, want)
		}
	})
}

// TestHelpSkipsFlagRules asserts that help is reported for a command line the
// user has not finished writing. Parsing stops at -h or --help, so a required
// flag that was never given is not held against them -- help is most useful
// to someone who does not yet know what to type.
func TestHelpSkipsFlagRules(t *testing.T) {
	var stdout strings.Builder
	cmd := NewCommand("test", "").
		HelpFlag().
		Flags(String(new(string), "name", "").Required()).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			t.Error("handler was called")
			return nil
		})

	code := Run(context.Background(), cmd, WithArgs("--help"), WithStdout(&stdout))
	if got, want := code, 0; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	assertOutput(t, "stdout", stdout.String(), "Usage: test")
}

// TestHandlerStreams asserts that a handler reads and writes the streams on
// its invocation, and that they are resolved from wherever the command is
// mounted: redirecting the root captures what a subcommand's handler prints,
// which is most of what a CLI emits.
func TestHandlerStreams(t *testing.T) {
	echo := NewCommand("echo", "").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			if _, err := io.Copy(inv.Stdout, inv.Stdin); err != nil {
				return err
			}
			fmt.Fprint(inv.Stderr, "echoed")
			return nil
		})
	app := NewCommand("app", "").Subcommands(echo)

	var stdout, stderr strings.Builder
	code := Run(context.Background(), app, WithArgs("echo"),
		WithStdin(strings.NewReader("hello")),
		WithStdout(&stdout), WithStderr(&stderr))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	assertString(t, "hello", stdout.String())
	assertString(t, "echoed", stderr.String())
}

// TestUsageFuncIsInherited asserts that a custom renderer set on one
// command serves its subcommands too, and that the command it is handed is
// the one being described rather than the one that set it: a subcommand
// that sets no renderer carries its nearest ancestor's.
func TestUsageFuncIsInherited(t *testing.T) {
	var stdout strings.Builder
	root := NewCommand("root", "Root summary").
		HelpFlag().
		UsageFunc(func(w io.Writer, cmd *ir.Command) error {
			_, err := fmt.Fprintf(w, "custom help for %s\n", cmd.FullName)
			return err
		}).
		Subcommands(
			NewCommand("child", "Child summary").
				HandleFunc(func(ctx context.Context, inv *Invocation) error {
					return nil
				}),
		)

	code := Run(context.Background(), root, WithArgs("child", "--help"), WithStdout(&stdout))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got, want := stdout.String(), "custom help for root child\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func ExampleInvocation() {
	// A team writes this command without knowing where it will be mounted,
	// so it reads its own name out of the invocation rather than repeating
	// it in the message.
	add := NewCommand("add", "Add a remote").
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			return Exitf(ExitCodeUsage,
				"no remote named: try \"%s --help\"",
				inv.Cmd.FullName,
			)
		})

	// Whoever composes the binary decides where it hangs.
	app := NewCommand("myapp", "").
		Subcommands(NewCommand("remote", "Manage remotes").Subcommands(add))

	code := Run(context.Background(), app, WithArgs("remote", "add"),
		WithStderr(os.Stdout)) // for tests
	fmt.Println("exit code:", code)
	// Output:
	// Error: no remote named: try "myapp remote add --help"
	// exit code: 2
}

// TestValidateDefaultNotAmongChoices asserts that a default outside the
// declared choices is a configuration error. Defaults bypass Set, so such
// a default would survive parsing and be advertised by help as a value the
// same program rejects on the command line.
func TestValidateDefaultNotAmongChoices(t *testing.T) {
	var env string
	cmd := NewCommand("test", "").Flags(
		String(&env, "env", "").Default("bogus").Choices("staging", "production"),
	)
	_, err := Parse(cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err),
		`--env: default "bogus" is not one of: staging, production`; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestValidateEmptyDefaultWithChoices asserts that an empty default is not
// held to the choices: it is how a flag says it has no default, and it is
// what every Required choice flag declares.
func TestValidateEmptyDefaultWithChoices(t *testing.T) {
	var env string
	cmd := NewCommand("test", "").Flags(
		String(&env, "env", "").Choices("staging", "production").Required(),
	)
	if _, err := Parse(cmd, "--env=staging"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "staging", env)
}

// TestValidateRepeatableDefaultWithChoices asserts that a repeatable flag
// escapes the default-among-choices rule: it accumulates, so its default
// renders as the whole collection rather than as a value any one choice
// could match.
func TestValidateRepeatableDefaultWithChoices(t *testing.T) {
	var tags []string
	cmd := NewCommand("test", "").Flags(
		Strings(&tags, "tag", "").Choices("red", "blue"),
	)
	if _, err := Parse(cmd, "--tag=red", "--tag=blue"); err != nil {
		t.Fatal(err)
	}
}

// TestValidatePositionalAlias asserts that a positional argument takes no
// alias. One would be a name nothing could match, since a positional never
// enters the option table, so it is reported rather than ignored.
func TestValidatePositionalAlias(t *testing.T) {
	var s string
	_, err := Parse(NewCommand("test", "").Flags(
		String(&s, "file", "").Aliases("f").Positional(),
	))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err), "FILE: positional arguments do not support aliases"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestValidateFlagWithoutName asserts that a flag of nothing but empty
// slots is rejected: it can never be named on the command line, and has no
// canonical name for an error to report it by.
func TestValidateFlagWithoutName(t *testing.T) {
	var s string
	_, err := Parse(NewCommand("test", "").Flags(
		String(&s, "", "").Aliases(""),
	))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err), "unknown: flag must declare a name"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestNegationCollision asserts what generating a spelling costs: a
// program can now collide with a name that appears nowhere in its source.
// The error has to say where the spelling came from, or the author is
// left looking for a declaration that does not exist.
func TestNegationCollision(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  *Command
		want string
	}{
		{
			name: "GeneratedAgainstDeclared",
			cmd: NewCommand("app", "").Flags(
				Bool(new(bool), "cache", ""),
				Bool(new(bool), "no-cache", ""),
			),
			want: "app: flag declared more than once: --no-cache (generated from --cache)",
		},
		{
			name: "GeneratedAgainstDeclaredValueFlag",
			cmd: NewCommand("app", "").Flags(
				Bool(new(bool), "cache", ""),
				String(new(string), "no-cache", ""),
			),
			want: "app: flag declared more than once: --no-cache (generated from --cache)",
		},
		{
			name: "GeneratedAgainstAncestor",
			cmd: NewCommand("root", "").
				Flags(Bool(new(bool), "cache", "").Persistent()).
				Subcommands(NewCommand("sub", "").Flags(
					Bool(new(bool), "no-cache", ""),
				)),
			want: `root sub: flag declared on both "root" and "sub": --no-cache (generated from --cache)`,
		},
		{
			// Two booleans that collide on a declared name collide on its
			// negation too, but that is one mistake, so it is reported
			// once and by the name the author actually wrote.
			name: "ShadowCollisionIsReportedOnce",
			cmd: NewCommand("app", "").Flags(
				Bool(new(bool), "force", "").Aliases("f"),
				Bool(new(bool), "force", "").Aliases("g"),
			),
			want: "app: flag declared more than once: --force",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.cmd)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got, want := humanMessage(err), tt.want; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

// TestOperandDoesNotCollideWithOption asserts what a positional stopped
// claiming when the collision check moved to value names: it answers to
// no option, so it cannot shadow one. A command taking a SERVICE operand
// may also declare --service, since the two share no spelling anywhere a
// reader sees them.
func TestOperandDoesNotCollideWithOption(t *testing.T) {
	var operand, option string
	_, err := Parse(NewCommand("test", "").Flags(
		String(&operand, "service", "").Positional(),
		String(&option, "service", ""),
	), "web")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := operand, "web"; got != want {
		t.Errorf("operand = %q, want %q", got, want)
	}
}

// TestDuplicateValueNameCollides asserts the collision a positional does
// still have: two operands shown by the same value name, which no error
// message could tell apart. An explicit ValueName is enough, since what a
// reader sees is the whole of the ambiguity.
func TestDuplicateValueNameCollides(t *testing.T) {
	var a, b string
	_, err := Parse(NewCommand("test", "").Flags(
		String(&a, "src", "").Positional().ValueName("PATH"),
		String(&b, "dst", "").Positional().ValueName("PATH"),
	))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := humanMessage(err), "test: operand declared more than once: PATH"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestUsageFuncIsInheritedAtCompileTime asserts that a command carries the
// renderer it will be printed with, rather than the Usage method going
// looking for one up the tree: a subcommand that sets none compiles with
// its nearest ancestor's, and one that sets its own keeps it.
func TestUsageFuncIsInheritedAtCompileTime(t *testing.T) {
	rootFunc := func(w io.Writer, cmd *ir.Command) error {
		_, err := io.WriteString(w, "root renderer\n")
		return err
	}
	leafFunc := func(w io.Writer, cmd *ir.Command) error {
		_, err := io.WriteString(w, "leaf renderer\n")
		return err
	}
	own := NewCommand("own", "").UsageFunc(leafFunc)
	inherits := NewCommand("inherits", "").Subcommands(
		NewCommand("deep", ""),
	)
	node, err := NewCommand("app", "").UsageFunc(rootFunc).
		Subcommands(own, inherits).Compile()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		path []string
		want string
	}{
		{nil, "root renderer\n"},
		{[]string{"own"}, "leaf renderer\n"},
		{[]string{"inherits"}, "root renderer\n"},
		{[]string{"inherits", "deep"}, "root renderer\n"}, // two levels up
	} {
		t.Run(strings.Join(append([]string{"app"}, tt.path...), " "), func(t *testing.T) {
			cmd := node
			for _, name := range tt.path {
				for _, sub := range cmd.Subcommands {
					if sub.Name == name {
						cmd = sub
					}
				}
			}
			if cmd.UsageFunc == nil {
				t.Fatal("UsageFunc was not resolved at compile time")
			}
			var buf strings.Builder
			if err := cmd.Usage(&buf); err != nil {
				t.Fatal(err)
			}
			if got := buf.String(); got != tt.want {
				t.Errorf("Usage() = %q, want %q", got, tt.want)
			}
		})
	}

	// A tree that sets none leaves it nil, so Usage falls back.
	bare, err := NewCommand("bare", "").Compile()
	if err != nil {
		t.Fatal(err)
	}
	if bare.UsageFunc != nil {
		t.Error("UsageFunc = non-nil, want nil so Usage falls back to the default")
	}
}

// TestAncestryIsResolvedAtCompileTime asserts that each compiled command
// carries the commands whose flags are in scope at it, from the root down,
// so nothing reading the tree walks back up to work it out. Siblings must
// not share a backing array: appending to the parent's slice in place
// would let the second subcommand overwrite the first.
func TestAncestryIsResolvedAtCompileTime(t *testing.T) {
	node, err := NewCommand("app", "").Subcommands(
		NewCommand("one", "").Subcommands(NewCommand("deep", "")),
		NewCommand("two", ""),
	).Compile()
	if err != nil {
		t.Fatal(err)
	}

	names := func(c *ir.Command) []string {
		var out []string
		for _, a := range c.Ancestry {
			out = append(out, a.Name)
		}
		return out
	}
	one, two := node.Subcommands[0], node.Subcommands[1]
	deep := one.Subcommands[0]

	for _, tt := range []struct {
		cmd  *ir.Command
		want []string
	}{
		{node, []string{"app"}},
		{one, []string{"app", "one"}},
		{two, []string{"app", "two"}},
		{deep, []string{"app", "one", "deep"}},
	} {
		if got := names(tt.cmd); !slices.Equal(got, tt.want) {
			t.Errorf("%s.Ancestry = %v, want %v", tt.cmd.Name, got, tt.want)
		}
	}

	// The ancestry ends with the command itself, and begins at its root.
	if got, want := deep.Ancestry[len(deep.Ancestry)-1], deep; got != want {
		t.Errorf("last of Ancestry = %v, want the command itself", got)
	}
	if got, want := deep.Ancestry[0], node; got != want {
		t.Errorf("first of Ancestry = %v, want the root", got)
	}
}

// TestRunCompilesOnce pins that an ordinary invocation lowers the tree
// exactly once, on both the success and the failure path. Before Run
// compiled once and passed the node down, the completion hook and the
// error reporter each compiled again for themselves.
//
// The count comes from a middleware, because Compile applies every
// wrapper on a command's path while lowering it, so one application per
// command is one compile. That is the contract Command.Middleware
// documents, which is what makes it a fair probe rather than a trick.
func TestRunCompilesOnce(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		// EnableCompletion is what made an ordinary invocation compile a
		// second time, for a variable that is not even set.
		{name: "success", args: nil, want: ExitCodeSuccess},
		// The error path compiled again to find the stream to report on.
		{name: "argument error", args: []string{"--nope"}, want: ExitCodeUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied := 0
			app := NewCommand("app", "").
				EnableCompletion().
				Middleware(func(next HandlerFunc) HandlerFunc {
					applied++
					return next
				}).
				HandleFunc(func(ctx context.Context, inv *Invocation) error {
					return nil
				})

			code := Run(context.Background(), app, WithArgs(tt.args...),
				WithStdout(io.Discard), WithStderr(io.Discard))
			if code != tt.want {
				t.Errorf("exit code = %d, want %d", code, tt.want)
			}
			if applied != 1 {
				t.Errorf("tree lowered %d times, want 1", applied)
			}
		})
	}
}

// TestInterruptCommandRunsBare asserts the constructor's whole contract in
// one line: under a root with a required flag and middleware, the
// interrupt answers without the flag and outside the wrapper.
func TestInterruptCommandRunsBare(t *testing.T) {
	var wrapped, ran bool
	root := NewCommand("test", "").
		Flags(String(new(string), "name", "").Required()).
		Middleware(func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, inv *Invocation) error {
				wrapped = true
				return next(ctx, inv)
			}
		}).
		Subcommands(InterruptCommand("about", "", func(ctx context.Context, inv *Invocation) error {
			ran = true
			return nil
		}))
	if code, _, stderr := runCaptured(root, "about"); code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !ran {
		t.Error("the interrupt did not run")
	}
	if wrapped {
		t.Error("middleware wrapped the interrupt")
	}
}

// TestInterruptOrHandlerNotBoth asserts that a command carrying both an
// interrupt and a handler is a configuration error: the callback is the
// marker, and only one can answer.
func TestInterruptOrHandlerNotBoth(t *testing.T) {
	noop := func(ctx context.Context, inv *Invocation) error { return nil }
	cmd := InterruptCommand("about", "", noop).HandleFunc(noop)
	if _, err := NewCommand("test", "").Subcommands(cmd).Compile(); err == nil {
		t.Fatal("Compile succeeded, want a configuration error")
	} else if got, want := err.Error(), "a command is an interrupt or has a handler, not both"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}
}
