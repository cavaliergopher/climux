package climux

import (
	"fmt"

	"go.hotsrc.dev/climux/ir"
)

// The exit codes Run terminates with. A handler may name any other code by
// returning an error that implements ExitCoder.
const (
	ExitCodeSuccess = ir.ExitCodeSuccess // A handler returned nil, help included.
	ExitCodeFailure = ir.ExitCodeFailure // A handler returned an error.
	ExitCodeUsage   = ir.ExitCodeUsage   // Nothing ran: the command line or tree was wrong, or no handler.
)

// ExitCoder is an error that names the exit code a program should terminate
// with. Run looks for one in the chain of errors returned by a handler,
// using errors.As, and exits with 1 if it finds none.
//
// *exec.ExitError implements ExitCoder, so a handler that shells out can
// return its error unchanged and exit with the child's code.
type ExitCoder = ir.ExitCoder

// ExitCode unwraps err until it finds an ExitCoder and returns its exit code.
// If none is found, it returns ExitCodeFailure.
func ExitCode(err error) int {
	return ir.ExitCode(err)
}

// Exit returns an error that reports err and asks Run to terminate the
// program with the given exit code.
//
// err may be nil to exit with a code and no explanation, in which case the
// error reads "exit status N", as an *exec.ExitError does.
func Exit(code int, err error) error {
	return ir.Exit(code, err)
}

// Exitf returns an error that reports the formatted error message and asks
// Run to terminate the program with the given exit code.
//
// Error wrapping is supported using the %w verb like fmt.Errorf.
func Exitf(code int, format string, a ...any) error {
	return ir.Exitf(code, format, a...)
}

// NewArgumentErrorf returns an error reporting that the command line was
// wrong, with its message formatted from format and a. Run reports it the
// way it reports a mistake the parser finds: "Argument error: ", the
// message, and the usage of cmd, exiting with ExitCodeUsage.
//
// Reach for it in a handler that finds a problem with its arguments the
// parser could not, such as an argument required only in some cases:
//
//	if plugin == "" {
//		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "", "missing subcommand or PLUGIN")
//	}
//
// cmd is usually inv.Cmd; if it is nil, Run prints the usage of the
// command it was given. flag and arg name the flag and the argument at
// fault, and may be nil and "". err, which may be nil, is wrapped and
// printed after the message.
func NewArgumentErrorf(err error, cmd *ir.Command, flag *ir.Flag, arg, format string, a ...any) *ir.ArgumentError {
	return ir.NewArgumentErrorf(err, cmd, flag, arg, format, a...)
}

// humanMessage prefers a String() method over Error(). The two differ by
// audience, not representation: on a ConfigError or ArgumentError from the
// ir package, String() is the plain sentence Run prints for a human, and
// Error() is that sentence tagged "climux: ", for a Go caller that prints
// or logs the error itself.
func humanMessage(err error) string {
	if s, ok := err.(fmt.Stringer); ok {
		return s.String()
	}
	return err.Error()
}
