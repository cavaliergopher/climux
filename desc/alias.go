package desc

// Alias is a further name a command answers to on the command line.
type Alias struct {
	Name string `json:"name"`

	// Hidden reports that the alias is omitted from help output, though
	// the command line still reaches the command by it.
	Hidden bool `json:"hidden,omitempty"`
}
