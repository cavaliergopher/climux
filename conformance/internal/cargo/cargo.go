// Package cargo models the parts of cargo the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package cargo

import "go.hotsrc.dev/climux"

// Run mimics '$ cargo run' and its builtin alias '$ cargo r'.
func Run() *climux.Command {
	// climux has no command alias, so r is declared as a second copy of
	// run.
	return climux.NewCommand("cargo", "").Subcommands(run("run"), run("r"))
}

func run(name string) *climux.Command {
	return climux.NewCommand(name, "").Flags(
		climux.Bool("release", "").Aliases("r"),
		climux.Strings("ARGS", "").Positional().EndOfOptions(),
	)
}
