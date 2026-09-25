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

// Flag describes a command line flag that may be specified on the command
// line.
//
// The same type also describes a positional operand once marked with
// Positional: every constructor and every chained modifier applies to
// both, and Flag is simply named for the more common case.
//
// Programs should not create Flag directly and instead use one of the typed
// constructors such as String, Int or Var to construct one.
type Flag struct {
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
	envVar       string
	choices      []string
	validateFunc ir.ValidateFunc
	completeFunc ir.CompleteFunc
	value        ir.Value

	// kind classifies the value being bound, set by whichever typed
	// constructor built this flag, or recovered from a flag.Getter for
	// one imported with FromFlagSet. See ir.Kind.
	kind ir.Kind

	// handlerFunc is what the flag runs in place of the command's handler
	// when the line names it, which is what makes it an interrupt. See
	// Flag.Interrupt.
	handlerFunc HandlerFunc

	// origin is this declaration's identity, minted once here and copied
	// onto every compiled flag lowered from it, which is what lets
	// SourceIn find this declaration's flag in a compiled tree without
	// going by name. See ir.Origin.
	origin ir.Origin
}

// Var returns a Flag that can be used to define a command line flag with
// custom value parsing.
//
// name becomes the flag's canonical name: one character is spelled with a
// single dash, so Var(v, "n", usage) declares "-n", and anything longer
// takes two. Add further names with Flag.Aliases.
//
// The flag's Kind is ir.KindOpaque unless value implements
// ir.KindValue, which lets a custom value describe what it accepts as
// precisely as a typed constructor such as String or Int already does.
func Var(value ir.Value, name, usage string) *Flag {
	return &Flag{
		origin:   ir.NewOrigin(),
		names:    []string{name},
		usage:    usage,
		minCount: defaultMinNArgs,
		maxCount: defaultMaxNArgs,
		value:    value,
		kind:     kindOf(value),
	}
}

// stringifyDefault returns the string form of a flag's default value, using
// its Value's String method if it implements fmt.Stringer.
func stringifyDefault(v ir.Value) string {
	if s, ok := v.(fmt.Stringer); ok {
		return s.String()
	}
	return ""
}

// BitField returns a Flag that can be used to define a uint64 flag
// with specified name, default value, and usage string. The argument p points
// to a uint64 variable in which to toggle each of the bits in the mask
// argument. You can specify multiple BitFieldVars to toggle bits in the same
// underlying uint64.
func BitField(p *uint64, mask uint64, name string, value bool, usage string) *Flag {
	v := newBitFieldValue(value, p, mask)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindBool
	return c
}

// Bool returns a Flag that can be used to define a bool flag with
// specified name, default value, and usage string. The argument p points to a
// bool variable in which to store the value of the flag.
func Bool(p *bool, name string, value bool, usage string) *Flag {
	v := newBoolValue(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindBool
	return c
}

// Duration returns a Flag that can be used to define a time.Duration
// flag with specified name, default value, and usage string. The argument p
// points to a time.Duration variable in which to store the value of the flag.
// The flag accepts a value acceptable to time.ParseDuration.
func Duration(p *time.Duration, name string, value time.Duration, usage string) *Flag {
	v := newDurationValue(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindDuration
	return c
}

// Float64 returns a Flag that can be used to define a float64 flag
// with specified name, default value, and usage string. The argument p points
// to a float64 variable in which to store the value of the flag.
func Float64(p *float64, name string, value float64, usage string) *Flag {
	v := newFloat64Value(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindFloat
	return c
}

// Func returns a Flag that calls fn with its value each time it is given on
// the command line. An error from fn is reported as a bad flag value.
//
// The flag may be given any number of times; constrain it with
// Flag.NArgs. Its Kind is ir.KindOpaque: fn may parse its argument as
// anything, so the flag is not described as text the way String is.
func Func(name, usage string, fn func(s string) error) *Flag {
	c := Var(funcValue(fn), name, usage).NArgs(0, 0)
	c.kind = ir.KindOpaque
	return c
}

// Int returns a Flag that can be used to define an int flag with
// specified name, default value, and usage string. The argument p points to an
// int variable in which to store the value of the flag.
func Int(p *int, name string, value int, usage string) *Flag {
	v := newIntValue(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindInt
	return c
}

// Int64 returns a Flag that can be used to define an int64 flag with
// specified name, default value, and usage string. The argument p points to an
// int64 variable in which to store the value of the flag.
func Int64(p *int64, name string, value int64, usage string) *Flag {
	v := newInt64Value(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindInt
	return c
}

// String returns a Flag that can be used to define a string flag with
// specified name, default value, and usage string. The argument p points to a
// string variable in which to store the value of the flag.
func String(p *string, name, value, usage string) *Flag {
	v := newStringValue(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindString
	return c
}

// Strings returns a Flag that can be used to define a string slice flag with specified name,
// default value, and usage string. The argument p points to a string slice variable in which each
// flag value will be stored in command line order.
func Strings(p *[]string, name string, value []string, usage string) *Flag {
	v := newStringSliceValue(value, p)
	c := Var(v, name, usage).NArgs(0, 0)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindString
	return c
}

// Uint returns a Flag that can be used to define an uint flag with
// specified name, default value, and usage string. The argument p points to an
// uint variable in which to store the value of the flag.
func Uint(p *uint, name string, value uint, usage string) *Flag {
	v := newUintValue(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindUint
	return c
}

// Uint64 returns a Flag that can be used to define an uint64 flag
// with specified name, default value, and usage string. The argument p points
// to an uint64 variable in which to store the value of the flag.
func Uint64(p *uint64, name string, value uint64, usage string) *Flag {
	v := newUint64Value(value, p)
	c := Var(v, name, usage)
	c.defValue = stringifyDefault(v)
	c.kind = ir.KindUint
	return c
}

// Interrupt returns a Flag that runs fn in place of the handler of
// whichever command was named, without its middleware:
//
//	Interrupt("version", "Show the version and exit", printVersion)
//
// The rest of the command line is read and checked as usual, except that
// an argument the command requires may be left out. That is what lets
// "app --help" answer someone who does not yet know what is required,
// while "app --version --format=json" still binds its format.
//
// The flag takes no value and is given by name alone; Flag.Interrupt
// makes a flag of any other kind an interrupt. See HelpFlag and
// VersionFlag for the two every program tends to want.
func Interrupt(name, usage string, fn HandlerFunc) *Flag {
	return &Flag{
		origin:      ir.NewOrigin(),
		names:       []string{name},
		usage:       usage,
		minCount:    defaultMinNArgs,
		maxCount:    defaultMaxNArgs,
		handlerFunc: fn,
	}
}

// HelpFlag returns the Interrupt that prints a command's help message.
// Given no names it answers to "--help" and "-h"; given some, it answers
// to those, so a program wanting "-h" for something of its own keeps the
// long name alone:
//
//	NewCommand("ssh", "").Flags(HelpFlag("help"))
//
// Mount it like any other flag. Command.HelpFlag is the shorthand.
func HelpFlag(names ...string) *Flag {
	if len(names) == 0 {
		names = []string{"help", "h"}
	}
	return Interrupt(canonicalName(names), "Show this help message and exit", printHelp).
		Aliases(names[1:]...)
}

// VersionFlag returns the Interrupt that prints version, alongside the
// name of the program it is mounted in. Given no names it answers to
// "--version"; given some, it answers to those.
//
// Mount it like any other flag. Command.VersionFlag is the shorthand, and
// this is the way to put it somewhere that shorthand cannot -- a flag
// group of its own, or hidden.
//
// Like HelpFlag, it ends the program before any handlers run. See
// VersionCommand for the same thing spelled as a subcommand.
func VersionFlag(version string, names ...string) *Flag {
	if len(names) == 0 {
		names = []string{"version"}
	}
	return Interrupt(canonicalName(names), "Show the version and exit", printVersion(version)).
		Aliases(names[1:]...)
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
// the command that was named, so "orbital deploy --version" reports
// orbital's version, whichever command the flag was given after. The
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
func (c *Flag) ShowDefault() *Flag {
	c.showDefault = true
	return c
}

// Aliases specifies further names the flag answers to, after the one its
// constructor gave. Each is matched on the command line and sets the same
// value, so the flag below answers to "--verbose", "-v" and "--loud"
// alike:
//
//	Bool(&v, "verbose", false, usage).Aliases("v", "loud")
//
// The first alias is the short name, and help prints it beside the
// constructor's name. Anything after it is matched but left out of help,
// which is what a compatibility spelling wants. A flag needing one of
// those but no short name leaves the first alias empty:
//
//	String(&c, "colour", "", usage).Aliases("", "color")
//
// A short name is one character from [A-Za-z0-9].
func (c *Flag) Aliases(names ...string) *Flag {
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
//	Strings(&tags, "tags", nil, usage).Positional().ValueName("tag")
//
// Give the name undecorated: "tag" is shown as TAG. A flag that takes no
// value, such as a boolean, ignores this.
func (c *Flag) ValueName(name string) *Flag {
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
func (c *Flag) Positional() *Flag {
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
//	String(&image, "IMAGE", "", usage).Positional().EndOfOptions()
//	Strings(&args, "ARG", nil, usage).Positional()
//
//	docker run -it alpine ls -la   ->  -it is run's; IMAGE=alpine; ARG=["ls", "-la"]
//
// Only a positional argument may use it.
func (c *Flag) EndOfOptions() *Flag {
	c.endOfOptions = true
	return c
}

// Interrupt makes the flag an interrupt: naming it runs fn in place of
// the handler of the command it was given on, without that command's
// middleware, and excuses any argument the line was required to give. The
// flag still binds its value, so fn can read it.
//
//	String(&topic, "help", "", "Show help for a topic").Interrupt(showHelp)
//
//	app --help deploy   ->  showHelp runs, with topic "deploy"
//
// The package-level Interrupt builds the common case, a flag that takes
// no value at all. A positional argument may not interrupt.
func (c *Flag) Interrupt(fn HandlerFunc) *Flag {
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
func (c *Flag) NArgs(min, max int) *Flag {
	c.minCount = min
	c.maxCount = max
	return c
}

// Required is shorthand for NArgs(1, 1) and indicates that this flag must be
// specified on the command line once and only once.
func (c *Flag) Required() *Flag {
	return c.NArgs(1, 1)
}

// Hidden hides the command line flag from all help messages but still allows
// the flag to be specified on the command line.
func (c *Flag) Hidden() *Flag {
	c.hidden = true
	return c
}

// Env allows the value of the flag to be specified with an environment
// variable if it is not specified on the command line.
func (c *Flag) Env(name string) *Flag {
	c.envVar = name
	return c
}

// Validate specifies a function to validate an argument for this flag before
// it is parsed. If the function returns an error, parsing will fail with the
// same error.
func (c *Flag) Validate(f ir.ValidateFunc) *Flag {
	c.validateFunc = f
	return c
}

// Choices restricts the flag's value to one of elems: any other value
// fails to parse, naming the legal choices.
func (c *Flag) Choices(elems ...string) *Flag {
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
func (c *Flag) Complete(fn ir.CompleteFunc) *Flag {
	c.completeFunc = fn
	return c
}

// SourceIn reports where the value this flag held during inv came from:
// SourceArgs if the command line set it, SourceEnv if the flag's
// environment variable did, and SourceDefault if neither did and it still
// held what it was constructed with.
//
//	if template != "" && !outputFlag.IsSetIn(inv) {
//	    output = "go-template"
//	}
//
// It finds the flag by the declaration rather than by its name, so a flag
// of the same name in another subtree can never answer for it. A flag
// that was not in scope for inv at all reports SourceDefault, which is
// the one answer worth asking InScope about.
//
// An interrupt the line named reports SourceArgs, even one that takes no
// value.
func (c *Flag) SourceIn(inv *Invocation) Source {
	flag := inv.Resolve(c.origin)
	if flag == nil {
		return SourceDefault
	}
	return inv.Sources[flag]
}

// IsSetIn reports whether the command line or the environment set this
// flag during inv, rather than leaving it the default it was constructed
// with. It is SourceIn(inv) != SourceDefault.
func (c *Flag) IsSetIn(inv *Invocation) bool {
	return c.SourceIn(inv) != SourceDefault
}

// InScope reports whether this flag was one inv could have been given:
// whether the command inv named, or an ancestor of it, declared it. It is
// what tells a flag nothing set from a flag the command line could not
// have named at all, which SourceIn answers alike.
func (c *Flag) InScope(inv *Invocation) bool {
	return inv.Resolve(c.origin) != nil
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
func (c *Flag) lower(errs *[]error) *ir.Flag {
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
		Origin:         c.origin,
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
func (c *Flag) validateNames(flag *ir.Flag, errs *[]error) {
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
	flags []*Flag
}

// NewFlagGroup returns a new FlagGroup with the given name that shows its
// flags under the given title in help messages.
//
// A group built standalone is how a library contributes flags bound to
// variables it owns: mount it on a command with Command.FlagGroups, or
// register it with Register so every command that mounts DefaultRegistry
// picks it up.
func NewFlagGroup(name, title string, flags ...*Flag) *FlagGroup {
	return &FlagGroup{
		name:  name,
		title: title,
		flags: flags,
	}
}

// Flags appends command line flags to the group.
func (c *FlagGroup) Flags(flags ...*Flag) *FlagGroup {
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
// A flag whose Value implements flag.Getter is described as precisely as
// a native one: its Kind is recovered from the concrete type Get
// returns, and is ir.KindOpaque for a Value that does not implement
// flag.Getter or whose concrete type matches none of the flag package's
// own.
func FromFlagSet(name, title string, fs *flag.FlagSet) *FlagGroup {
	group := NewFlagGroup(name, title)
	fs.VisitAll(func(f *flag.Flag) {
		flg := Var(f.Value, f.Name, f.Usage)
		flg.defValue = f.DefValue
		flg.kind = kindFromFlagValue(f.Value)
		group.Flags(flg)
	})
	return group
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
