package deploy

import (
	"context"
	"fmt"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/fleet"
	"go.hotsrc.dev/climux/examples/orbital/internal/middleware"
)

// runCommand returns "orbital deploy run", the one command in this binary
// that changes what is running, so it declares middleware.Audit. The
// timing trace it also runs inside is the root's, inherited.
func runCommand(client *fleet.Client) *climux.Command {
	var (
		service    string
		release    string
		env        string
		strategy   string
		tags       []string
		confirm    bool
		skipHealth bool
	)
	return climux.NewCommand("run", "Roll out a new version of a service").
		Middleware(middleware.Audit).
		Flags(
			climux.String(&service, "service", "Service to deploy").
				Aliases("s").
				Required(),
			// Not "--version": the root mounts one to report orbital's
			// own, and a name claimed there is claimed for the whole
			// tree. Compile reports the clash rather than letting a
			// deploy silently print a version instead.
			climux.String(&release, "release", "Release to deploy, such as a git SHA").
				Required().
				Validate(validRelease),
			climux.String(&env, "env", "Environment to deploy to").Default("staging").
				Choices("staging", "production").
				ShowDefault(),
			climux.String(&strategy, "strategy", "Rollout strategy").Default("rolling").
				Choices("rolling", "blue-green", "canary").
				ShowDefault(),
			climux.Strings(&tags, "tag", "Metadata tag to attach to this rollout (repeatable)").
				NArgs(0, 5),
			climux.Bool(&confirm, "confirm", "Confirm a deploy to production"),
			climux.Bool(&skipHealth, "unsafe-skip-health-checks", "Skip post-deploy health checks").
				Hidden(),
		).
		HandleFunc(
			func(ctx context.Context, inv *climux.Invocation) error {
				if env == "production" && !confirm {
					// A misuse the parser could not catch on its own --
					// two flags whose combination matters -- names its
					// own exit code rather than the generic failure one.
					return climux.Exitf(4,
						"refusing to deploy %s to production without --confirm", service)
				}
				if err := client.Deploy(ctx, service, release); err != nil {
					return err
				}
				fmt.Fprintf(inv.Stdout, "%s: deployed %s using the %s strategy (tags: %v)\n",
					service, release, strategy, tags)
				if skipHealth {
					fmt.Fprintln(inv.Stderr, "warning: health checks skipped (--unsafe-skip-health-checks)")
				}
				return nil
			},
		)
}

// validRelease is a stand-in for real release validation.
func validRelease(arg string) error {
	if len(arg) < 4 {
		return fmt.Errorf("release %q is too short to identify a build", arg)
	}
	return nil
}
