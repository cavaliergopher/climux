package desc

// Flag describes a command line flag or positional argument: what it is
// called, what options reach it, and what kind of value it takes.
type Flag struct {
	// Name is the flag's declared canonical name, undecorated by any
	// dialect: "force" rather than "--force". A positional argument's
	// Name is the name it was declared with, same as any other flag.
	Name string `json:"name"`

	// ValueName is how the value the flag takes is written where the
	// flag is shown to a reader, such as the "SERVICE" of "Usage: deploy
	// SERVICE". A positional argument, which names no option, is shown
	// by this alone. A flag that takes no value has none.
	ValueName string `json:"valueName,omitempty"`

	// Kind names the kind of value the flag takes: "bool", "string",
	// "int", "uint", "float", "duration", or "opaque" for a custom value
	// that does not say. It is absent for an interrupt, which binds no
	// value and so has none to classify.
	Kind string `json:"kind,omitempty"`

	Usage string `json:"usage,omitempty"`

	// Default is the flag's default value, already rendered as a
	// string, or "" when it has none.
	Default string `json:"default,omitempty"`

	// ShowDefault reports that a formatter should print Default beside
	// Usage.
	ShowDefault bool `json:"showDefault,omitempty"`

	// Positional reports that the flag is an operand rather than an
	// option: named by position on the command line rather than by any
	// of Options.
	Positional bool `json:"positional,omitempty"`

	// EndOfOptions reports that every argument after this one is treated
	// as an argument rather than a flag, even if it begins with a dash, as
	// if the user had typed "--" after it.
	EndOfOptions bool `json:"endOfOptions,omitempty"`

	// Hidden reports that the flag is omitted from help output.
	Hidden bool `json:"hidden,omitempty"`

	// Persistent reports that the flag may be written beneath the command
	// that declares it, after any of its subcommands is named. Any other
	// flag is valid only until a subcommand is named.
	Persistent bool `json:"persistent,omitempty"`

	MinCount int `json:"minCount,omitempty"`

	// MaxCount is the most times the flag may be given, or 0 for no
	// limit.
	MaxCount int `json:"maxCount"`

	// EnvVar is the name of the environment variable that supplies the
	// flag's value when it is not given on the command line, or "" when
	// it has none. The variable's own value never appears here.
	EnvVar string `json:"envVar,omitempty"`

	// Choices lists the only values the flag accepts, or is empty when
	// any value is accepted.
	Choices []string `json:"choices,omitempty"`

	// TakesValue reports whether naming the flag on the command line
	// consumes a value. A boolean flag reports false: naming it alone
	// stands for true. Every other flag, including every positional
	// argument, reports true.
	TakesValue bool `json:"takesValue,omitempty"`

	// Options is every option that reaches the flag: the canonical
	// name's option, then the short name's, then any further aliases,
	// then every option a dialect generated, in the order of the named
	// options each was generated from. The order is part of the format,
	// so a formatter can tell a short name from an alias without
	// counting dashes. A positional argument has none.
	Options []Option `json:"options,omitempty"`
}
