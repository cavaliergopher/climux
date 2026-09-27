// Package gh models the parts of gh the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package gh

import (
	"context"

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

// PRCreate mimics '$ gh pr create', which refuses --editor beside --web.
func PRCreate() *climux.Command {
	editorFlag := climux.Bool("editor", "").Aliases("e")
	webFlag := climux.Bool("web", "").Aliases("w")
	createHandler := func(ctx context.Context, inv *climux.Invocation) error {
		if !editorFlag.State().IsSet() || !webFlag.State().IsSet() {
			return nil
		}
		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "", "specify only one of `--editor` or `--web`")
	}
	createCommand := climux.NewCommand("create", "").Flags(editorFlag, webFlag).HandleFunc(createHandler)
	prCommand := climux.NewCommand("pr", "").Subcommands(createCommand)
	return climux.NewCommand("gh", "").Subcommands(prCommand)
}
