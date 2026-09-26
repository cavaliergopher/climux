package deploy

import (
	"context"
	"fmt"
	"time"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/fleet"
)

// statusCommand returns "orbital deploy status SERVICE". It is read-only,
// so it declares no middleware.Audit; it still runs inside the timing
// trace the root declared, which every command in the tree inherits.
func statusCommand(client *fleet.Client) *climux.Command {
	service := climux.String("SERVICE", "Service to inspect").
		Positional().
		Required().
		State()
	timeout := climux.Duration("timeout", "How long to wait for the fleet API").
		Default(5 * time.Second).
		ShowDefault().
		State()
	return climux.NewCommand("status", "Show the rollout status of a service").
		Flags(service, timeout).
		HandleFunc(func(ctx context.Context, inv *climux.Invocation) error {
			ctx, cancel := context.WithTimeout(ctx, timeout.Value())
			defer cancel()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond): // stand-in for a fleet API round trip
			}
			fmt.Fprintf(inv.Stdout, "%s: healthy (region=%s)\n", service.Value(), client.Region)
			return nil
		})
}
