package config

import (
	"context"
	"fmt"

	"go.hotsrc.dev/climux"
)

// getCommand returns "orbital config get KEY".
func getCommand() *climux.Command {
	key := climux.String("KEY", "Configuration key to read").
		Positional().
		Required().
		State()
	return climux.NewCommand("get", "Print the value of a configuration key").
		Flags(key).
		HandleFunc(func(ctx context.Context, inv *climux.Invocation) error {
			v, ok := store[key.Value()]
			if !ok {
				// A plain error: nothing about this is a usage mistake the
				// parser could have caught, so it is returned as-is and
				// exits 1 rather than naming its own code.
				return fmt.Errorf("unknown configuration key: %s", key.Value())
			}
			fmt.Fprintln(inv.Stdout, v)
			return nil
		})
}
