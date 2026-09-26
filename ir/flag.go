package ir

import (
	"maps"
	"slices"
	"sync/atomic"

	"go.hotsrc.dev/climux/desc"
)

// Claim is what naming a flag by one option on the command line means:
// Source is the declared option the parser resolves it to -- itself, if
// the option was declared outright, or the option a convention generated
// it from -- and Effect is what the convention says naming it this way
// does to the value, such as "negate" for the boolean opposite
// getopt_long-style conventions write. Absent, the effect is the
// ordinary one: naming the flag this way sets it.
//
// Effect is a word the convention that wrote it defines and a reader
// consumes; nothing in this package or the root package may compare it
// against a value either knows. The parser derives what it needs to know
// about a claim, such as negation, by running its own generating rule
// forward against Source instead; see internal/argv's
// resolvedOptionsInto.
type Claim struct {
	Source string
	Effect string
}

// An Origin identifies the declaration a compiled flag was lowered from.
// It is minted where a flag is declared and copied onto every compiled
// flag lowered from it, so every flag any number of compiles of one
// declaration produce shares one Origin, and no two declarations ever
// share one. The zero Origin belongs to no declaration, and is what a
// flag assembled by hand rather than lowered from one carries.
//
// This package mints none and reads none. It carries the identity so
// that a program holding a declaration can find what that declaration
// compiled to in a tree it did not build; see Invocation.Resolve.
// Nothing here can name the declaration itself, since the configuration
// types that declare flags live in the package that imports this one.
type Origin uint64

// lastOrigin is the last identity NewOrigin handed out. It counts
// declarations rather than describing any one tree, which is why a
// package modeling what a program means keeps a counter at all.
var lastOrigin atomic.Uint64

// NewOrigin returns an Origin distinct from every other, to stamp on a
// flag declaration. climux's flag constructors call it; nothing reading a
// compiled tree needs to.
func NewOrigin() Origin { return Origin(lastOrigin.Add(1)) }

// Flag is the compiled, implementation form of a command line flag or
// positional argument, produced by lowering a configuration tree with
// (*climux.Command).Compile.
//
// Every field is exported, including Value, ValidateFunc, CompleteFunc
// and Handler, which carry behavior: ir is never encoded, so nothing has
// to be hidden from an encoder to keep it out of a document. See the
// package doc for the three-type model this is the middle of.
type Flag struct {
	// NamedOptions is the option each name the program declared is shown
	// as: "--verbose" and "-v" rather than "verbose" and "v". It runs
	// parallel to those names, in the order
	// climux.Flag.Aliases documents -- the canonical name, the short
	// name, then any further aliases -- so a slot a flag left empty stays
	// empty here rather than closing up, and a help formatter can print
	// the first two knowing what they are.
	//
	// A positional argument has none. It is an operand rather than an
	// option, so nothing names it on the command line and ValueName is
	// how it is shown.
	//
	// Not every option a parser accepts appears here, so this is what is
	// worth showing rather than an exhaustive account of what matches;
	// see ClaimedOptions.
	NamedOptions []string

	// ClaimedOptions is every option the flag answers to on the command
	// line, each mapped to a Claim naming the option it came from and
	// what naming the flag that way means. Where NamedOptions is what a
	// reader is shown, this is what a reader may type, and the two differ
	// once an option is generated: a boolean answers to --no-verbose,
	// claimed with Source "--verbose", without that belonging beside the
	// flag's synonyms in help.
	//
	// It is the enumerable half of what matches, not the whole of it. A
	// convention may also match by rule -- every casing of an option, or
	// every unambiguous prefix of one -- and no map can hold those, so
	// completion offers from this while the command line still accepts
	// more. Every entry of NamedOptions appears here; Validate checks it,
	// since a convention showing an option it will not accept is a bug in
	// the convention rather than in the program that declared the flag.
	//
	// A positional argument claims none, answering to no option at all.
	ClaimedOptions map[string]Claim

	// Name is the flag's declared canonical name, undecorated by any
	// dialect: "force" rather than "--force". A positional argument's
	// Name is the name it was declared with, same as any other flag.
	//
	// It identifies the flag independent of how any one option is
	// spelled or ordered, which NamedOptions and ClaimedOptions are not:
	// both are keyed, or ordered, by a dialect's decoration.
	Name string

	// ValueName is how the value the flag takes is written where the flag
	// is shown to a reader: the "SERVICE" of "Usage: deploy SERVICE" and
	// of "missing required argument: SERVICE", and the placeholder beside
	// an option that takes one. Where NamedOptions says what a reader
	// types to name the flag, this says what they type after it -- so a
	// positional argument, which names no option and is only its value,
	// is shown by this alone.
	//
	// Like NamedOptions it arrives already written, so a formatter prints it as
	// given. Both the name it is taken from and the convention it is
	// written by belong to whatever reads the command line, which is what
	// lets one convention print SERVICE where another prints <service>. A
	// flag that takes no value has none.
	ValueName string

	// Kind classifies the value the flag takes; see Kind. It is empty for
	// a flag bound to no value, which has none to classify.
	Kind Kind

	Usage       string
	Default     string
	ShowDefault bool
	Positional  bool

	// EndOfOptions reports that every argument after this one is treated
	// as an argument rather than a flag, even if it begins with a dash, as
	// if the user had typed "--" after it.
	EndOfOptions bool

	Hidden bool

	// Persistent reports that the flag stays valid beneath the command
	// that declares it. Any other flag is valid only until the command
	// line dispatches to a subcommand; see Command.ScopedFlags.
	Persistent bool

	MinCount int
	MaxCount int
	EnvVar   string
	Choices  []string

	// TakesValue reports whether giving the flag on the command line
	// consumes a value. A boolean flag reports false: naming it alone
	// stands for true, though it still accepts a value attached with "=" to
	// set it false explicitly. Every other flag, including every
	// positional argument, reports true.
	TakesValue bool

	// Value is the flag's bound value: Set writes to it once for each
	// argument the flag is given on the command line, after ValidateFunc,
	// if any, approves the argument.
	Value Value

	// ValidateFunc, if set, validates an argument before Set writes it to
	// Value.
	ValidateFunc ValidateFunc

	// CompleteFunc, if set, completes the flag's value for a shell. See
	// climux.Complete.
	CompleteFunc CompleteFunc

	// Origin identifies the declaration this flag was lowered from, and
	// is zero for a flag built by hand rather than lowered from one. It is
	// written once, while lowering, and never afterwards: it says where
	// the flag came from, not what has been done to it. See Origin.
	Origin Origin

	// Reset forgets the previous reading of the flag, so that the next
	// time the line names it is again the first, and writes nothing.
	// SetDefault writes the flag's default into its variable if the
	// reading never named it. The parser calls Reset for every flag in
	// the tree before it applies argv and SetDefault for every flag
	// afterwards, so a variable is written once per reading, or not at
	// all. Whether the flag was named is the flag's own to know, since
	// one declaration may compile to several nodes that share a
	// variable. Either is nil for a flag with nothing to do.
	Reset      func()
	SetDefault func()

	// Handler, if set, makes the flag an interrupt: naming it on the
	// command line runs this in place of the handler of the command it was
	// given on, which is the command the resulting Invocation names.
	//
	// The rest of the line is read as usual, and every rule of it still
	// holds but one: a required argument left out is not an error. That
	// is what lets "app --help" answer someone who does not yet know what
	// the command requires. No middleware wraps it.
	//
	// An interrupt binds its value like any other flag. One with no Value
	// -- see climux.Unbound -- takes no argument on the command line, and
	// has no default to restore and no opposite to spell.
	Handler HandlerFunc
}

// String returns how the flag is shown wherever one string stands for it,
// which is how the usage line, the help message and every error naming a
// flag all refer to it: the first option it is shown by, or, for a
// positional argument that is shown by no option, its value name. A flag
// that declared no name at all has neither, and is shown as "unknown" so
// that the error saying so still reads as a sentence.
//
// It writes nothing for itself. How an option is written is settled when
// the tree is compiled, so that everything showing a flag shows the same
// thing.
func (f *Flag) String() string {
	for _, option := range f.NamedOptions {
		if option != "" {
			return option
		}
	}
	if f.ValueName != "" {
		return f.ValueName
	}
	return "unknown"
}

// Claims reports whether naming the option name on the command line
// reaches this flag. It answers only for the options that can be listed:
// a convention matching by rule, such as by prefix, accepts more than
// this reports. See ClaimedOptions.
func (f *Flag) Claims(name string) bool {
	_, ok := f.ClaimedOptions[name]
	return ok
}

// Set validates s with the flag's ValidateFunc, if it has one, and then
// sets its bound Value to s.
func (f *Flag) Set(s string) error {
	if f.ValidateFunc != nil {
		if err := f.ValidateFunc(s); err != nil {
			return err
		}
	}
	return f.Value.Set(s)
}

// Describe returns f's description: its declared name, the kind of value
// it takes, and every option that reaches it. Behavior -- Value,
// ValidateFunc, CompleteFunc and Handler -- carries nothing to describe
// and is absent from the result.
func (f *Flag) Describe() *desc.Flag {
	return &desc.Flag{
		Name:         f.Name,
		ValueName:    f.ValueName,
		Kind:         string(f.Kind),
		Usage:        f.Usage,
		Default:      f.Default,
		ShowDefault:  f.ShowDefault,
		Positional:   f.Positional,
		EndOfOptions: f.EndOfOptions,
		Hidden:       f.Hidden,
		Persistent:   f.Persistent,
		MinCount:     f.MinCount,
		MaxCount:     f.MaxCount,
		EnvVar:       f.EnvVar,
		Choices:      slices.Clone(f.Choices),
		TakesValue:   f.TakesValue,
		Options:      f.describeOptions(),
	}
}

// describeOptions returns every option that reaches f, in the order
// desc.Flag.Options documents: NamedOptions in order, skipping any empty
// slot, then every option a dialect generated, ordered by the named
// option it was generated from, so a derivative sits by the rank of its
// source rather than by the alphabet. A positional argument claims no
// option and so describes none.
func (f *Flag) describeOptions() []desc.Option {
	named := make(map[string]struct{}, len(f.NamedOptions))
	var options []desc.Option
	for _, name := range f.NamedOptions {
		if name == "" {
			continue // an empty slot, which climux.Flag.Aliases documents
		}
		named[name] = struct{}{}
		options = append(options, desc.Option{
			Option: name,
			Effect: f.ClaimedOptions[name].Effect,
		})
	}
	generated := slices.Sorted(maps.Keys(f.ClaimedOptions))
	for _, source := range f.NamedOptions {
		for _, option := range generated {
			if _, ok := named[option]; ok {
				continue
			}
			if f.ClaimedOptions[option].Source != source {
				continue
			}
			options = append(options, desc.Option{
				Option: option,
				Effect: f.ClaimedOptions[option].Effect,
			})
		}
	}
	return options
}

// CompDirective tells a shell how to treat the candidates a CompleteFunc
// returns.
type CompDirective int

const (
	// CompDefault lets the shell fall back to its own filename completion
	// when the candidates given do not satisfy it, such as when there are
	// none.
	CompDefault CompDirective = iota

	// CompNoFileComp tells the shell not to fall back to filename
	// completion: the candidates given, even if there are none, are the
	// whole answer.
	CompNoFileComp
)

// CompleteFunc completes a flag's value for a shell. inv is the invocation
// parsed so far, and word is the fragment under the cursor, which may be
// empty.
//
// inv is given because what completes a value often depends on flags
// already given -- git checkout completing a ref depends on which
// repository -r named, for instance -- and not on the word alone. Flags
// named earlier on the command line are set on inv's command by the time
// CompleteFunc is called; flags named later are not, since completion has
// not read that far.
type CompleteFunc func(inv *Invocation, word string) ([]string, CompDirective)

// FlagGroup is the compiled, implementation form of a nominal grouping of
// flags, which affects how the flags are shown in help messages. Unlike
// Command and Flag, FlagGroup carries no behavior of its own, so every
// field is exported.
type FlagGroup struct {
	Name string

	// Title is the heading a formatter prints above the group's flags.
	Title string

	// Mounted reports that the command took this group from a shared
	// Registry rather than declaring it. The flags belong to the library
	// that registered them, so the same group may appear on more than one
	// command in a tree without those commands conflicting.
	Mounted bool

	Flags []*Flag
}

// Describe returns g's description, and every flag in it.
func (g *FlagGroup) Describe() *desc.FlagGroup {
	group := &desc.FlagGroup{
		Name:  g.Name,
		Title: g.Title,
	}
	for _, flag := range g.Flags {
		group.Flags = append(group.Flags, flag.Describe())
	}
	return group
}
