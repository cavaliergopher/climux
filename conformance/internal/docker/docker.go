// Package docker models the parts of docker the conformance cases run.
// Each function returns a fresh tree, since a tree reads one command line.
package docker

import "go.hotsrc.dev/climux"

// Run mimics '$ docker run'.
func Run() *climux.Command {
	return climux.NewCommand("docker", "").
		Flags(
			climux.Bool("debug", "").Aliases("D").Persistent(),
		).
		Subcommands(
			climux.NewCommand("run", "").Flags(
				climux.Bool("interactive", "").Aliases("i"),
				climux.Bool("tty", "").Aliases("t"),
				climux.Bool("rm", ""),
				climux.Strings("volume", "").Aliases("v"),
				climux.String("IMAGE", "").Positional().Required().EndOfOptions(),
				climux.Strings("ARG", "").Positional(),
			),
		)
}
