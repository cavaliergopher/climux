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
	service := climux.String("service", "Service to deploy").
		Aliases("s").
		Required().
		State()
	// Not "--version": the root mounts one to report orbital's own, and
	// a name claimed there is claimed for the whole tree. Compile reports
	// the clash rather than letting a deploy silently print a version
	// instead.
	release := climux.String("release", "Release to deploy, such as a git SHA").
		Required().
		Validate(validRelease).
		State()
	env := climux.String("env", "Environment to deploy to").
		Default("staging").
		Choices("staging", "production").
		ShowDefault().
		State()
	strategy := climux.String("strategy", "Rollout strategy").
		Default("rolling").
		Choices("rolling", "blue-green", "canary").
		ShowDefault().
		State()
	tags := climux.Strings("tag", "Metadata tag to attach to this rollout (repeatable)").
		NArgs(0, 5).
		State()
	confirm := climux.Bool("confirm", "Confirm a deploy to production").State()
	skipHealth := climux.Bool("unsafe-skip-health-checks", "Skip post-deploy health checks").
		Hidden().
		State()
	return climux.NewCommand("run", "Roll out a new version of a service").
		Middleware(middleware.Audit).
		Flags(service, release, env, strategy, tags, confirm, skipHealth).
		HandleFunc(
			func(ctx context.Context, inv *climux.Invocation) error {
				if env.Value() == "production" && !confirm.Value() {
					// A misuse the parser could not catch on its own --
					// two flags whose combination matters -- names its
					// own exit code rather than the generic failure one.
					return climux.Exitf(4,
						"refusing to deploy %s to production without --confirm", service.Value())
				}
				if err := client.Deploy(ctx, service.Value(), release.Value()); err != nil {
					return err
				}
				fmt.Fprintf(inv.Stdout, "%s: deployed %s using the %s strategy (tags: %v)\n",
					service.Value(), release.Value(), strategy.Value(), tags.Value())
				if skipHealth.Value() {
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
