package climux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"go.hotsrc.dev/climux/internal/argv"
	"go.hotsrc.dev/climux/ir"
)

// A RunOption replaces something Run takes from the process: the
// command line it reads, or the streams it and the command's handlers
// write. Pass none and a program behaves as a command line program
// does.
type RunOption func(*runConfig)

// runConfig is what the options build: everything Run takes from the
// process, resolved once before anything runs.
type runConfig struct {
	args   []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func newRunConfig(opts []RunOption) *runConfig {
	cfg := &runConfig{
		args:   os.Args[1:],
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WithArgs reads args as the command line, in place of the arguments the
// program was started with.
//
//	climux.Run(ctx, cmd, climux.WithArgs("deploy", "--dry-run"))
func WithArgs(args ...string) RunOption {
	return func(cfg *runConfig) { cfg.args = args }
}

// WithStdin reads r as Invocation.Stdin, in place of the process's
// standard input.
func WithStdin(r io.Reader) RunOption {
	return func(cfg *runConfig) { cfg.stdin = r }
}

// WithStdout writes w as Invocation.Stdout, in place of the process's
// standard output. Help and every other answer an interrupt gives go
// here too.
func WithStdout(w io.Writer) RunOption {
	return func(cfg *runConfig) { cfg.stdout = w }
}

// WithStderr writes w as Invocation.Stderr, in place of the process's
// standard error. Errors and the usage that follows them go here too.
func WithStderr(w io.Writer) RunOption {
	return func(cfg *runConfig) { cfg.stderr = w }
}

// Run runs the command or subcommand named by the command line and
// returns the exit code the program should terminate with:
//
//	0  the handler returned nil, or an interrupt such as --help ran
//	1  the handler returned an error
//	2  the command line or the command tree was wrong
//
// A handler names its own exit code by returning an error that
// implements ExitCoder. See Exit and Exitf.
//
// The command line comes from the arguments the program was started
// with, and the streams from the process. See WithArgs and WithStdout
// for reading and writing somewhere else, which is what a test does.
//
//	func main() {
//	    ctx, stop := climux.NotifyContext(context.Background())
//	    defer stop()
//	    os.Exit(climux.Run(ctx, cmd))
//	}
//
// Help is printed to stdout. An error is printed to stderr, followed by
// the command's usage when the command line was at fault.
//
// ctx reaches the handler unchanged. See NotifyContext.
func Run(ctx context.Context, cmd *Command, opts ...RunOption) int {
	cfg := newRunConfig(opts)

	// The one place an ordinary invocation compiles the tree. Everything
	// below takes the compiled node, so lowering happens once however
	// many steps consult it.
	node, err := cmd.Compile()
	if err != nil {
		return reportConfigError(err, cfg.stderr)
	}
	if cmd.completionEnabled {
		if code, handled := completionHook(node, cfg.stdout); handled {
			return code
		}
	}
	return report(node, dispatch(ctx, node, cfg), cfg.stderr)
}

// Dispatch runs the command or subcommand named by the command line and
// returns the handler's error, or the error that stopped the command
// line from being read. It prints nothing and maps nothing to an exit
// code.
//
// Reach for it in a program that reports errors its own way, or embeds
// climux in a larger command loop: Run is Dispatch plus the reporting
// and the exit code. It takes the same options; see WithArgs.
//
// An interrupt such as --help is not an error: it runs in place of the
// command, and what it returns is returned here.
func Dispatch(ctx context.Context, cmd *Command, opts ...RunOption) error {
	node, err := cmd.Compile()
	if err != nil {
		return err
	}
	return dispatch(ctx, node, newRunConfig(opts))
}

// Parse reads args into the flags cmd and its subcommands declare and
// returns an Invocation naming the command the arguments reached. No
// handler runs.
//
// Reach for it in a test, or in a program that wants the parsed result
// without dispatching. Parse reads one command line against cmd; calling
// it again with the same tree is not supported.
//
// If an interrupt such as --help was given, the returned Invocation names
// it, and an argument the line left out is not reported.
//
// The Invocation's streams are the process's, since Parse runs nothing
// that would write to them. See Run to read and write somewhere else.
func Parse(cmd *Command, args ...string) (*Invocation, error) {
	node, err := cmd.Compile()
	if err != nil {
		return nil, err
	}
	inv, err := argv.Parse(node, args)
	if err != nil {
		return nil, err
	}
	inv.Stdin, inv.Stdout, inv.Stderr = os.Stdin, os.Stdout, os.Stderr
	return inv, nil
}

// Complete returns the shell completion candidates for a command line that
// is still being typed. args is the line so far, without the program name
// or the word under the cursor; word is that word, possibly empty.
//
// Reach for it to test completion without a shell in the loop: it is the
// engine behind the reply Run sends one. A command tree that does not
// compile completes nothing.
func Complete(cmd *Command, args []string, word string) ([]string, ir.CompDirective) {
	node, err := cmd.Compile()
	if err != nil {
		return nil, ir.CompNoFileComp
	}
	return argv.Complete(node, args, word)
}

// dispatch is Dispatch against a tree that is already compiled, which is
// what lets Run lower the tree once and hand the same node to everything
// below it.
//
// It is where an Invocation gets its streams: the parser leaves them nil,
// having no business knowing what a process was given, and everything
// that runs from here reads and writes what cfg resolved.
func dispatch(ctx context.Context, cmd *ir.Command, cfg *runConfig) error {
	inv, err := argv.Parse(cmd, cfg.args)
	if err != nil {
		return err
	}
	inv.Stdin, inv.Stdout, inv.Stderr = cfg.stdin, cfg.stdout, cfg.stderr
	if inv.Interrupt != nil {
		return inv.Interrupt.Handler(ctx, inv)
	}
	return inv.Cmd.Handler(ctx, inv)
}

// report reports err against cmd, an already-compiled tree, and returns
// the exit code the program should terminate with. A joined error -- a
// wrong command line can report more than one fault -- prints one
// prefixed line per error.
//
// Every line goes to stderr, which is the process's unless the Run call
// replaced it. An argument error is followed by the usage of the command
// it names, so the reader who mistyped sees what to type instead; see
// docs/adr/argument-errors-print-usage.md.
//
// A configuration error reaches here only from a tree that compiled and
// then failed anyway -- restoring a default through Set is the one way
// that happens. The fault is the program's rather than the user's, so it
// prints no usage. A tree that failed to compile at all never reaches
// here; see reportConfigError.
func report(cmd *ir.Command, err error, stderr io.Writer) int {
	if err == nil {
		return ExitCodeSuccess
	}

	for _, e := range ir.FlattenErrors(err) {
		errTypeName := "Error"

		var argErr *ir.ArgumentError
		switch {
		case errors.As(e, &argErr):
			errTypeName = "Argument error"

		case errors.As(e, new(*ir.ConfigError)):
			// Both exit 2, so the prefix is all that says whether the
			// fault was the user's or the program's.
			errTypeName = "Program error"
		}

		// A failure to write is ignored, as the flag package ignores it:
		// there is nowhere left to report that reporting failed, and the
		// exit code already says what happened.
		fmt.Fprintf(stderr, "%s: %s\n", errTypeName, humanMessage(e))
		if argErr != nil {
			usageCmd := argErr.Cmd
			if usageCmd == nil {
				// The error names no command, so the command Run was given
				// describes itself instead.
				usageCmd = cmd
			}
			usageCmd.Usage(stderr)
		}
	}
	return ExitCode(err)
}

// reportConfigError reports a command tree that failed to compile and
// returns the exit code the program should terminate with.
//
// It is the one report that does not consult the tree, because the tree
// is what failed: a command's stream overrides are inherited along the
// parent links Compile checks, so nothing it says about where its output
// goes can be trusted, and the message goes to os.Stderr whatever it
// configured. No usage follows, because a malformed tree cannot describe
// itself. Compile reports the same faults as an ordinary error value,
// which is where a program is best served catching them.
func reportConfigError(err error, stderr io.Writer) int {
	for _, e := range ir.FlattenErrors(err) {
		// A failure to write here has nowhere left to go: os.Stderr is
		// already where the fallback would write, and it must not cost
		// the exit code that says the program is malformed.
		fmt.Fprintf(stderr, "Program error: %s\n", humanMessage(e))
	}
	return ExitCode(err)
}

// NotifyContext returns a copy of parent that is canceled when the program
// is interrupted, on SIGINT or SIGTERM, and a stop function that releases
// the signal handler.
//
// Unlike signal.NotifyContext, a second interrupt terminates the program
// even if it is wedged: the first asks it to stop, and the second insists.
func NotifyContext(parent context.Context) (ctx context.Context, stop func()) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-ch:
		case <-ctx.Done():
		}
		// However this ended -- a signal, stop, or a canceled parent --
		// stop relaying signals, which restores their default disposition
		// so a second one kills a process that did not stop on the first.
		signal.Stop(ch)
		cancel()
	}()
	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}
