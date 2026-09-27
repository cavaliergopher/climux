// Package kubectl models the parts of kubectl the conformance cases run.
// Each function returns a fresh tree, since a tree reads one command line.
package kubectl

import (
	"context"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/conformance/internal/outcome"
)

// Exec mimics '$ kubectl exec'.
func Exec() *climux.Command {
	return climux.NewCommand("kubectl", "").Subcommands(
		climux.NewCommand("exec", "").HandleFunc(outcome.NoOpHandler).Flags(
			climux.String("POD", "").Positional().Required(),
			climux.String("container", "").Aliases("c"),
			climux.Strings("COMMAND", "").Positional(),
		),
	)
}

// Run mimics '$ kubectl run'.
func Run() *climux.Command {
	return climux.NewCommand("kubectl", "").Subcommands(
		climux.NewCommand("run", "").HandleFunc(outcome.NoOpHandler).Flags(
			climux.String("NAME", "").Positional().Required(),
			climux.String("image", ""),
			// kubectl's --dry-run may be given bare; climux cannot yet
			// declare that, so this one always takes a value.
			climux.String("dry-run", "").Default("none"),
			climux.Strings("ARGS", "").Positional(),
		),
	)
}

// Logs mimics '$ kubectl logs', which refuses --since beside --since-time.
func Logs() *climux.Command {
	podFlag := climux.String("POD", "").Positional().Required()
	sinceFlag := climux.Duration("since", "")
	sinceTimeFlag := climux.String("since-time", "")
	logsHandler := func(ctx context.Context, inv *climux.Invocation) error {
		if !sinceFlag.State().IsSet() || !sinceTimeFlag.State().IsSet() {
			return nil
		}
		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "", "at most one of `sinceTime` or `sinceSeconds` may be specified")
	}
	logsCommand := climux.NewCommand("logs", "").Flags(podFlag, sinceFlag, sinceTimeFlag).HandleFunc(logsHandler)
	return climux.NewCommand("kubectl", "").Subcommands(logsCommand)
}
