// Package cargo models the parts of cargo the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package cargo

import (
	"context"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/conformance/internal/outcome"
)

// Run mimics '$ cargo run' and its builtin alias '$ cargo r'.
func Run() *climux.Command {
	return climux.NewCommand("cargo", "").Subcommands(
		climux.NewCommand("run", "").Aliases("r").HandleFunc(outcome.NoOpHandler).Flags(
			climux.Bool("release", "").Aliases("r"),
			climux.Strings("ARGS", "").Positional().EndOfOptions(),
		),
	)
}

// Build mimics '$ cargo build', whose --release conflicts with --profile.
func Build() *climux.Command {
	releaseFlag := climux.Bool("release", "").Aliases("r")
	profileFlag := climux.String("profile", "")
	buildHandler := func(ctx context.Context, inv *climux.Invocation) error {
		if !releaseFlag.State().IsSet() || !profileFlag.State().IsSet() {
			return nil
		}
		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "", "the argument '--release' cannot be used with '--profile <PROFILE-NAME>'")
	}
	buildCommand := climux.NewCommand("build", "").Flags(releaseFlag, profileFlag).HandleFunc(buildHandler)
	return climux.NewCommand("cargo", "").Subcommands(buildCommand)
}
