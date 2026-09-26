package climux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"go.hotsrc.dev/climux/ir"
)

// tracer records the order in which middleware and handlers ran, which is
// the whole of what these tests assert: middleware is composition, so
// nothing about it is observable except who ran, in what order, and
// whether the wrapped handler ran at all.
type tracer struct {
	steps []string
}

// step returns a Middleware that records name on entry and again on exit,
// so a test can tell an outer wrapper from an inner one.
func (tr *tracer) step(name string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, inv *Invocation) error {
			tr.steps = append(tr.steps, name+" in")
			err := next(ctx, inv)
			tr.steps = append(tr.steps, name+" out")
			return err
		}
	}
}

// handler returns a HandlerFunc that records name and returns err.
func (tr *tracer) handler(name string, err error) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		tr.steps = append(tr.steps, name)
		return err
	}
}

func (tr *tracer) String() string { return strings.Join(tr.steps, ", ") }

// TestMiddlewareOrder asserts the two orderings that make middleware
// predictable: within one command, wrappers run in the order declared, and
// across the path, a wrapper declared higher in the tree runs outside one
// declared lower.
func TestMiddlewareOrder(t *testing.T) {
	var tr tracer
	sub := NewCommand("sub", "").
		Middleware(tr.step("sub")).
		HandleFunc(tr.handler("handler", nil))
	app := NewCommand("app", "").
		Middleware(tr.step("root1"), tr.step("root2")).
		Subcommands(sub)

	if err := Dispatch(context.Background(), app, WithArgs("sub")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "root1 in, root2 in, sub in, handler, sub out, root2 out, root1 out"
	assertString(t, want, tr.String())
}

// TestMiddlewareAppends asserts that repeated calls append rather than
// replace, so a command assembled in stages -- by a constructor and then
// by whoever mounts it -- keeps every wrapper.
func TestMiddlewareAppends(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		Middleware(tr.step("first")).
		Middleware(tr.step("second")).
		HandleFunc(tr.handler("handler", nil))

	if err := Dispatch(context.Background(), app, WithArgs()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "first in, second in, handler, second out, first out", tr.String())
}

// TestMiddlewareWrapsSiblingsIndependently asserts that a subcommand's own
// middleware does not leak onto its siblings, which is what makes
// inheritance a path rather than a tree-wide broadcast.
func TestMiddlewareWrapsSiblingsIndependently(t *testing.T) {
	var tr tracer
	audited := NewCommand("audited", "").
		Middleware(tr.step("audit")).
		HandleFunc(tr.handler("audited handler", nil))
	plain := NewCommand("plain", "").
		HandleFunc(tr.handler("plain handler", nil))
	app := NewCommand("app", "").
		Middleware(tr.step("root")).
		Subcommands(audited, plain)

	ctx := context.Background()
	if err := Dispatch(ctx, app, WithArgs("plain")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "root in, plain handler, root out", tr.String())

	tr.steps = nil
	if err := Dispatch(ctx, app, WithArgs("audited")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "root in, audit in, audited handler, audit out, root out", tr.String())
}

// TestMiddlewareRefusesInvocation asserts that a wrapper that returns
// without calling the handler it wrapped stops the invocation, and that
// Run maps its error to an exit code exactly as it would the handler's --
// which is what makes a middleware an authorization check.
func TestMiddlewareRefusesInvocation(t *testing.T) {
	handled := false
	var stderr strings.Builder
	app := NewCommand("app", "").
		Middleware(func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, inv *Invocation) error {
				return Exitf(7, "not authorized")
			}
		}).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			handled = true
			return nil
		})

	if code := Run(context.Background(), app, WithArgs(), WithStderr(&stderr)); code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
	if handled {
		t.Error("handler ran behind a middleware that did not call it")
	}
	assertString(t, "Error: not authorized\n", stderr.String())
}

// TestMiddlewareSeesParsedFlags asserts that middleware runs after the
// command line is applied, so a wrapper may read the flags the invocation
// set rather than only the invocation itself.
func TestMiddlewareSeesParsedFlags(t *testing.T) {
	var actor, seen string
	app := NewCommand("app", "").
		Flags(String(&actor, "actor", "")).
		Middleware(func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, inv *Invocation) error {
				seen = actor
				return next(ctx, inv)
			}
		}).
		HandleFunc(func(ctx context.Context, inv *Invocation) error { return nil })

	if err := Dispatch(context.Background(), app, WithArgs("--actor=alice")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "alice", seen)
}

// TestMiddlewareSkippedWithoutHandler asserts the two dispatches that run
// no handler -- a request for help, and a command that only groups
// subcommands -- run no middleware either. A wrapper exists to wrap a
// handler, so with none to wrap it has nothing to say.
func TestMiddlewareSkippedWithoutHandler(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		HelpFlag().
		Middleware(tr.step("root")).
		Subcommands(NewCommand("sub", "").HandleFunc(tr.handler("handler", nil)))

	ctx := context.Background()
	if err := Dispatch(ctx, app, WithArgs("--help"), WithStdout(io.Discard)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var argErr *ir.ArgumentError
	if err := Dispatch(ctx, app, WithArgs()); !errors.As(err, &argErr) {
		t.Fatalf("error = %v, want an *ir.ArgumentError", err)
	}
	assertString(t, "", tr.String())
}

// TestHandlerIsWrappedAtCompileTime asserts that a compiled command's
// Handler is the whole of what the command does -- its own handler inside
// every wrapper on its path -- so that calling it is all a dispatcher has
// to do, and that lowering runs none of those wrappers.
func TestHandlerIsWrappedAtCompileTime(t *testing.T) {
	var tr tracer
	sub := NewCommand("sub", "").
		Middleware(tr.step("sub")).
		HandleFunc(tr.handler("handler", nil))
	app := NewCommand("app", "").
		Middleware(tr.step("root")).
		Subcommands(sub)

	node, err := app.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "", tr.String())

	subNode := node.Subcommands[0]
	if err := subNode.Handler(context.Background(), &Invocation{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertString(t, "root in, sub in, handler, sub out, root out", tr.String())
}

// TestGroupingCommandReportsUsageError asserts what lowering substitutes
// for a command that declared no handler: one reporting an argument error
// against that command, and running none of the wrappers on its path,
// there being no handler to wrap.
func TestGroupingCommandReportsUsageError(t *testing.T) {
	var tr tracer
	app := NewCommand("app", "").
		Middleware(tr.step("root")).
		Subcommands(NewCommand("sub", "").HandleFunc(tr.handler("handler", nil)))

	node, err := app.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if node.Handler == nil {
		t.Fatal("a command that declared no handler compiled to a nil one")
	}
	var argErr *ir.ArgumentError
	if err := node.Handler(context.Background(), &Invocation{}); !errors.As(err, &argErr) {
		t.Fatalf("error = %v, want an *ir.ArgumentError", err)
	}
	if argErr.Cmd != node {
		t.Errorf("error names %v, want %v", argErr.Cmd, node)
	}
	assertString(t, "", tr.String())
}

// TestMiddlewareNilIsConfigError asserts that a nil wrapper is reported as
// a configuration error rather than panicking when it is applied, and that
// the error names the command that declared it.
func TestMiddlewareNilIsConfigError(t *testing.T) {
	sub := NewCommand("sub", "").Middleware(nil)
	app := NewCommand("app", "").Subcommands(sub)

	_, err := app.Compile()
	var cfgErr *ir.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error = %v, want an *ir.ConfigError", err)
	}
	if got, want := fmt.Sprint(err), "sub: middleware must not be nil"; !strings.Contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}
}
