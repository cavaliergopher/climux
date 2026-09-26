// This example demonstrates middleware: a wrapper around a command's
// handler, declared once on an ancestor and inherited by every command
// beneath it, for the work every command in a subtree has to do.
package climux

import (
	"context"
	"fmt"
	"os"
	"time"
)

// auditFlags is the settings struct the fictional platform team owns. Its
// actor is set from a flag on the root command, and the middleware below
// reads it back at call time rather than being handed a copy at
// registration, so mounting order cannot matter.
var auditFlags struct {
	actor string
}

// requireActor refuses to run anything until the caller has said who they
// are. A middleware that returns without calling next stops the
// invocation, which is what makes it an authorization check rather than a
// hook, and Exitf names the exit code the refusal exits with.
func requireActor(next HandlerFunc) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		if auditFlags.actor == "" {
			return Exitf(ExitCodeUsage, "%s: pass --actor to say who is running this", inv.Cmd.FullName)
		}
		fmt.Fprintf(inv.Stderr, "audit: %s by %s\n", inv.Cmd.FullName, auditFlags.actor)
		return next(ctx, inv)
	}
}

// timing reports how long the handler took. Wrapping means the handler's
// error passes through untouched: a middleware that only observes must
// return what it was given.
func timing(next HandlerFunc) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		start := time.Now()
		err := next(ctx, inv)
		_ = time.Since(start) // a real one would report it
		fmt.Fprintf(inv.Stderr, "trace: %s finished\n", inv.Cmd.FullName)
		return err
	}
}

func Example_middleware() {
	ctx := context.Background()

	// Both wrappers are declared once, on the root. Every handler in the
	// tree runs inside them, in the order given here, and neither
	// subcommand below knows they exist.
	//
	// The tree is built per run because one command line is all a tree
	// reads: this example shows three, where a program shows one. The
	// variable is shared across them, so the flag says what an unnamed
	// --actor means rather than leaving the last run's answer in place.
	newApp := func() *Command {
		return NewCommand("fleet", "Operate the fleet").
			Middleware(requireActor, timing).
			Flags(String(&auditFlags.actor, "actor", "Who is running this").Default("")).
			Subcommands(
				NewCommand("restart", "Restart a service").
					HandleFunc(func(ctx context.Context, inv *Invocation) error {
						fmt.Fprintln(inv.Stdout, "restarted")
						return nil
					}),
				NewCommand("status", "Report what is running").
					HandleFunc(func(ctx context.Context, inv *Invocation) error {
						fmt.Fprintln(inv.Stdout, "all green")
						return nil
					}),
			)
	}

	// Both wrappers report on stderr, as does Run when one of them
	// refuses, so the example points that stream at stdout to show
	// everything in one place. A real program would leave it alone.
	toStdout := WithStderr(os.Stdout)

	fmt.Println("+ fleet --actor=alice restart")
	Run(ctx, newApp(), WithArgs("--actor=alice", "restart"), toStdout)

	fmt.Println()
	fmt.Println("+ fleet --actor=alice status")
	Run(ctx, newApp(), WithArgs("--actor=alice", "status"), toStdout)

	// The wrapper refuses before the handler runs, so nothing is
	// restarted and the exit code is the one it named.
	fmt.Println()
	fmt.Println("+ fleet restart")
	fmt.Printf("exit code %d\n", Run(ctx, newApp(), WithArgs("restart"), toStdout))

	// Output:
	// + fleet --actor=alice restart
	// audit: fleet restart by alice
	// restarted
	// trace: fleet restart finished
	//
	// + fleet --actor=alice status
	// audit: fleet status by alice
	// all green
	// trace: fleet status finished
	//
	// + fleet restart
	// Error: fleet restart: pass --actor to say who is running this
	// exit code 2
}
