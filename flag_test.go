package climux

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"go.hotsrc.dev/climux/ir"
)

func TestBitField(t *testing.T) {
	var v uint64
	_, err := Parse(
		NewCommand("test", "").
			Flags(
				BitField(&v, 0x01, "foo", ""),
				BitField(&v, 0x02, "bar", ""),
				BitField(&v, 0x04, "baz", "").Default(true),
			),
		"--foo",
	)
	if err != nil {
		t.Fatal(err)
	}
	assertInt64(t, 0x05, int64(v))
}

// TestStringsFirstNamingReplacesDefault asserts the rule every VarType
// is written against: the first naming finds the zero value, not the
// default, so a slice flag's default is what it holds when never named
// and nothing else.
func TestStringsFirstNamingReplacesDefault(t *testing.T) {
	var v []string
	if assertFlagParses(t, Strings(&v, "foo", "").Default([]string{"stable"}), "--foo=a", "--foo=b") {
		assertStrings(t, []string{"a", "b"}, v)
	}
	if assertFlagParses(t, Strings(&v, "foo", "").Default([]string{"stable"})) {
		assertStrings(t, []string{"stable"}, v)
	}
}

// labelsType accumulates KEY=VALUE arguments into a map, and carries no
// state to do it: the variable is zero on first naming, so the nil check
// is where the map initialises, and every later naming folds into it.
type labelsType struct{}

func (labelsType) Kind() ir.Kind                     { return ir.KindString }
func (labelsType) Format(v map[string]string) string { return fmt.Sprint(v) }
func (labelsType) Decode(v *map[string]string, s string) error {
	k, val, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("want KEY=VALUE, got %q", s)
	}
	if *v == nil {
		*v = map[string]string{}
	}
	(*v)[k] = val
	return nil
}

// TestVarAccumulates asserts that a VarType needs no state to accumulate.
func TestVarAccumulates(t *testing.T) {
	var labels map[string]string
	flag := Var(&labels, "label", "", labelsType{}).NArgs(0, 0)
	if assertFlagParses(t, flag, "--label=a=1", "--label=b=2") {
		if got, want := fmt.Sprint(labels), "map[a:1 b:2]"; got != want {
			t.Errorf("labels = %s, want %s", got, want)
		}
	}
}

// yesNoType reports IsBoolFlag, so a fixture can assert that a custom
// type's flag stands alone on the command line as Bool's does.
type yesNoType struct{}

func (yesNoType) IsBoolFlag() bool       { return true }
func (yesNoType) Kind() ir.Kind          { return ir.KindString }
func (yesNoType) Format(v string) string { return v }

func (yesNoType) Decode(v *string, s string) error {
	switch s {
	case "true":
		*v = "yes"
	case "false":
		*v = "no"
	default:
		return fmt.Errorf("want true or false, got %q", s)
	}
	return nil
}

// TestVarIsBoolFlag asserts that a type with IsBoolFlag makes a flag
// that takes no argument when named alone.
func TestVarIsBoolFlag(t *testing.T) {
	var answer string
	if assertFlagParses(t, Var(&answer, "confirm", "", yesNoType{}), "--confirm") {
		if got, want := answer, "yes"; got != want {
			t.Errorf("answer = %q, want %q", got, want)
		}
	}
}

// TestUintRejectsNegative asserts that an unsigned flag refuses a
// negative argument rather than wrapping it.
func TestUintRejectsNegative(t *testing.T) {
	var u uint
	if _, err := Parse(NewCommand("test", "").Flags(Uint(&u, "n", "")), "--n=-1"); err == nil {
		t.Error("Uint accepted -1")
	}
	var u64 uint64
	if _, err := Parse(NewCommand("test", "").Flags(Uint64(&u64, "n", "")), "--n=-1"); err == nil {
		t.Error("Uint64 accepted -1")
	}
}

// TestDefault asserts that a variable is written once per parse or not
// at all: declaring writes nothing; parsing writes what the line says,
// else the declared default, else nothing.
func TestDefault(t *testing.T) {
	s := "stale"
	flag := String(&s, "output", "").Default("json")
	if got, want := s, "stale"; got != want {
		t.Errorf("after declaring, s = %q, want %q", got, want)
	}
	if got, want := flag.defValue, "json"; got != want {
		t.Errorf("defValue = %q, want %q", got, want)
	}
	if assertFlagParses(t, flag) {
		if got, want := s, "json"; got != want {
			t.Errorf("unnamed, after parsing s = %q, want the default %q", got, want)
		}
	}
	if assertFlagParses(t, String(&s, "output", "").Default("json"), "--output=yaml") {
		if got, want := s, "yaml"; got != want {
			t.Errorf("named, after parsing s = %q, want %q", got, want)
		}
	}
	s = "stale"
	if assertFlagParses(t, String(&s, "output", "")) {
		if got, want := s, "stale"; got != want {
			t.Errorf("no Default and unnamed, after parsing s = %q, want it untouched", got)
		}
	}
}

func TestBool(t *testing.T) {
	v := false
	if assertFlagParses(t, Bool(&v, "foo", ""), "--foo") {
		assertBool(t, true, v)
	}
}

func TestDuration(t *testing.T) {
	var v time.Duration
	if assertFlagParses(t, Duration(&v, "foo", ""), "--foo=1s") {
		assertDuration(t, time.Second, v)
	}
	if assertFlagParses(t, Duration(&v, "foo", ""), "--foo=-1s") {
		assertDuration(t, -time.Second, v)
	}
}

func TestFloat64(t *testing.T) {
	var v float64
	if assertFlagParses(t, Float64(&v, "foo", ""), "--foo=1.0") {
		assertFloat64(t, 1.0, v)
	}
	if assertFlagParses(t, Float64(&v, "foo", ""), "--foo=-1.0") {
		assertFloat64(t, -1.0, v)
	}
}

func TestInt64(t *testing.T) {
	var v int64
	if assertFlagParses(t, Int64(&v, "foo", ""), "--foo=1") {
		assertInt64(t, 1, v)
	}
	if assertFlagParses(t, Int64(&v, "foo", ""), "--foo=-1") {
		assertInt64(t, -1, v)
	}
}

func TestString(t *testing.T) {
	var v string
	if assertFlagParses(t, String(&v, "foo", ""), "--foo=bar") {
		assertString(t, "bar", v)
	}
}

func TestStringSlice(t *testing.T) {
	var v []string
	if assertFlagParses(
		t,
		Strings(&v, "foo", ""),
		"--foo", "baz", "--foo", "qux",
	) {
		assertStrings(t, []string{"baz", "qux"}, v)
	}
}

func TestFunc(t *testing.T) {
	var v []string
	fn := func(s string) error {
		v = append(v, s)
		return nil
	}
	if assertFlagParses(
		t,
		Func("foo", "", fn),
		"--foo", "baz", "--foo", "qux",
	) {
		assertStrings(t, []string{"baz", "qux"}, v)
	}
}

func TestFuncError(t *testing.T) {
	fn := func(s string) error { return fmt.Errorf("nope: %s", s) }
	assertArgumentError(t, parseFlag(Func("foo", "", fn), "--foo=bar"))
}

func TestFlagChoices(t *testing.T) {
	var v string
	flag := String(&v, "foo", "").Choices("bar", "baz")
	assertFlagParses(t, flag, "--foo=bar")
	assertFlagParses(t, flag, "--foo=baz")
	assertArgumentError(t, parseFlag(flag, "--foo=qux"))
	assertArgumentError(t, parseFlag(flag, "--foo=ba"))
	assertArgumentError(t, parseFlag(flag, "--foo=barr"))
}

func ExampleFlagBuilder_Validate() {
	var ip string

	cmd := NewCommand("ping", "").
		Flags(
			String(&ip, "ip", "IP Address to ping").Default("127.0.0.1").
				Validate(func(arg string) error {
					if net.ParseIP(arg) == nil {
						return fmt.Errorf("invalid IP: %s", arg)
					}
					return nil
				}),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "ping: %s\n", ip)
			return nil
		})

	ctx := context.Background()
	Run(ctx, cmd, WithArgs("--ip=127.0.0.1"), WithStderr(os.Stdout))

	// 256 is not a valid IPv4 component
	Run(ctx, cmd, WithArgs("--ip=256.0.0.1"), WithStderr(os.Stdout))
	// Output:
	// ping: 127.0.0.1
	// Argument error: --ip: invalid IP: 256.0.0.1
	// Usage: ping [OPTIONS]
	//
	// Options:
	//    --ip  IP Address to ping
}

func ExampleBitField() {
	const (
		UserRead    uint64 = 0400
		UserWrite   uint64 = 0200
		UserExecute uint64 = 0100
	)

	var mode uint64 = 0444 // -r--r--r--

	cmd := NewCommand("user-allow", "").
		Flags(
			BitField(&mode, UserRead, "r", "Enable user read"),
			BitField(&mode, UserWrite, "w", "Enable user write"),
			BitField(&mode, UserExecute, "x", "Enable user execute"),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "File mode: %s\n", os.FileMode(mode))
			return nil
		})

	// Enable user read and write
	Run(context.Background(), cmd, WithArgs("-r", "-w"), WithStderr(os.Stdout))
	// Output: File mode: -rw-r--r--
}

func ExampleCommand_VersionFlag() {
	// What a build stamps into the binary. Both spellings print it, so
	// the two can never disagree.
	const version = "1.4.2"

	cmd := NewCommand("orbital", "Deploy and operate services").
		VersionFlag(version).
		VersionCommand(version)

	ctx := context.Background()
	Run(ctx, cmd, WithArgs("--version"), WithStderr(os.Stdout))
	Run(ctx, cmd, WithArgs("version"), WithStderr(os.Stdout))
	// Output:
	// orbital 1.4.2
	// orbital 1.4.2
}

func ExampleFunc() {
	var ip net.IP

	cmd := NewCommand("ping", "").
		Flags(
			Func("ip", "IP address to ping", func(s string) error {
				ip = net.ParseIP(s)
				if ip == nil {
					return fmt.Errorf("invalid IP: %s", s)
				}
				return nil
			}),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "ping: %s\n", ip)
			return nil
		})

	ctx := context.Background()
	Run(ctx, cmd, WithArgs("--ip", "127.0.0.1"), WithStderr(os.Stdout))

	// 256 is not a valid IPv4 component
	Run(ctx, cmd, WithArgs("--ip", "256.0.0.1"), WithStderr(os.Stdout))
	// Output:
	// ping: 127.0.0.1
	// Argument error: --ip: invalid IP: 256.0.0.1
	// Usage: ping [OPTIONS]
	//
	// Options:
	//    --ip  IP address to ping
}

func ExampleStrings() {
	var widgets []string

	cmd := NewCommand("create-widgets", "").
		Flags(
			// Configure a repeatable string slice flag that must be specified
			// at least once.
			Strings(&widgets, "name", "Widget name").NArgs(1, 0),
		).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Printf("Created new widgets: %s", strings.Join(widgets, ", "))
			return nil
		})

	Run(context.Background(), cmd, WithArgs("--name=foo", "--name=bar"), WithStderr(os.Stdout))
	// Output: Created new widgets: foo, bar
}

// TestFlagGroupStandalone asserts that a group built with NewFlagGroup and
// mounted with Command.FlagGroups parses and describes exactly like one
// declared inline with Command.FlagGroup.
func TestFlagGroupStandalone(t *testing.T) {
	var level, format string
	group := NewFlagGroup(
		"logging", "Logging options",
		String(&level, "log-level", "Set log verbosity").Default("info"),
	).Flags(
		String(&format, "log-format", "Log output format").Default("text"),
	)
	cmd := NewCommand("test", "").FlagGroups(group)

	if _, err := Parse(cmd, "--log-level=debug", "--log-format=json"); err != nil {
		t.Fatal(err)
	}
	assertString(t, "debug", level)
	assertString(t, "json", format)

	node, err := cmd.Compile()
	if err != nil {
		t.Fatal(err)
	}
	// The implicit "options" group is first; the mounted group follows.
	if got, want := len(node.FlagGroups), 2; got != want {
		t.Fatalf("len(FlagGroups) = %d, want %d", got, want)
	}
	if got, want := node.FlagGroups[1].Title, "Logging options"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
	if got, want := len(node.FlagGroups[1].Flags), 2; got != want {
		t.Errorf("len(Flags) = %d, want %d", got, want)
	}
}

// TestCompileFlag asserts that every field of a flag configured with the
// chained setters is described on its ir.Flag counterpart, and that
// behavior (the Value, the ValidateFunc) is dropped.
func TestCompileFlag(t *testing.T) {
	var s string
	flg := String(&s, "name", "flag usage").Default("default-value").
		Aliases("n").
		NArgs(1, 3).
		Hidden().
		ShowDefault().
		Env("MY_ENV").
		Choices("red", "blue")
	cmd := NewCommand("test", "").Flags(flg)

	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := len(node.FlagGroups), 1; got != want {
		t.Fatalf("len(FlagGroups) = %d, want %d", got, want)
	}
	if got, want := len(node.FlagGroups[0].Flags), 1; got != want {
		t.Fatalf("len(FlagGroups[0].Flags) = %d, want %d", got, want)
	}
	df := node.FlagGroups[0].Flags[0]
	if got, want := df.String(), "--name"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got, want := strings.Join(df.NamedOptions, ","), "--name,-n"; got != want {
		t.Errorf("NamedOptions = %q, want %q", got, want)
	}
	if got, want := df.Usage, "flag usage"; got != want {
		t.Errorf("Usage = %q, want %q", got, want)
	}
	if got, want := df.Default, "default-value"; got != want {
		t.Errorf("Default = %q, want %q", got, want)
	}
	if got, want := df.ShowDefault, true; got != want {
		t.Errorf("ShowDefault = %v, want %v", got, want)
	}
	if got, want := df.Positional, false; got != want {
		t.Errorf("Positional = %v, want %v", got, want)
	}
	if got, want := df.Hidden, true; got != want {
		t.Errorf("Hidden = %v, want %v", got, want)
	}
	if got, want := df.MinCount, 1; got != want {
		t.Errorf("MinCount = %d, want %d", got, want)
	}
	if got, want := df.MaxCount, 3; got != want {
		t.Errorf("MaxCount = %d, want %d", got, want)
	}
	if got, want := df.EnvVar, "MY_ENV"; got != want {
		t.Errorf("EnvVar = %q, want %q", got, want)
	}
	assertStrings(t, []string{"red", "blue"}, df.Choices)
}

// TestCompileTakesValue asserts that a flag's TakesValue is derived from
// whether its Value is a BoolValue: a bool flag stands alone on the command
// line, everything else needs an argument.
func TestCompileTakesValue(t *testing.T) {
	var s string
	var b bool
	cmd := NewCommand("test", "").Flags(
		String(&s, "name", ""),
		Bool(&b, "verbose", ""),
	)
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	flags := node.FlagGroups[0].Flags
	if got, want := flags[0].TakesValue, true; got != want {
		t.Errorf("String: TakesValue = %v, want %v", got, want)
	}
	if got, want := flags[1].TakesValue, false; got != want {
		t.Errorf("Bool: TakesValue = %v, want %v", got, want)
	}
}

// TestCompilePositional asserts that Positional is described too, using a
// separate command since a single command cannot mix positional flags with
// the option above without also adding subcommands (which is itself
// disallowed alongside positionals).
func TestCompilePositional(t *testing.T) {
	var s string
	flg := String(&s, "ARG", "positional usage").Positional()
	cmd := NewCommand("test", "").Flags(flg)

	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	df := node.FlagGroups[0].Flags[0]
	if got, want := df.Positional, true; got != want {
		t.Errorf("Positional = %v, want %v", got, want)
	}
}

// TestCompileValueName asserts what a flag's value is called: the flag's
// own name by default, the override where one was given, and nothing at
// all for a flag that takes no value.
func TestCompileValueName(t *testing.T) {
	var s, o, forced string
	var b bool
	var tags []string
	cmd := NewCommand("test", "").Flags(
		String(&s, "name", "usage"),
		String(&o, "output", "usage").ValueName("path"),
		Bool(&b, "verbose", "usage"),
		String(&forced, "log-level", "usage"),
		Strings(&tags, "tags", "usage").ValueName("tag"),
	)
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, want := range []string{"NAME", "PATH", "", "LOG_LEVEL", "TAG"} {
		if got := node.FlagGroups[0].Flags[i].ValueName; got != want {
			t.Errorf("Flags[%d].ValueName = %q, want %q", i, got, want)
		}
	}
}

// TestCompileName asserts that a flag's Name is its declared canonical
// name: undecorated by the dialect, unaffected by any alias, and, for a
// positional argument, the name it was declared with like any other flag.
func TestCompileName(t *testing.T) {
	var s, arg string
	cmd := NewCommand("test", "").Flags(
		String(&s, "name", "").Aliases("n", "alias"),
		String(&arg, "ARG", "").Positional(),
	)
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := node.FlagGroups[0].Flags[0].Name, "name"; got != want {
		t.Errorf("Flags[0].Name = %q, want %q", got, want)
	}
	if got, want := node.FlagGroups[0].Flags[1].Name, "ARG"; got != want {
		t.Errorf("Flags[1].Name = %q, want %q", got, want)
	}
}

// kindStringType claims ir.KindString, so a fixture can assert that Var
// reports the Kind its type declares.
type kindStringType struct{}

func (kindStringType) Decode(v *string, s string) error { *v = s; return nil }
func (kindStringType) Format(v string) string           { return v }
func (kindStringType) Kind() ir.Kind                    { return ir.KindString }

// TestCompileKind asserts what ir.Flag.Kind is set to by each typed
// constructor, that Var reports what its VarType declares, and that an
// interrupt, binding no value, has none.
func TestCompileKind(t *testing.T) {
	var (
		bo  bool
		bf  uint64
		du  time.Duration
		fl  float64
		in  int
		i64 int64
		s   string
		ss  []string
		u   uint
		u64 uint64
		ks  string
		ip  net.IP
	)
	cmd := NewCommand("test", "").Flags(
		Bool(&bo, "bool", ""),
		BitField(&bf, 0x1, "bitfield", ""),
		Duration(&du, "duration", ""),
		Float64(&fl, "float", ""),
		Func("func", "", func(string) error { return nil }),
		Int(&in, "int", ""),
		Int64(&i64, "int64", ""),
		String(&s, "string", ""),
		Strings(&ss, "strings", ""),
		Uint(&u, "uint", ""),
		Uint64(&u64, "uint64", ""),
		Var(&ks, "kind-var", "", kindStringType{}),
		IPVar(&ip, "opaque-var", ""),
		Unbound("stop", "").Interrupt(func(ctx context.Context, inv *Invocation) error { return nil }),
	)
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, want := range []ir.Kind{
		ir.KindBool, ir.KindBool, ir.KindDuration, ir.KindFloat, ir.KindOpaque,
		ir.KindInt, ir.KindInt, ir.KindString, ir.KindString, ir.KindUint,
		ir.KindUint, ir.KindString, ir.KindOpaque, "",
	} {
		if got := node.FlagGroups[0].Flags[i].Kind; got != want {
			t.Errorf("Flags[%d].Kind = %q, want %q", i, got, want)
		}
	}
}

// TestPositionalIsShownByItsValueName asserts that a positional argument,
// having no spelling of its own, is shown by its value name wherever a
// flag is named -- and that the name is displayed as a synopsis shows
// one, upper-cased with dashes as underscores.
func TestPositionalIsShownByItsValueName(t *testing.T) {
	for _, tt := range []struct {
		name string
		flag *FlagBuilder[string]
		want string
	}{
		{"FromFlagName", String(new(string), "src", "usage").Positional(), "SRC"},
		{"Overridden", String(new(string), "src", "usage").Positional().ValueName("path"), "PATH"},
		{"DashesBecomeUnderscores", String(new(string), "log-level", "usage").Positional(), "LOG_LEVEL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewCommand("test", "").Flags(tt.flag.Required())
			node, err := cmd.Compile()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := node.FlagGroups[0].Flags[0].String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			var sb strings.Builder
			if err := node.Usage(&sb); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if want := "Usage: test " + tt.want + "\n"; !strings.HasPrefix(sb.String(), want) {
				t.Errorf("usage = %q, want prefix %q", sb.String(), want)
			}
			_, err = Parse(cmd)
			if want := "missing required argument: " + tt.want; err == nil ||
				!strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want one containing %q", err, want)
			}
		})
	}
}

// TestCanonicalNameCoalesces asserts that the option a flag is known by
// is the first slot it declares that is not empty, so a flag declaring
// only a short name still reports itself by one.
func TestCanonicalNameCoalesces(t *testing.T) {
	var s string
	flg := String(&s, "", "usage").Aliases("v")
	cmd := NewCommand("test", "").Flags(flg)

	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	df := node.FlagGroups[0].Flags[0]
	if got, want := df.String(), "-v"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	// The empty slot survives compiling, so a formatter can still tell the
	// short name from an alias by position.
	if got, want := strings.Join(df.NamedOptions, ","), ",-v"; got != want {
		t.Errorf("NamedOptions = %q, want %q", got, want)
	}
}

// TestAliasIsMatchedButNotPrinted asserts the bargain the third slot
// makes: an alias resolves on the command line and stays out of help,
// which is what a compatibility spelling wants.
func TestAliasIsMatchedButNotPrinted(t *testing.T) {
	var s string
	cmd := NewCommand("test", "").Flags(
		String(&s, "colour", "which colour").Aliases("", "color"),
	)
	if _, err := Parse(cmd, "--color", "red"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := s, "red"; got != want {
		t.Errorf("value = %q, want %q", got, want)
	}
	node, err := cmd.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sb strings.Builder
	if err := ir.Usage(&sb, node); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if help := sb.String(); !strings.Contains(help, "--colour") ||
		strings.Contains(help, "--color ") {
		t.Errorf("help should name --colour and not the alias:\n%s", help)
	}
}

// TestAliasPositionIsNotEnforced asserts that where a name is declared
// decides nothing but where help prints it: how a name is spelled comes
// from its shape, so a name of more than one character given as the first
// alias is spelled with two dashes rather than rejected.
func TestAliasPositionIsNotEnforced(t *testing.T) {
	var s string
	node, err := NewCommand("test", "").Flags(
		String(&s, "foo", "").Aliases("xx"),
	).Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	options := node.FlagGroups[0].Flags[0].NamedOptions
	if got, want := strings.Join(options, ","), "--foo,--xx"; got != want {
		t.Errorf("NamedOptions = %q, want %q", got, want)
	}
}
