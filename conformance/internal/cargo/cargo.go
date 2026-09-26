// Package cargo models the parts of cargo the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package cargo

import "go.hotsrc.dev/climux"

// Run mimics '$ cargo run' and its builtin alias '$ cargo r'.
func Run() *climux.Command {
	return climux.NewCommand("cargo", "").Subcommands(
		climux.NewCommand("run", "").Aliases("r").Flags(
			climux.Bool("release", "").Aliases("r"),
			climux.Strings("ARGS", "").Positional().EndOfOptions(),
		),
	)
}
