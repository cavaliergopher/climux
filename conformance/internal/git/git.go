// Package git models the parts of git the conformance cases run. Each
// function returns a fresh tree, since a tree reads one command line.
package git

import (
	"context"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/conformance/internal/outcome"
)

// Bisect mimics '$ git bisect run'.
func Bisect() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		climux.NewCommand("bisect", "").Subcommands(
			climux.NewCommand("run", "").HandleFunc(outcome.NoOpHandler).Flags(
				climux.Strings("CMD", "").Positional().NArgs(1, 0).EndOfOptions(),
			),
		),
	)
}

// Status mimics '$ git status', and '$ git st' with alias.st set to
// "status -sb" in the user's config.
func Status() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		statusCommand("status").HandleFunc(outcome.NoOpHandler),
		statusCommand("st").Hidden().HandleFunc(stHandler),
	)
}

func statusCommand(name string) *climux.Command {
	return climux.NewCommand(name, "").Flags(
		climux.Bool("short", "").Aliases("s"),
		climux.Bool("branch", "").Aliases("b"),
		// git's -u may be given bare, which means "all". This one
		// needs a value, which -uno gives it; see optional-value.
		climux.String("untracked-files", "").Aliases("u"),
	)
}

// stHandler redispatches to status with the -sb the alias carries,
// which git reads as if it were typed.
func stHandler(ctx context.Context, inv *climux.Invocation) error {
	o := outcome.FromContext(ctx)
	o.Cmd = "git status"
	if o.Flags == nil {
		o.Flags = outcome.Flags{}
	}
	o.Flags["short"] = outcome.Bound{Value: true, Source: climux.SourceArgs}
	o.Flags["branch"] = outcome.Bound{Value: true, Source: climux.SourceArgs}
	return nil
}

// Log mimics '$ git log'.
func Log() *climux.Command {
	return climux.NewCommand("git", "").Subcommands(
		climux.NewCommand("log", "").HandleFunc(outcome.NoOpHandler).Flags(
			climux.Bool("oneline", ""),
			climux.Unbound("end-of-options", "").EndOfOptions(),
			climux.Strings("REV", "").Positional(),
		),
	)
}

// Commit mimics '$ git commit', which refuses -m beside -F.
func Commit() *climux.Command {
	messageFlag := climux.String("message", "").Aliases("m")
	fileFlag := climux.String("file", "").Aliases("F")
	commitHandler := func(ctx context.Context, inv *climux.Invocation) error {
		if !messageFlag.State().IsSet() || !fileFlag.State().IsSet() {
			return nil
		}
		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "", "options '-m' and '-F' cannot be used together")
	}
	commitCommand := climux.NewCommand("commit", "").Flags(messageFlag, fileFlag).HandleFunc(commitHandler)
	return climux.NewCommand("git", "").Subcommands(commitCommand)
}
