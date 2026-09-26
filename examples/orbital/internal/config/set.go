package config

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/middleware"
)

// setCommand returns "orbital config set KEY VALUE". It mutates the local
// store, so it declares middleware.Audit like the other mutating
// commands; "config get" does not.
func setCommand() *climux.Command {
	var key, value string
	return climux.NewCommand("set", "Set a configuration key to a value").
		Middleware(middleware.Audit).
		Flags(
			climux.String(&key, "KEY", "Configuration key to set").
				Positional().
				Required().
				Validate(validKey),
			climux.String(&value, "VALUE", "New value").
				Positional().
				Required(),
		).
		HandleFunc(
			func(ctx context.Context, inv *climux.Invocation) error {
				store[key] = value
				fmt.Fprintf(inv.Stdout, "%s = %s\n", key, value)
				return nil
			},
		)
}

// validKey rejects a key the store doesn't already recognize, naming the
// legal ones so a typo is easy to fix.
func validKey(arg string) error {
	if _, ok := store[arg]; ok {
		return nil
	}
	keys := make([]string, 0, len(store))
	for k := range store {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Errorf("unknown key %q, expected one of: %s", arg, strings.Join(keys, ", "))
}
