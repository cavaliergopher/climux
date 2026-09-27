// Package outcome describes what running a command line meant, in terms
// that do not depend on how climux exposes it. The runner reads one from
// the parse, then hands it to the handler that runs, which may change it:
// a handler that redispatches reports where the line really ended up.
package outcome

import (
	"context"

	"go.hotsrc.dev/climux"
)

// An Outcome is what running a command line meant.
type Outcome struct {
	Cmd       string   // full name of the command reached
	Flags     Flags    // flags the command line or environment set
	Interrupt string   // name of the interrupt given, if any
	Err       *Failure // non-nil if the line was rejected
}

// Flags maps a flag's declared name to what it was set to.
type Flags map[string]Bound

// A Bound is the value a flag holds and where it came from. Values have
// the type the flag was declared with: bool, int, string, []string. A
// flag that binds no value holds true once given.
type Bound struct {
	Value  any
	Source climux.Source
}

// A Failure is a command line the tool rejects. Arg is the argument it
// blames, if any.
type Failure struct {
	Arg string
}

type key struct{}

// NewContext returns a copy of ctx carrying o, for the handler the
// runner calls.
func NewContext(ctx context.Context, o *Outcome) context.Context {
	return context.WithValue(ctx, key{}, o)
}

// FromContext returns the outcome ctx carries, for a handler to change.
func FromContext(ctx context.Context) *Outcome {
	return ctx.Value(key{}).(*Outcome)
}

// NoOpHandler is a handler that leaves the outcome as the line parsed.
func NoOpHandler(ctx context.Context, inv *climux.Invocation) error {
	return nil
}
