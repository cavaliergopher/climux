// Package kubectl models the parts of kubectl the conformance cases run.
// Each function returns a fresh tree, since a tree reads one command line.
package kubectl

import "go.hotsrc.dev/climux"

// Exec mimics '$ kubectl exec'.
func Exec() *climux.Command {
	return climux.NewCommand("kubectl", "").Subcommands(
		climux.NewCommand("exec", "").Flags(
			climux.String("POD", "").Positional().Required(),
			climux.String("container", "").Aliases("c"),
			climux.Strings("COMMAND", "").Positional(),
		),
	)
}

// Run mimics '$ kubectl run'.
func Run() *climux.Command {
	return climux.NewCommand("kubectl", "").Subcommands(
		climux.NewCommand("run", "").Flags(
			climux.String("NAME", "").Positional().Required(),
			climux.String("image", ""),
			// kubectl's --dry-run may be given bare; climux cannot yet
			// declare that, so this one always takes a value.
			climux.String("dry-run", "").Default("none"),
			climux.Strings("ARGS", "").Positional(),
		),
	)
}
