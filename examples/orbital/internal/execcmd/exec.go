// Package execcmd implements "orbital exec --service NAME CMD ARGS...",
// the one place orbital hands arguments to something else rather than
// interpreting them itself: CMD ends option processing, so the command's
// own options arrive in ARGS intact instead of being read as orbital's.
package execcmd

import (
	"context"
	"fmt"
	"strings"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/middleware"
)

// Command returns the "exec" command.
func Command() *climux.Command {
	var service, command string
	var args []string
	return climux.NewCommand("exec", "Run a one-off command inside a service's container").
		Middleware(middleware.Audit).
		Flags(
			climux.String(&service, "service", "Service whose container to exec into").
				Aliases("s").
				Required(),
			climux.String(&command, "cmd", "Command to run inside the container").
				Positional().
				Required().
				EndOfOptions(),
			climux.Strings(&args, "arg", "Arguments to the command").
				Positional(),
		).
		HandleFunc(
			func(ctx context.Context, inv *climux.Invocation) error {
				fmt.Fprintf(inv.Stdout, "%s: would run: %s\n",
					service, strings.Join(append([]string{command}, args...), " "))
				return nil
			},
		)
}
