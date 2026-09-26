package climux

import (
	"context"
	"flag"
	"fmt"
	"slices"
	"strings"
	"time"

	"go.hotsrc.dev/climux/internal/argv"
	"go.hotsrc.dev/climux/ir"
)

const (
	defaultMinNArgs = 0
	defaultMaxNArgs = 1
)

// TODO: mutually exclusive flags?
// TODO: error handling modes

// Flag is a command line flag a command mounts, whatever type it binds.
// Every FlagBuilder is one, so Command.Flags takes a String beside a
// Bool, and a FlagGroup holds them together. Nothing outside this package
// implements it.
type Flag interface {
	lower(errs *[]error) *ir.Flag
}

// FlagBuilder declares a command line flag binding a T, and is what every
// constructor returns. Each chained method returns the builder again, so
// a declaration reads as one expression:
//
//	String("output", "Output format").Default("json").Env("APP_OUTPUT")
//
// The same type also declares a positional operand once marked with
// Positional: every constructor and every chained method applies to
// both, and it is simply named for the more common case.
//
// A flag holds its own value. State returns the half of the flag a
// handler reads -- the value, whether the flag was given, and where its
// value came from -- and Bind is how a program keeps the value in a
// variable of its own instead.
//
// Programs should not create a FlagBuilder directly and instead use one of
// the typed constructors such as String, Int or Var to construct one.
type FlagBuilder[T any] struct {
	flagConfig

	// t describes the type the flag binds: nil for one binding no value.
	t VarType[T]

	// writeDefault is how a default reaches the variable: the plain
	// write for every flag but BitField, whose default also sets a bit in
	// a shared word.
	writeDefault func(value T)

	// state is the runtime half, built here so that State can hand back
	// the same object however often it is called.
	state *FlagState[T]
}

// FlagState is the runtime half of a flag: what one reading of the
// command line made of it, read back typed. State returns it, and it is
// what a handler holds:
//
//	var output = climux.String("output", "Output format").Default("json").State()
//
//	func run(ctx context.Context, inv *climux.Invocation) error {
//		if output.IsSet() {
//			fmt.Fprintln(inv.Stdout, "you chose", output.Value())
//		}
//		return nil
//	}
//
// It carries no configuration vocabulary and offers no route back to the
// declaration. It is a Flag, so a program that keeps the state alone can
// still mount it with Command.Flags.
type FlagState[T any] struct {
	owner *FlagBuilder[T]

	// p points to the variable the value lives in: the flag's own unless
	// Bind pointed it at the program's.
	p *T

	// read returns what the flag holds. A flag with a variable reads it;
	// one binding nothing reports whether it was named.
	read func() T

	// shared is the state every node lowered from the declaration points
	// at, so what the parser wrote is what this reports.
	shared *ir.FlagState
}

// Value returns the flag's value: what the command line or the
// environment set, or else the default, or else the zero value. For a
// flag binding no value, such as Unbound, it reports whether the flag
// was given.
func (s *FlagState[T]) Value() T { return s.read() }

// IsSet reports whether the command line or the environment set the
// flag, rather than leaving it its default. It is Source() != SourceDefault.
func (s *FlagState[T]) IsSet() bool { return s.shared.Source != ir.SourceDefault }

// Source reports where the flag's value came from: SourceArgs if the
// command line set it, SourceEnv if the flag's environment variable
// did, and SourceDefault if neither did.
func (s *FlagState[T]) Source() Source { return s.shared.Source }

// Count reports how many times the command line named the flag, which
// is what a repeated flag such as -vvv counts, and how many values a
// last-wins flag discarded. A flag set from the environment counts one.
func (s *FlagState[T]) Count() int { return s.shared.Count }

// lower lets a hoisted FlagState be mounted directly, so a program that
// keeps the runtime half never has to keep the builder too.
func (s *FlagState[T]) lower(errs *[]error) *ir.Flag { return s.owner.lower(errs) }

// get is the untyped read a walker holding only the compiled tree uses.
func (s *FlagState[T]) get() any { return s.read() }

// State returns the flag's runtime half, and is both the terminal call
// of a declaration and the accessor a handler reads. It returns the same
// object every time, which is what lets a program hoist the call into
// its declaration or leave it out entirely.
func (c *FlagBuilder[T]) State() *FlagState[T] { return c.state }

// Bind keeps the flag's value in the variable p points to, instead of in
// the flag. There is one variable either way: the flag reads what p
// holds, so Value and p never disagree unless the program writes p
// itself. Nothing is written to p before the command line is parsed.
func (c *FlagBuilder[T]) Bind(p *T) *FlagBuilder[T] {
	c.state.p = p
	return c
}

// Default sets the flag's default value, which parsing writes when the
// command line and the environment leave the flag alone, and which help
// shows for it. Without it nothing is written for an unnamed flag, and
// Value reports the zero value, or whatever the program left in a bound
// variable. Declaring a flag writes nothing either way; the one write,
// of the default or of what the line says, happens when the line is
// parsed.
func (c *FlagBuilder[T]) Default(value T) *FlagBuilder[T] {
	c.defValue = fmt.Sprint(value)
	if c.t != nil {
		c.defValue = c.t.Format(value)
	}
	c.shared.SetDefault = func() { c.writeDefault(value) }
	return c
}

// flagConfig is what every flag declares whatever it binds: the half of
// a FlagBuilder that has no T, and the whole of a flag imported from a
// flag.FlagSet, which has none to offer.
type flagConfig struct {
	// names are every name the flag answers to, in the slot order Names
	// documents. A slot may be empty, which is how a flag declares an
	// alias without a short name, so the slice is read by index rather
	// than compacted.
	names    []string
	usage    string
	defValue string

	// valueName overrides the name shown for the flag's value; empty
	// leaves the command line conventions to name it. See Flag.ValueName.
	valueName string

	showDefault  bool
	positional   bool
	endOfOptions bool
	minCount     int
	maxCount     int
	hidden       bool
	persistent   bool
	envVar       string
	choices      []string
	validateFunc ir.ValidateFunc
	completeFunc ir.CompleteFunc
	value        ir.Value

	// shared is the runtime state every node lowered from this
	// declaration points at, and the parser writes. See ir.FlagState.
	shared *ir.FlagState

	// kind classifies the value being bound, set by whichever typed
	// constructor built this flag, or recovered from a flag.Getter for
	// one imported with FromFlagSet. See ir.Kind.
	kind ir.Kind

	// handlerFunc is what the flag runs in place of the command's handler
	// when the line names it, which is what makes it an interrupt. See
	// FlagBuilder.Interrupt.
	handlerFunc HandlerFunc
}

// Var returns a flag binding a value of a type t describes: how one is
// decoded, shown and classified. The flag holds the value; read it with
// State, or keep it in a variable of the program's own with Bind.
//
// name becomes the flag's canonical name: one character is spelled with a
// single dash, so Var("n", usage, t) declares "-n", and anything longer
// takes two. Add further names with FlagBuilder.Aliases.
func Var[T any](name, usage string, t VarType[T]) *FlagBuilder[T] {
	c := newFlag[T](name, usage)
	c.t = t
	c.value = value[T]{s: c.state}
	c.kind = t.Kind()
	c.writeDefault = func(value T) { *c.state.p = value }
	return c
}

// newFlag returns the builder every constructor starts from, with a
// variable of its own and its runtime state allocated, binding no value
// yet.
func newFlag[T any](name, usage string) *FlagBuilder[T] {
	shared := &ir.FlagState{}
	c := &FlagBuilder[T]{
		flagConfig: flagConfig{
			names:    []string{name},
			usage:    usage,
			minCount: defaultMinNArgs,
			maxCount: defaultMaxNArgs,
			shared:   shared,
		},
		writeDefault: func(T) {},
	}
	c.state = &FlagState[T]{owner: c, p: new(T), shared: shared}
	c.state.read = func() T { return *c.state.p }
	shared.Get = c.state.get
	return c
}

// BitField returns a bool flag which sets the bits of mask in the uint64
// variable p points to when it is true. Several BitFields may share one
// variable, each setting its own bits, and a Default of true sets them
// too. A false leaves the variable as it is. The flag's own value is its
// bit, and Bind keeps that bit in a bool of the program's own.
func BitField(p *uint64, mask uint64, name, usage string) *FlagBuilder[bool] {
	c := Var(name, usage, bitFieldType{word: p, mask: mask})
	c.writeDefault = func(value bool) {
		*c.state.p = value
		if value {
			*p |= mask
		}
	}
	return c
}

// Bool returns a bool flag with the specified name and usage string. It
// stands alone on the command line, and "--name=false" sets it false.
func Bool(name, usage string) *FlagBuilder[bool] {
	return Var(name, usage, boolType{})
}

// Duration returns a time.Duration flag with the specified name and usage
// string. The flag accepts a value acceptable to time.ParseDuration.
func Duration(name, usage string) *FlagBuilder[time.Duration] {
	return Var(name, usage, durationType{})
}

// Float64 returns a float64 flag with the specified name and usage
// string.
func Float64(name, usage string) *FlagBuilder[float64] {
	return Var(name, usage, float64Type{})
}

// Func returns a flag that calls fn with its value each time it is given
// on the command line. An error from fn is reported as a bad flag value.
//
// The flag may be given any number of times; constrain it with
// FlagBuilder.NArgs. Its Kind is ir.KindOpaque: fn may parse its argument
// as anything, so the flag is not described as text the way String is.
func Func(name, usage string, fn func(s string) error) *FlagBuilder[bool] {
	return Var(name, usage, funcType(fn)).NArgs(0, 0)
}

// Int returns an int flag with the specified name and usage string.
func Int(name, usage string) *FlagBuilder[int] {
	return Var(name, usage, intType{})
}

// Int64 returns an int64 flag with the specified name and usage string.
func Int64(name, usage string) *FlagBuilder[int64] {
	return Var(name, usage, int64Type{})
}

// String returns a string flag with the specified name and usage string.
func String(name, usage string) *FlagBuilder[string] {
	return Var(name, usage, stringType{})
}

// Strings returns a string slice flag with the specified name and usage
// string. It may be given any number of times, and each value is
// appended in command line order.
func Strings(name, usage string) *FlagBuilder[[]string] {
	return Var(name, usage, stringsType{}).NArgs(0, 0)
}

// Uint returns a uint flag with the specified name and usage string.
func Uint(name, usage string) *FlagBuilder[uint] {
	return Var(name, usage, uintType{})
}

// Uint64 returns a uint64 flag with the specified name and usage string.
func Uint64(name, usage string) *FlagBuilder[uint64] {
	return Var(name, usage, uint64Type{})
}

// Unbound returns a flag that binds no value. It is given by name alone:
// it takes no argument and has no negated spelling. What it does when
// given is whatever is chained onto it:
//
//	Unbound("end-of-options", usage).EndOfOptions()
//	Unbound("version", usage).Interrupt(printVersion)
//
// With nothing chained, a handler asks whether it was given through
// State: its Value and IsSet both report that. It cannot be a positional
// argument, reads no environment variable, and has no Default worth
// setting.
func Unbound(name, usage string) *FlagBuilder[bool] {
	c := newFlag[bool](name, usage)
	// It binds nothing, so what it holds is whether it was named.
	c.state.read = func() bool { return c.shared.Count > 0 }
	return c
}

// HelpFlag returns the interrupt that prints a command's help message.
// Given no names it answers to "--help" and "-h"; given some, it answers
// to those, so a program wanting "-h" for something of its own keeps the
// long name alone:
//
//	NewCommand("ssh", "").Flags(HelpFlag("help"))
//
// It is persistent, so mounted on the root it answers under every
// command, and prints the help of whichever command it was written
// after. Mount it like any other flag. Command.HelpFlag is the shorthand.
func HelpFlag(names ...string) *FlagBuilder[bool] {
	if len(names) == 0 {
		names = []string{"help", "h"}
	}
	return Unbound(canonicalName(names), "Show this help message and exit").
		Aliases(names[1:]...).
		Interrupt(printHelp).
		Persistent()
}

// VersionFlag returns the interrupt that prints version, alongside the
// name of the program it is mounted in. Given no names it answers to
// "--version"; given some, it answers to those.
//
// Mount it like any other flag. Command.VersionFlag is the shorthand, and
// this is the way to put it somewhere that shorthand cannot -- a flag
// group of its own, or hidden.
//
// Like HelpFlag, it ends the program before any handlers run. See
// VersionCommand for the same thing spelled as a subcommand.
func VersionFlag(version string, names ...string) *FlagBuilder[bool] {
	if len(names) == 0 {
		names = []string{"version"}
	}
	return Unbound(canonicalName(names), "Show the version and exit").
		Aliases(names[1:]...).
		Interrupt(printVersion(version))
}

// printHelp is the handler of the flag asking for help: the command the
// command line named describes itself.
func printHelp(ctx context.Context, inv *Invocation) error {
	return inv.Cmd.Usage(inv.Stdout)
}

// printVersion returns the handler that prints version, which VersionFlag
// and VersionCommand both run.
//
// The program's name comes from the root of the tree rather than from
// the command that was named, so a version flag made persistent reports
// orbital's version from "orbital deploy --version" too. The
// program supplies only the version itself, which is what a build stamps
// into a constant.
func printVersion(version string) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		_, err := fmt.Fprintf(inv.Stdout, "%s %s\n", inv.Cmd.Root.Name, version)
		return err
	}
}

// ShowDefault specifies that the default value of this flag should be shown
// in the help message.
func (c *FlagBuilder[T]) ShowDefault() *FlagBuilder[T] {
	c.showDefault = true
	return c
}

// Aliases specifies further names the flag answers to, after the one its
// constructor gave. Each is matched on the command line and sets the same
// value, so the flag below answers to "--verbose", "-v" and "--loud"
// alike:
//
//	Bool("verbose", usage).Aliases("v", "loud")
//
// The first alias is the short name, and help prints it beside the
// constructor's name. Anything after it is matched but left out of help,
// which is what a compatibility spelling wants. A flag needing one of
// those but no short name leaves the first alias empty:
//
//	String("colour", usage).Aliases("", "color")
//
// A short name is one character from [A-Za-z0-9].
func (c *FlagBuilder[T]) Aliases(names ...string) *FlagBuilder[T] {
	c.names = append(c.names, names...)
	return c
}

// ValueName names the value the flag takes, which stands in for it
// wherever the flag is shown: the usage line, the help message, and any
// error naming it. A positional argument is shown by this alone.
//
// Without it the flag's own name is used, so this is needed only where
// that name reads poorly for the value. An option whose only name is a
// single character is shown as VALUE instead, since the letter says
// nothing about what it takes:
//
//	Strings("tags", usage).Positional().ValueName("tag")
//
// Give the name undecorated: "tag" is shown as TAG. A flag that takes no
// value, such as a boolean, ignores this.
func (c *FlagBuilder[T]) ValueName(name string) *FlagBuilder[T] {
	c.valueName = name
	return c
}

// Positional indicates that this flag is a positional argument, and therefore
// has no "-" or "--" delimiter.
//
// A command may declare positional arguments and subcommands together. Its
// first operand decides between them: a word naming a subcommand runs it,
// and any other word binds the first positional argument, after which every
// word binds a positional argument until they are all full.
func (c *FlagBuilder[T]) Positional() *FlagBuilder[T] {
	c.positional = true
	return c
}

// EndOfOptions treats every argument after this one as an argument
// rather than a flag, even if it begins with a dash, as if the user had
// typed "--" after it.
//
// Use it for a command that passes the rest of its command line to
// another program, so that program's flags reach it instead of being
// read as this command's own:
//
//	String("IMAGE", usage).Positional().EndOfOptions()
//	Strings("ARG", usage).Positional()
//
//	docker run -it alpine ls -la   ->  -it is run's; IMAGE=alpine; ARG=["ls", "-la"]
//
// An option may use it too, to give the user a second spelling of "--":
//
//	Unbound("end-of-options", usage).EndOfOptions()
//
//	git log --end-of-options --weird-branch   ->  REV=["--weird-branch"]
func (c *FlagBuilder[T]) EndOfOptions() *FlagBuilder[T] {
	c.endOfOptions = true
	return c
}

// Interrupt makes the flag an interrupt: naming it runs fn in place of
// the handler of the command it was given on, without that command's
// middleware, and excuses any argument the line was required to give. The
// flag still binds its value, so fn can read it.
//
//	String("help", "Show help for a topic").Interrupt(showHelp)
//
//	app --help deploy   ->  showHelp runs, with topic "deploy"
//
// Unbound builds the common case, a flag that takes no value at all. A
// positional argument may not interrupt.
func (c *FlagBuilder[T]) Interrupt(fn HandlerFunc) *FlagBuilder[T] {
	c.handlerFunc = fn
	return c
}

// NArgs sets how many times this flag may be given on the command line.
//
// A count of 0 removes the bound, and so means something different at each
// end: a min of 0 is no floor, making the flag optional, while a max of 0 is
// no ceiling, letting it repeat without limit.
//
//	NArgs(0, 1)  optional, at most once -- the default
//	NArgs(1, 1)  exactly once; see Required
//	NArgs(0, 0)  optional, unbounded -- what Strings and Func set
//	NArgs(1, 0)  required, unbounded
func (c *FlagBuilder[T]) NArgs(min, max int) *FlagBuilder[T] {
	c.minCount = min
	c.maxCount = max
	return c
}

// Required is shorthand for NArgs(1, 1) and indicates that this flag must be
// specified on the command line once and only once.
func (c *FlagBuilder[T]) Required() *FlagBuilder[T] {
	return c.NArgs(1, 1)
}

// Hidden hides the command line flag from all help messages but still allows
// the flag to be specified on the command line.
func (c *FlagBuilder[T]) Hidden() *FlagBuilder[T] {
	c.hidden = true
	return c
}

// Persistent keeps the flag valid beneath the command that declares it:
// it may be written after any of that command's subcommands is named, and
// means the same thing there. Otherwise a flag is valid only until the
// command line names a subcommand. A positional argument cannot be
// persistent.
func (c *FlagBuilder[T]) Persistent() *FlagBuilder[T] {
	c.persistent = true
	return c
}

// Env allows the value of the flag to be specified with an environment
// variable if it is not specified on the command line.
func (c *FlagBuilder[T]) Env(name string) *FlagBuilder[T] {
	c.envVar = name
	return c
}

// Validate specifies a function to validate an argument for this flag before
// it is parsed. If the function returns an error, parsing will fail with the
// same error.
func (c *FlagBuilder[T]) Validate(f ir.ValidateFunc) *FlagBuilder[T] {
	c.validateFunc = f
	return c
}

// Choices restricts the flag's value to one of elems: any other value
// fails to parse, naming the legal choices.
func (c *FlagBuilder[T]) Choices(elems ...string) *FlagBuilder[T] {
	c.choices = elems
	return c.Validate(
		func(arg string) error {
			for _, elem := range c.choices {
				if arg == elem {
					return nil
				}
			}
			return fmt.Errorf("expected one of: %s", strings.Join(c.choices, ", "))
		},
	)
}

// Complete registers fn to complete this flag's value for a shell, whether
// the flag is an option or a positional argument. It is consulted only
// when Choices is not declared; Choices, being the enumerable case, always
// wins.
func (c *FlagBuilder[T]) Complete(fn ir.CompleteFunc) *FlagBuilder[T] {
	c.completeFunc = fn
	return c
}

// lower returns the compiled ir.Flag for c: its data fields copied
// across, its names decorated the way the command line writes them, with
// TakesValue derived from whether its Value is a BoolValue, and its
// behavior -- the Value, the ValidateFunc and the CompleteFunc -- copied
// into the fields only Compile has any business setting.
//
// The rules a name itself must keep are checked here, into errs, because
// this is the last point at which the names are still undecorated. The
// rest of flag configuration is not checked here; see ir.Flag's own
// validation, which Compile runs over the whole lowered tree.
func (c *flagConfig) lower(errs *[]error) *ir.Flag {
	// A flag bound to no value is named and never given one, so it is not
	// written as if it took one.
	takesValue := c.value != nil && (c.positional || !isBoolValue(c.value))
	// How a flag is written down is the command line's question rather
	// than this package's, in both halves of it, so what it declared goes
	// over and the answers come back whole: which options it has and
	// which it only answers to, and what its value is called, including
	// when the answer is none. See ir.Flag.
	namedOptions, claimedOptions := argv.OptionsFor(c.names, c.positional, takesValue, c.value == nil)
	valueName := argv.ValueNameFor(canonicalName(c.names), c.valueName, c.positional, takesValue)
	flag := &ir.Flag{
		State:          c.shared,
		NamedOptions:   namedOptions,
		ClaimedOptions: claimedOptions,
		Name:           canonicalName(c.names),
		ValueName:      valueName,
		Kind:           c.kind,
		Usage:          c.usage,
		Default:        c.defValue,
		ShowDefault:    c.showDefault,
		Positional:     c.positional,
		EndOfOptions:   c.endOfOptions,
		Hidden:         c.hidden,
		Persistent:     c.persistent,
		MinCount:       c.minCount,
		MaxCount:       c.maxCount,
		EnvVar:         c.envVar,
		Choices:        slices.Clone(c.choices),
		TakesValue:     takesValue,
		Value:          c.value,
		ValidateFunc:   c.validateFunc,
		CompleteFunc:   c.completeFunc,
		Handler:        c.handlerFunc,
	}
	c.validateNames(flag, errs)
	return flag
}

// validateNames checks the names c was declared with, undecorated, and
// records what it finds against the lowered flag so an error can name the
// flag the way everything else does.
func (c *flagConfig) validateNames(flag *ir.Flag, errs *[]error) {
	// A flag with nothing but empty slots can never be named, and has
	// nothing for an error to report it by.
	if canonicalName(c.names) == "" {
		*errs = append(*errs, ir.NewConfigErrorf(nil, nil, flag,
			"flag must declare a name"))
	}
	// What a name may be, and whether a flag of this shape may have more
	// than one, are both the command line's questions; see
	// argv.ValidateNames.
	for _, err := range argv.ValidateNames(c.names, c.positional) {
		*errs = append(*errs, ir.NewConfigErrorf(nil, nil, flag, "%s", err))
	}
}

// canonicalName returns the first of names that is not empty, which is
// the name a flag is known by wherever one name has to stand for it: a
// flag that declares only a short name still has something for its value
// name to be taken from.
func canonicalName(names []string) string {
	for _, name := range names {
		if name != "" {
			return name
		}
	}
	return ""
}

// FlagGroup is a nominal grouping of flags which affects how the flags are
// shown in help messages.
type FlagGroup struct {
	name  string
	title string
	flags []Flag
}

// NewFlagGroup returns a new FlagGroup with the given name that shows its
// flags under the given title in help messages.
//
// A group built standalone is how a library contributes flags bound to
// variables it owns: mount it on a command with Command.FlagGroups, or
// register it with Register so every command that mounts DefaultRegistry
// picks it up.
func NewFlagGroup(name, title string, flags ...Flag) *FlagGroup {
	return &FlagGroup{
		name:  name,
		title: title,
		flags: flags,
	}
}

// Flags appends command line flags to the group.
func (c *FlagGroup) Flags(flags ...Flag) *FlagGroup {
	c.flags = append(c.flags, flags...)
	return c
}

// FromFlagSet returns a FlagGroup holding the flags declared on fs, a flag
// set from Go's flag package, so a program can carry flags from
// stdlib-flavored libraries. Pass flag.CommandLine for the flags declared
// on the flag package itself. Mount the group with Command.FlagGroups, or
// register it with Registry.FlagGroups.
//
// The flag set is read once, here: a flag declared on fs afterwards is not
// seen. Parsing and error handling are this package's from then on.
//
// Every flag is persistent. A flag set is written for a whole program, and
// its flags are read wherever the program likes, so none belongs to the
// one command it happens to be mounted on.
//
// A flag whose Value implements flag.Getter is described as precisely as
// a native one: its Kind is recovered from the concrete type Get
// returns, and is ir.KindOpaque for a Value that does not implement
// flag.Getter or whose concrete type matches none of the flag package's
// own.
func FromFlagSet(name, title string, fs *flag.FlagSet) *FlagGroup {
	group := NewFlagGroup(name, title)
	fs.VisitAll(func(f *flag.Flag) {
		group.Flags(fromFlag(f))
	})
	return group
}

// fromFlag returns the Flag for one imported from a flag.FlagSet. It is
// the one way a flag.Value enters this package: the parser calls Set on
// it directly, since it already speaks ir.Value, and its default is the
// string the flag package rendered. It binds no T, so it is the config
// half alone.
func fromFlag(f *flag.Flag) *flagConfig {
	return &flagConfig{
		shared:     &ir.FlagState{},
		names:      []string{f.Name},
		usage:      f.Usage,
		defValue:   f.DefValue,
		minCount:   defaultMinNArgs,
		maxCount:   defaultMaxNArgs,
		persistent: true,
		value:      f.Value,
		kind:       kindFromFlagValue(f.Value),
	}
}

// kindFromFlagValue recovers the Kind of a value imported from a
// flag.FlagSet, so a flag declared with any of the flag package's own
// constructors is described as precisely as one declared with climux's.
// The concrete type flag.Getter's Get returns classifies the eight
// value-carrying constructors, flag.Bool through flag.Duration; a value
// answering IsBoolFlag, which is how flag.BoolFunc marks itself, is a
// boolean whatever it returns. Anything else -- flag.Func, flag.TextVar,
// or a custom flag.Value -- compiles to ir.KindOpaque.
func kindFromFlagValue(v flag.Value) ir.Kind {
	if g, ok := v.(flag.Getter); ok {
		switch g.Get().(type) {
		case bool:
			return ir.KindBool
		case string:
			return ir.KindString
		case int, int64:
			return ir.KindInt
		case uint, uint64:
			return ir.KindUint
		case float64:
			return ir.KindFloat
		case time.Duration:
			return ir.KindDuration
		}
	}
	if b, ok := v.(ir.BoolValue); ok && b.IsBoolFlag() {
		return ir.KindBool
	}
	return ir.KindOpaque
}

// lower returns the compiled ir.FlagGroup for c.
func (c *FlagGroup) lower(mounted bool, errs *[]error) *ir.FlagGroup {
	group := &ir.FlagGroup{
		Name:    c.name,
		Title:   c.title,
		Mounted: mounted,
	}
	for _, flag := range c.flags {
		group.Flags = append(group.Flags, flag.lower(errs))
	}
	return group
}
