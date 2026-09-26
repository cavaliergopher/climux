package ir

// Value is the interface to the dynamic value stored in a flag.
// (The default value is represented as a string.)
//
// Set is called once, in command line order, for each flag present. In
// POSIX terms, s is the option-argument written after an option, or the
// operand bound to a positional flag.
//
// This is Go's flag.Value without String, so a value already written for
// the flag package satisfies it, which is how climux.FromFlagSet mounts
// one unchanged. String is not required because a compiled flag carries
// its default already rendered, captured when the flag was constructed.
// The likeness stops at the interface: command lines are POSIX/GNU and
// not the flag package's dialect, so a program migrating keeps its values
// and changes the arguments its users type.
//
// A program declaring its own flags never implements this; it writes a
// climux.Decoder, and the root package adapts one to it.
type Value interface {
	Set(s string) error
}

// BoolValue is an optional interface to indicate boolean flags that can be
// supplied without a "=value" argument.
//
// It names the flag package's IsBoolFlag convention. A value reporting
// true is an option that takes no option-argument, so naming it alone
// stands for true and a following argument is left as an operand. It
// still accepts a value attached with "=", which is how "--verbose=false"
// sets false; that much follows Go rather than getopt_long.
type BoolValue interface {
	Value
	IsBoolFlag() bool
}

// Kind classifies the value a flag or positional argument takes, coarser
// than the Go type behind it: a command line accepts a decimal integer
// whether the variable it is bound to is an int or an int64, and Kind
// names what a program means by the flag rather than how it is built. It
// is written as its own string, so a kind added later is a new constant
// rather than a change to how one already written is read.
type Kind string

const (
	KindBool     Kind = "bool"
	KindString   Kind = "string"
	KindInt      Kind = "int"
	KindUint     Kind = "uint"
	KindFloat    Kind = "float"
	KindDuration Kind = "duration"

	// KindOpaque is every value with no narrower kind to report: one
	// whose decoder, or imported Value, does not say.
	KindOpaque Kind = "opaque"
)

// KindValue is an optional interface for a Value that reports what Kind
// it is, in the manner BoolValue reports IsBoolFlag.
//
// A value compiles to KindOpaque unless it implements this, which lets
// one describe what it accepts as precisely as a typed constructor, such
// as String or Int, already does. A climux.Decoder says the same thing
// with a Kind method of its own.
type KindValue interface {
	Value
	Kind() Kind
}

// ValidateFunc is a function that validates an argument before it is
// parsed. arg is the option-argument or operand about to be handed to
// Value.Set.
type ValidateFunc = func(arg string) error
