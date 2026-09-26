// Package git models the parts of git the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package git

import "go.hotsrc.dev/climux"

// Bisect mimics '$ git bisect run'.
func Bisect() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		climux.NewCommand("bisect", "").Subcommands(
			climux.NewCommand("run", "").Flags(
				climux.Strings("CMD", "").Positional().NArgs(1, 0).EndOfOptions(),
			),
		),
	)
}

// Status mimics '$ git status', and '$ git st' with alias.st set to
// "status -sb" in the user's config.
func Status() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		climux.NewCommand("status", "").Aliases("st").Flags(
			climux.Bool("short", "").Aliases("s"),
			climux.Bool("branch", "").Aliases("b"),
			// git's -u may be given bare, which means "all". This one
			// needs a value, which -uno gives it; see optional-value.
			climux.String("untracked-files", "").Aliases("u"),
		),
	)
}

// Log mimics '$ git log'.
func Log() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		climux.NewCommand("log", "").Flags(
			climux.Bool("oneline", ""),
			climux.Unbound("end-of-options", "").EndOfOptions(),
			climux.Strings("REV", "").Positional(),
		),
	)
}
