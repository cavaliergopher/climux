// Command orbital is a fictional platform team's deploy and operations
// CLI. It shows how a large engineering org assembles one climux binary
// out of packages owned by different teams: main only wires the tree
// together, and each internal package owns its own commands, flags and
// handlers.
package main

import (
	"context"
	"os"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/config"
	"go.hotsrc.dev/climux/examples/orbital/internal/deploy"
	"go.hotsrc.dev/climux/examples/orbital/internal/execcmd"
	"go.hotsrc.dev/climux/examples/orbital/internal/fleet"
	"go.hotsrc.dev/climux/examples/orbital/internal/identity"
	"go.hotsrc.dev/climux/examples/orbital/internal/legacy"
	"go.hotsrc.dev/climux/examples/orbital/internal/logscmd"
	"go.hotsrc.dev/climux/examples/orbital/internal/middleware"

	// Linked in for what they register into climux.DefaultRegistry, which
	// Mount below picks up: telemetry's --log-level and --trace flags with
	// the wrapper that honors them, and the hidden "debug" command. Adding
	// a library to the binary is the whole of adding its command line.
	_ "go.hotsrc.dev/climux/examples/orbital/internal/debug"
	_ "go.hotsrc.dev/climux/examples/orbital/internal/telemetry"
)

// version is what a build stamps into the binary. Both --version and the
// "version" subcommand print it, so the two never disagree.
const version = "1.4.2"

func main() {
	client := fleet.NewClient("us-west-2")

	root := climux.NewCommand("orbital", "Deploy and operate services on the fleet").
		// Declared first, by convention, so both head the list of
		// options. Both are interrupts, so both answer before --actor is
		// missed: "orbital --version" works without an identity.
		HelpFlag().
		VersionFlag(version).
		EnableCompletion().
		// Declared once here and inherited by every command in the tree,
		// so --out redirects whichever one runs. The audit check is not:
		// only the commands that change fleet state declare it. The
		// timing trace is neither, arriving instead with the telemetry
		// flags it reads; see the mount below.
		Middleware(middleware.Output).
		Description(
			"orbital is the platform team's command line front end to the\n"+
				"fleet API. Each subcommand below is owned and versioned by the\n"+
				"team named in its help text; orbital itself only assembles\n"+
				"them into one binary.",
		).
		FlagGroups(legacy.FlagGroup()).
		Mount(climux.DefaultRegistry).
		Flags(
			climux.String(&identity.Actor, "actor", "", "Identity performing this action, recorded for the audit trail").
				Required().
				Env("ORBITAL_ACTOR").
				Persistent(),
			middleware.OutputFlag(),
		).
		Subcommands(
			config.Command(),
			deploy.Command(client),
			logscmd.Command(client),
			execcmd.Command(),
			// An interrupt too, like the flag above: "orbital version"
			// answers without --actor the same way "orbital --version"
			// does.
			climux.VersionCommand(version),
			// Also an interrupt, so tooling can ask "orbital schema" for
			// the whole tree without an identity to hand it.
			climux.SchemaCommand(),
		)

	ctx, stop := climux.NotifyContext(context.Background())
	defer stop()
	os.Exit(climux.Run(ctx, root))
}
