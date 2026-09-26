// Package middleware holds the wrappers orbital itself puts around its
// handlers: an --out redirection that every command in the binary runs
// inside, and an audit check that only the commands changing fleet state
// do.
//
// Both are climux.Middleware, so neither is registered by the handler it
// wraps. main.go declares Output on the root, which inherits down the
// whole tree, and each mutating command declares Audit on itself; a
// read-only command such as "deploy status" declares nothing and so is
// not audited. That is the point of the mechanism here: the platform team
// owns what wraps a handler without touching the team that wrote it.
//
// A wrapper belonging to a shared library goes in that library instead,
// registered beside the flags it reads -- see internal/telemetry, whose
// Timing wrapper arrives with --trace rather than being declared here.
package middleware

import (
	"context"
	"fmt"
	"os"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/examples/orbital/internal/identity"
)

// Audit refuses to run as the placeholder "anonymous" identity, in place
// of a real authorization check against an actor directory. Returning
// without calling next is how a middleware refuses an invocation.
//
// It reads identity.Actor at call time rather than closing over a copy,
// because the root --actor flag it comes from is not applied until the
// command line parses, which is after every command in the tree was
// built.
func Audit(next climux.HandlerFunc) climux.HandlerFunc {
	return func(ctx context.Context, inv *climux.Invocation) error {
		if identity.Actor == "anonymous" {
			return fmt.Errorf(
				"%s: refusing to run as %q; pass --actor or set ORBITAL_ACTOR",
				inv.Cmd.FullName, identity.Actor,
			)
		}
		return next(ctx, inv)
	}
}

// outFile is set by the --out flag below. It is read at call time for the
// same reason identity.Actor is: the flag is not applied until the
// command line parses, long after the tree was built.
var outFile string

// OutputFlag returns the --out flag Output reads. The flag ships with the
// middleware because neither is any use without the other; main.go
// declares both on the root, so every command in the tree can be
// redirected.
func OutputFlag() climux.Flag {
	return climux.String(&outFile, "out",
		"Write command output to FILE instead of stdout").
		ValueName("file").
		Persistent()
}

// Output sends whatever the handler writes to inv.Stdout to the file
// named by --out, and closes it once the handler returns. Replacing a
// stream on inv is how a middleware changes what the handler it wraps
// works with: the handler writes inv.Stdout as it always does and never
// learns it is writing a file.
//
// Help is not redirected, because asking for help does not run a handler
// and so runs no middleware either.
func Output(next climux.HandlerFunc) climux.HandlerFunc {
	return func(ctx context.Context, inv *climux.Invocation) error {
		if outFile == "" {
			return next(ctx, inv)
		}
		f, err := os.Create(outFile)
		if err != nil {
			return err
		}
		// The invocation outlives this wrapper, so the stream goes back
		// as it was rather than leaving a closed file behind for whatever
		// declared Output to write to.
		stdout := inv.Stdout
		inv.Stdout = f
		defer func() { inv.Stdout = stdout }()

		err = next(ctx, inv)
		if cerr := f.Close(); err == nil {
			// A handler that succeeded but whose output did not reach the
			// disk has still failed the caller.
			err = cerr
		}
		return err
	}
}
