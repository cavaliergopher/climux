// This example demonstrates a Registry: what a platform team contributes
// to whichever programs mount it -- a flag, the middleware that honors
// that flag, and a subcommand -- registered once by the team and mounted
// in one line by the program.
package climux

import (
	"context"
	"fmt"
	"time"
)

// platformRegistry is the registry the fictional platform team owns. A
// team wanting its work in every binary registers into DefaultRegistry
// instead; a registry of its own lets a program mount this team's
// contributions and leave another's out.
var platformRegistry = new(Registry)

// platformSettings is the settings struct the team owns. Its flags bind
// to the receiver rather than to package variables, so a test can build
// an isolated instance the way example_group_test.go does.
type platformSettings struct {
	deadline time.Duration
}

// FlagGroup returns a new group of flags bound to s.
func (s *platformSettings) FlagGroup() *FlagGroup {
	return NewFlagGroup("platform", "Platform options",
		Duration("timeout", "Abort the command after this long").Bind(&s.deadline),
	)
}

// Wrap bounds a handler by whatever --timeout asked for. It reads
// s.deadline at call time rather than closing over a copy, because the
// flag is not applied until the command line parses, which is long after
// the tree was built.
func (s *platformSettings) Wrap(next HandlerFunc) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		if s.deadline <= 0 {
			return next(ctx, inv)
		}
		ctx, cancel := context.WithTimeout(ctx, s.deadline)
		defer cancel()
		return next(ctx, inv)
	}
}

var platformFlags = &platformSettings{}

// The flag, the wrapper that gives it meaning, and a subcommand the team
// contributes to every program are registered together. Registering them
// as one unit is the point: a program cannot mount the flag and miss the
// wrapper, so --timeout means the same thing in every binary.
func init() {
	platformRegistry.
		FlagGroups(platformFlags.FlagGroup()).
		Middleware(platformFlags.Wrap).
		Subcommands(
			NewCommand("diag", "Report platform diagnostics").
				HandleFunc(func(ctx context.Context, inv *Invocation) error {
					fmt.Fprintln(inv.Stdout, "diag: platform library v1")
					return nil
				}),
		)
}

func Example_registry() {
	ctx := context.Background()

	// The program mounts the registry in one line and names none of what
	// it holds. Its own subcommand is declared as usual; the team's
	// arrives with the mount, listed after it.
	app := NewCommand("myapp", "Do things").
		HelpFlag().
		Mount(platformRegistry).
		Subcommands(
			NewCommand("run", "Run the thing").
				HandleFunc(func(ctx context.Context, inv *Invocation) error {
					if deadline, ok := ctx.Deadline(); ok {
						fmt.Fprintf(inv.Stdout, "run: bounded, %v to go\n",
							time.Until(deadline).Round(time.Minute))
						return nil
					}
					fmt.Fprintln(inv.Stdout, "run: unbounded")
					return nil
				}),
		)

	fmt.Println("+ myapp --help")
	Run(ctx, app, WithArgs("--help"))

	// The registered wrapper runs whether or not the program knows it is
	// there, and it bounds the program's own handler rather than the
	// other way round.
	fmt.Println()
	fmt.Println("+ myapp run")
	Run(ctx, app, WithArgs("run"))

	fmt.Println()
	fmt.Println("+ myapp --timeout=30m run")
	Run(ctx, app, WithArgs("--timeout=30m", "run"))

	// The team's own subcommand needs no wiring in the program at all.
	fmt.Println()
	fmt.Println("+ myapp diag")
	Run(ctx, app, WithArgs("diag"))
	// Output:
	// + myapp --help
	// Usage: myapp [OPTIONS] COMMAND
	//
	// Do things
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//
	// Platform options:
	//    --timeout  Abort the command after this long
	//
	// Commands:
	//   run   Run the thing
	//   diag  Report platform diagnostics
	//
	// + myapp run
	// run: unbounded
	//
	// + myapp --timeout=30m run
	// run: bounded, 30m0s to go
	//
	// + myapp diag
	// diag: platform library v1
}
