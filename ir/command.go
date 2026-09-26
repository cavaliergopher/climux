package ir

import (
	"context"
	"io"
	"slices"

	"go.hotsrc.dev/climux/desc"
)

// An Invocation is the result of parsing a command line. It records which
// command the arguments named, what was left for its handler, and the
// streams the handler should read and write.
//
// Where the invoked command sits in the program is Cmd's to answer:
// Cmd.FullName names it from the root down, and Cmd.Ancestry is the
// commands themselves.
type Invocation struct {
	// Cmd is the command the arguments named.
	Cmd *Command

	// Interrupt is the first interrupt flag the command line gave, or nil
	// if it gave none. Cmd is then the command it was given on, whose
	// handler it runs in place of -- the one whose help is printed when
	// the flag is the one asking for it. See Flag.Handler.
	Interrupt *Flag

	// Stdin, Stdout and Stderr are the streams the handler should use in
	// place of the process streams, so that a caller redirecting a
	// command captures its output. They are the process streams unless
	// the Run call answering replaced them; see climux.WithStdout.
	//
	// They are never nil.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Source names where the value a flag holds came from, which is what
// tells a value an operator typed from one the program supplied: a
// command line that never mentions a flag leaves it holding its default,
// and nothing about the value itself says so afterwards.
//
// A source belongs to one reading of one command line, so it is recorded
// in the FlagState the declaration owns and the compiled flag points at,
// never on the Flag itself.
type Source int

const (
	// SourceDefault is a flag that neither the command line nor the
	// environment set, which holds its default. It is the zero value, so
	// a flag nothing has read yet reports it.
	SourceDefault Source = iota

	// SourceEnv is a flag set from the environment variable it declared.
	// That happens only where the command line did not set it; see
	// climux.FlagBuilder.Env.
	SourceEnv

	// SourceArgs is a flag the command line set, which is the source
	// that wins over the environment.
	SourceArgs
)

// String returns the source as a single lowercase word -- "default",
// "env" or "args" -- so that a message reporting where a value came from
// reads as a sentence. A Source outside the three reads as "unknown".
func (s Source) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceEnv:
		return "env"
	case SourceArgs:
		return "args"
	}
	return "unknown"
}

// A HandlerFunc handles the invocation of a command specified by command
// line arguments.
//
// ctx is the context given to climux.Dispatch, so a handler
// that does anything cancelable should honor it.
//
// inv describes the invocation: the command that was named, the path it was
// reached by, and the streams to work with. A handler should read inv.Stdin and write
// inv.Stdout and inv.Stderr rather than the process streams, so that a
// caller that redirects the command captures its output too. Nothing
// enforces it; a handler that reaches for os.Stdout simply escapes the
// redirection.
//
// Returning nil exits with code 0 and returning an error exits with code 1,
// unless the error implements ExitCoder, in which case it names its own
// code.
type HandlerFunc func(ctx context.Context, inv *Invocation) error

// Command is the compiled, implementation form of a command that users may
// invoke from the command line, produced by lowering a configuration tree
// with (*climux.Command).Compile.
//
// Every field is exported, including Ancestry and Root, which would make
// an encoded tree self-referential, and Handler, UsageFunc and the three
// streams, which carry behavior: ir is never encoded, so nothing has to
// be hidden from an encoder to keep any of it out of a document. See the
// package doc for the three-type model this is the middle of.
type Command struct {
	Name        string
	Summary     string
	Description string
	Hidden      bool

	// Interrupts reports that the command answers the way an interrupt
	// flag does: a required argument left out does not stop it running,
	// and no middleware wraps it. Every other rule of the command line
	// still holds. What it runs is Handler, like any other command. See
	// climux.InterruptCommand.
	Interrupts bool

	// FullName is the command's name joined with each ancestor's, from the
	// root down, so a deep subcommand reads as "app remote add" rather
	// than the bare "add" that String returns. Compile computes it top
	// down while lowering.
	FullName string

	FlagGroups  []*FlagGroup
	Subcommands []*Command

	// Ancestry is every command from the root of the tree down to and
	// including this one, which is the commands whose flags this command's
	// handler may read. Which of them the command line may write here is
	// narrower; see ScopedFlags.
	//
	// Compile builds it top down while lowering, so nothing reading a
	// compiled tree has to walk back up to reconstruct it. It is
	// derivable from the tree's shape; the command that mounted this one
	// is Ancestry's second to last entry.
	Ancestry []*Command

	// Root is the command at the top of the tree this command belongs to,
	// and is the command itself at the root. Whole-tree work -- validation,
	// and restoring defaults before a parse -- starts here, so that calling
	// Parse on a subcommand still governs the tree it belongs to, and is
	// what requires it to be set. Like Ancestry, it is derivable from the
	// tree's shape.
	Root *Command

	// Handler runs the command once its command line parses
	// successfully, and is never nil: it is the whole of what a command
	// does, assembled while lowering. Whatever the command declared
	// arrives here already wrapped in the wrappers its program put around
	// it, and a command that declared no handler of its own gets one
	// reporting a usage error, since such a command exists only to group
	// its subcommands. Calling it is how a command is run, whether or
	// not the command interrupts; see Interrupts.
	Handler HandlerFunc

	// UsageFunc renders this command's help message in place of the
	// default, and is inherited from the nearest ancestor that set one.
	// Compile resolves it while lowering, so a command carries the
	// renderer it will actually be printed with rather than one the Usage
	// method has to go looking for; it is nil only when no command on the
	// path set one, and Usage falls back to the default renderer.
	UsageFunc UsageFunc
}

// ScopedFlags returns the flags the command line may write once it has
// reached c: every persistent flag of c's ancestors, from the root down,
// then c's own flags, positional arguments included. An ancestor's other
// flags were valid only until the line dispatched below it. See
// docs/adr/flags-are-local-by-default.md.
//
// A flag group mounted from a Registry at two depths of one path lowers
// to a flag at each, sharing a State. Only the deepest is returned, since
// it is the one the command line reaches here.
func (c *Command) ScopedFlags() []*Flag {
	var flags []*Flag
	for _, cmd := range c.Ancestry {
		for _, group := range cmd.FlagGroups {
			for _, f := range group.Flags {
				if cmd == c || f.Persistent {
					flags = append(flags, f)
				}
			}
		}
	}
	// Walked from the deepest, so the flag kept for a shared State is the
	// last one the loop above appended.
	seen := make(map[*FlagState]bool)
	kept := flags[:0:0]
	for _, f := range slices.Backward(flags) {
		if f.State != nil && seen[f.State] {
			continue
		}
		seen[f.State] = true
		kept = append(kept, f)
	}
	slices.Reverse(kept)
	return kept
}

// String returns the command's own name, unqualified by its ancestry. See
// FullName for the full path from the root.
func (c *Command) String() string { return c.Name }

// Validate checks c and, recursively, each of its subcommands for
// configuration errors, reporting every error found in one run -- a
// malformed tree surfaces its errors in a batch, not one per run.
//
// Validation always covers the whole tree, from Root down, wherever in the
// tree it is called. It checks each command and flag on its own terms:
// whether two flags would answer to the same spelling is settled where
// spelling is, so a tree that passes here may still be rejected by
// (*climux.Command).Compile, which runs both. A Command produced by
// Compile is already validated.
func (c *Command) Validate() error {
	return validateTree(c.Root)
}

// Usage prints a help message for c to w, using c's UsageFunc, which
// Compile resolved from the nearest ancestor that set one, or the default
// renderer, Usage, when no command on the path did.
func (c *Command) Usage(w io.Writer) error {
	return writeUsage(c, w)
}

// Describe returns c's description, and, recursively, every subcommand
// beneath it. Behavior -- Handler, UsageFunc and the three streams --
// carries nothing to describe and is absent from the result.
//
// The document a program publishes should be rooted at c.Root. Describing
// a subtree is legal but understates what it accepts: an ancestor's
// persistent flags are valid beneath it, so a command's ancestors hold
// flags it accepts that are not beneath it here.
func (c *Command) Describe() *desc.Command {
	cmd := &desc.Command{
		Name:        c.Name,
		FullName:    c.FullName,
		Summary:     c.Summary,
		Description: c.Description,
		Hidden:      c.Hidden,
	}
	for _, group := range c.FlagGroups {
		cmd.FlagGroups = append(cmd.FlagGroups, group.Describe())
	}
	for _, sub := range c.Subcommands {
		cmd.Subcommands = append(cmd.Subcommands, sub.Describe())
	}
	return cmd
}
