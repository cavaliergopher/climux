package desc

// Command describes a command that users may invoke from the command
// line: its own name, the flags it accepts and every subcommand beneath
// it.
//
// A Command names only the flags declared or mounted on it, not those
// inherited from an ancestor. A flag is in scope for a command from the
// point its own command is named onward, so an ancestor's flags are not
// repeated on each of its descendants; see the machine-readable-schema
// ADR.
type Command struct {
	// Name is the command's own name, unqualified by its ancestry: "add"
	// rather than "orbital remote add".
	Name string `json:"name"`

	// FullName is Name joined with each ancestor's, from the root down:
	// "orbital remote add".
	FullName string `json:"fullName"`

	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`

	// Hidden reports that the command is omitted from help output.
	Hidden bool `json:"hidden,omitempty"`

	FlagGroups  []*FlagGroup `json:"flagGroups,omitempty"`
	Subcommands []*Command   `json:"subcommands,omitempty"`
}
