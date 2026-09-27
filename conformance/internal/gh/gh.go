// Package gh models the parts of gh the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package gh

import (
	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/conformance/internal/outcome"
)

// PR mimics '$ gh pr checkout', and '$ gh co' with co aliased to
// "pr checkout".
func PR() *climux.Command {
	return climux.NewCommand("gh", "").Subcommands(
		climux.NewCommand("pr", "").Subcommands(checkoutCommand("checkout")),
		checkoutCommand("co"),
	)
}

func checkoutCommand(name string) *climux.Command {
	return climux.NewCommand(name, "").HandleFunc(outcome.NoOpHandler).Flags(
		climux.String("branch", "").Aliases("b"),
		climux.Bool("detach", ""),
		climux.Bool("force", "").Aliases("f"),
		climux.String("NUMBER", "").Positional().Required(),
	)
}
