/*
Package climux implements command-line flag parsing and is a compatible alternative to Go's flag
package. This package provides higher-order features such as subcommands, positional arguments,
required arguments, validation, support for environment variables and others.

Package climux aims to make composing large, full-featured command line tools as simple and clean as
possible. Chained setters are employed to configure commands and flags declaratively.

For compatibility, a flag.FlagSet may be imported with FromFlagSet.

# Usage

Every climux program must define a top-level command using climux.NewCommand:

	import (
		"context"
		"os"

		"go.hotsrc.dev/climux"
	)

	var App = climux.NewCommand(os.Args[0], "My application")

	func main() {
		ctx, stop := climux.NotifyContext(context.Background())
		defer stop()
		os.Exit(climux.Run(ctx, App))
	}

You can import all global flags defined using Go's flag library with FromFlagSet.

	var App = climux.NewCommand(os.Args[0], "").
		FlagGroups(climux.FromFlagSet("go", "Options", flag.CommandLine))

You can bind a flag to a variable using the Var functions.

	var flagvar int

	var App = climux.NewCommand(os.Args[0], "").
		Flags(
			climux.Int(
				&flagvar, "flagname", 1234, "help message for flagname",
			),
		)

Or you can bind a variable of any type by writing a Decoder for it and
coupling the two with Var:

	climux.Var(&flagVal, "name", "help message for flagname", myDecoder{})

For such flags, the default value is just the initial value of the variable.

A handler may be defined for your command by

	var App = climux.NewCommand(os.Args[0], "").HandleFunc(MyAppHandler)

	func MyAppHandler(ctx context.Context, inv *climux.Invocation) error {
		return nil
	}

The handler is given the context passed to Run, which NotifyContext cancels on
SIGINT or SIGTERM, and an Invocation describing how it was called: the command
that was named, the path it was reached by, any arguments after the "--"
terminator, and the streams to work with. A command is often mounted in a tree
its author does not own, so the path is something only the invocation can tell
it.

A handler should read and write the streams on its invocation rather than the
process streams. They are the process streams unless the Run call answering
replaced them, so a test or an embedding program captures what a command
writes without the command knowing; see WithStdout.

	func MyAppHandler(ctx context.Context, inv *climux.Invocation) error {
		fmt.Fprintln(inv.Stdout, "Hello, World!")
		return nil
	}

Option parsing stops at "--". Every argument after it is an operand, so a command can be given an
operand that looks like an option. A positional marked EndOfOptions stops it the same way once it has
taken its token, for a command that hands its remaining arguments on to another program.

You can define subcommands by

	var (
		FooCommand = climux.NewCommand("foo", "Foo command")
		BarCommand = climux.NewCommand("bar", "Bar command")

		App = climux.NewCommand(os.Args[0], "Foo bar program").
			Subcommands(FooCommand, BarCommand)
	)

After all flags are defined, call

	climux.Run(ctx, App)

to parse the command line into the defined flags and call the handler associated with the command or
any if its subcommands if specified in os.Args.

Flags may then be used directly.

	fmt.Println("ip has value ", ip)
	fmt.Println("flagvar has value ", flagvar)

# Where a value came from

A variable a flag is bound to says what the flag holds, never how it came to
hold it: a flag nobody named holds the default its constructor gave it, and a
flag named with that same value looks identical afterwards. Where the answer
matters -- a flag that means something different for being given at all, or a
report that says which setting the operator actually chose -- ask the
invocation.

	func MyAppHandler(ctx context.Context, inv *climux.Invocation) error {
		if inv.IsSet("output") {
			// The operator chose a format; honor it as given.
		}
		fmt.Fprintln(inv.Stdout, "output came from", inv.Source("output"))
		return nil
	}

Source reports one of three answers -- SourceArgs, SourceEnv or SourceDefault
-- in the precedence the parser applies them: the command line wins over a
flag's environment variable, which wins over its default. IsSet is the
yes-or-no form, true for either of the first two. Both take a flag's declared
name, undecorated: "output" rather than "--output".

A program that keeps its declarations in variables asks them instead, which
costs no name at all:

	var (
		outputFlag   = climux.String(&output, "output", "", "Output format")
		templateFlag = climux.String(&template, "template", "", "Go template")
	)

	func MyAppHandler(ctx context.Context, inv *climux.Invocation) error {
		// --template picks the format, but only if -o did not.
		if template != "" && !outputFlag.IsSetIn(inv) {
			output = "go-template"
		}
		return nil
	}

Flag.IsSetIn and Flag.SourceIn answer the same two questions as the invocation's
own, and answer them better: a name is unique only along one command path, so
two sibling commands may both declare "force" and the name form reports
whichever is in scope. A declaration answers for itself or for nothing, and
Flag.InScope is what says which.

A source belongs to one reading of one command line rather than to the flag
itself, so it is the invocation that carries it and nothing about the flag
changes. See Invocation.Sources for the whole record.

# Help and version

Command.HelpFlag adds the flag that prints a command's help message,
answering to --help and -h. A program that reports a version adds one or
both spellings of it from the one string a build stamps into a constant:

	var App = climux.NewCommand("orbital", "Operate the fleet").
		HelpFlag().              // --help, -h
		VersionFlag(version).    // --version
		VersionCommand(version)  // orbital version

Add the flags to the root. The help flag is persistent, so every command
below answers to it too, each printing its own help; --version answers on
the root alone. They excuse a missing required argument, so they answer
a half-typed command line as well, but the rest of the line is still read
and checked: "app --bogus --help" reports the typo.

Declaring them first is a convention rather than a rule: it puts them at
the head of the options a command lists, which is where argparse puts
them and roughly where every other convention does. A program that wants
them elsewhere -- last, hidden, or under a heading of its own -- builds
them with HelpFlag, VersionFlag and VersionCommand and puts them where it
likes.

All three flags are interrupts, which is the whole of what makes --help
special: they run in place of the command that was named, without its
middleware, and answer even when the line leaves out an argument it
requires. Make one of your own with Flag.Interrupt, often on a flag from
Unbound, which binds no value.

# Middleware

Command.Middleware wraps a command's handler, and every handler beneath
it, in one function of your own:

	var App = climux.NewCommand(os.Args[0], "My application").
		Middleware(Authorize, Trace).
		Subcommands(GetCommand, DeleteCommand)

	func Authorize(next climux.HandlerFunc) climux.HandlerFunc {
		return func(ctx context.Context, inv *climux.Invocation) error {
			if !allowed(inv.Cmd.FullName) {
				return climux.Exitf(climux.ExitCodeUsage, "not authorized")
			}
			return next(ctx, inv)
		}
	}

This is for the work every command in a subtree has to do -- an
authorization check, a timing trace, opening a resource and closing it
again -- written once rather than at the top of every handler. Middleware
is inherited down the command path, so one declared on the root wraps
every command in the program, and the outermost wrapper is the one
declared highest in the tree.

A wrapper decides whether to call the handler it wrapped, so returning an
error without calling it refuses the invocation, and Run maps that error
to an exit code as it would the handler's own. It runs only around a
handler, and only after the command line has parsed: neither an
interrupt such as --help, an unparsable command line, nor a command that
exists only to group subcommands reaches one.

Middleware cannot change what a handler is given, since both sides of it
are a HandlerFunc.

A wrapper must be a pure function of the handler it is given, doing its
work in the handler it returns rather than in the wrapper itself. The
wrapping happens when the command tree is compiled, which happens more
than once in a run, so a wrapper that registers a metric or opens a
connection before returning does so more often than its author expects.

# Registries

A Registry is what a library contributes to a command tree, and what a
program mounts in one line. It carries flag groups, middleware and
subcommands, so a team ships a flag together with the wrapper that
honors it and neither can be mounted without the other:

	package timeouts

	var settings = &Settings{}

	func init() {
		climux.DefaultRegistry.
			FlagGroups(settings.FlagGroup()). // --timeout
			Middleware(settings.Wrap)         // honors it
	}

The program links the package in and mounts what it registered, naming
none of it:

	var App = climux.NewCommand(os.Args[0], "My application").
		Mount(climux.DefaultRegistry).
		Subcommands(GetCommand, DeleteCommand)

DefaultRegistry is the well-known one, for the packages that make up a
single program. Nothing limits a program to it: an organization that
keeps a registry per platform team mounts on each command the ones that
command should carry, and a registry mounted on a subcommand reaches that
subtree alone. A library published for programs it does not own should
export a registry rather than register into DefaultRegistry, so that
linking a package in is not by itself a decision about a program's
command line.

A registry is read when a command that mounts it runs, not when a
contribution is registered, so registration order never matters and
anything registered during package initialization is always seen. What
it contributes takes its place after what the mounting command declared
-- flag groups after its own, subcommands after its own children --
except middleware, which wraps outside what that command declared, since
a wrapper registered beside the flag it reads has to bound the handlers
the mounting program wrote.

A registry is not a node in the command tree. It holds contributions and
claims nothing, so a command registered as a subcommand must not already
be mounted in a tree of its own, and the command that mounts the registry
is its parent in the compiled tree alone. That is what lets two programs,
or two tests, mount the same registry without either writing to it.

# Exit codes

Run returns the exit code the program should terminate with:

	0  the handler returned nil, or an interrupt such as --help ran
	1  the handler returned an error
	2  the command line or the command tree was wrong, or there is no handler

A handler names its own exit code by returning an error that implements
ExitCoder. Exit and Exitf attach a code to an error — a handler reporting a
misuse the parser cannot detect itself, such as two mutually exclusive
flags, returns Exitf(ExitCodeUsage, ...) — and *exec.ExitError already
implements ExitCoder, so the error from a child process can be returned
unchanged to exit with its code.

# Command line flag syntax

In addition to positional arguments, the following forms are permitted:

	-f
	-fx
	-f=x
	-f x // non-boolean flags only
	-abc // equivalent to -a -b -c, for boolean -a and -b
	--flag
	--flag=x
	--flag x   // non-boolean flags only
	--no-flag  // boolean flags only

Short flags group into one argument while each takes no value. The first
that takes one takes the rest of the argument as its value, so -abfx is
-a -b -f x when -a and -b are boolean. An "=" is always a delimiter rather
than a flag name, so a boolean is set false as -f=false or --flag=false.

Every boolean also answers to --no-flag, which sets it false, for each of
its long names. This needs no declaring and cannot be switched off: it is
a second spelling of --flag=false rather than a feature a flag opts into.
The value negates with the flag, so --no-flag=false sets true. Short names
have no negated spelling, since -f=false is already the short way to say
it, and help does not list the negated spellings, since every boolean has
one. A flag built with Unbound, such as --help, binds no value and so
has none of these forms: it is given by name and nothing else.

The detached forms are not permitted for boolean flags because the meaning
of the command

	cmd -x *

where * is a Unix shell wildcard, would change if there were a file called
0, false, and so on.

A flag is valid from its own command's name until the command line names
a subcommand, and unknown after that. One marked Persistent stays valid
beneath its command and means the same thing there, which is what a flag
every command honors, such as --help, wants:

	git remote --verbose add   // either kind
	git remote add --verbose   // persistent only

An attached value is taken literally, so it may look like a flag: --flag=-5
is negative five, where --flag -5 is a missing value. See
docs/adr/posix-argument-conventions.md for the dialect in full, and for the
two places it departs from getopt.

# Shell completion

Command.EnableCompletion opts a command into shell completion:

	var App = climux.NewCommand(os.Args[0], "My application").
		EnableCompletion()

Once enabled, Run checks one environment variable before
doing anything else -- the command's name, uppercased, with every
non-alphanumeric rune mapped to "_", and "_COMPLETE" appended, so "myapp"
answers to MYAPP_COMPLETE. A recognized value there makes Run print a
completion script or a completion reply and return, without invoking any
handler; any other value, including the variable being unset, leaves Run's
behavior exactly as if EnableCompletion had not been called.

A user enables completion in their shell with a one-liner naming that
variable:

	source <(MYAPP_COMPLETE=bash_source myapp 2>/dev/null)
	source <(MYAPP_COMPLETE=zsh_source myapp 2>/dev/null)

That prints a small script, generated for the shell asked for, which
re-invokes the binary as the user types to ask what completes the word
under the cursor. Flags declare what completes their value with Choices,
for a fixed list, or Flag.Complete, for a callback computing candidates
from the Invocation parsed so far -- what completes one flag's value often
depends on another already given, as the ref argument to `git checkout`
depends on which repository is checked out.

Complete is the engine behind the reply, and answers the same
question programmatically: given the command line so far and the word
being completed, which candidates apply. It is exported so it can be
tested and driven directly, without a shell in the loop.
*/
package climux
