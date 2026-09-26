// Package telemetry stands in for a platform library that every team's
// binary links in for logging and tracing. It contributes the way a
// shared library should: a settings struct with a FlagGroup method, and
// the middleware that honors those flags, registered together into
// climux.DefaultRegistry so that any command mounting that registry picks
// up both. Registering them as one unit is the point -- --trace cannot be
// mounted into a binary that forgot to wrap its handlers, so the flag
// means the same thing everywhere.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.hotsrc.dev/climux"
)

// Settings holds telemetry configuration. A package that wants an
// isolated instance -- a test, say -- constructs its own Settings and
// calls FlagGroup on it, rather than reaching for the package-level Flags
// below; see example_group_test.go in the climux module root.
type Settings struct {
	LogLevel string
	Trace    bool
}

// FlagGroup returns a new group of flags bound to s.
func (s *Settings) FlagGroup() *climux.FlagGroup {
	return climux.NewFlagGroup(
		"telemetry", "Telemetry options",
		climux.String(&s.LogLevel, "log-level", "Set the log verbosity").Default("info").
			Choices("debug", "info", "warn", "error").
			ShowDefault().
			Persistent(),
		climux.Bool(&s.Trace, "trace", "Emit a timing trace for every command").
			Env("ORBITAL_TRACE").
			Persistent(),
	)
}

// Timing reports how long the handler took, but only when --trace asked
// for it. It reads s at call time rather than closing over a copy of the
// setting, because the flag is not applied until the command line parses,
// which is after every command in the tree was built.
func (s *Settings) Timing(next climux.HandlerFunc) climux.HandlerFunc {
	return func(ctx context.Context, inv *climux.Invocation) error {
		start := time.Now()
		err := next(ctx, inv)
		if s.Trace {
			fmt.Fprintf(inv.Stderr, "trace: %s took %s\n",
				inv.Cmd.FullName, time.Since(start))
		}
		return err
	}
}

// Flags is the instance orbital mounts. The init below registers its
// group and its middleware into climux.DefaultRegistry together, so any
// command that mounts that registry -- see main.go's
// Mount(climux.DefaultRegistry) -- picks up both with no further wiring,
// and a blank import of this package is all a program needs.
var Flags = &Settings{}

func init() {
	climux.DefaultRegistry.
		FlagGroups(Flags.FlagGroup()).
		Middleware(Flags.Timing)
}
