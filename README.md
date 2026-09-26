<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/logo/wordmark-ansi.svg">
  <img alt="climux" src="docs/logo/wordmark-ansi-light.svg" width="340">
</picture>

# A command line multiplexer for Go

[![Go Reference](https://pkg.go.dev/badge/go.hotsrc.dev/climux.svg)](https://pkg.go.dev/go.hotsrc.dev/climux)
[![CI](https://github.com/cavaliergopher/climux/actions/workflows/ci.yml/badge.svg)](https://github.com/cavaliergopher/climux/actions/workflows/ci.yml)

Many commands in, one dispatched out. Package climux implements command line
flag parsing and is a compatible alternative to Go's flag package, with the
higher-order features a real tool needs: subcommands, positional arguments,
required arguments, validation, environment variables and others.

Package climux aims to make composing large, full-featured command line tools as
simple and clean as possible. Chained setters are employed to configure
commands and flags declaratively. There are no dependencies beyond the standard
library, and no code generation or struct tags.

A command tree compiles into a described form the program can publish, so a
packager, a documentation generator or an agent can read what a binary accepts
in one call rather than crawling `--help`.

## Install

```
go get go.hotsrc.dev/climux
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"os"

	"go.hotsrc.dev/climux"
)

var flagName string

var App = climux.NewCommand("greet", "Print a greeting").
	Flags(
		climux.String("name", "Who to greet").Default("World").Bind(&flagName),
	).
	HandleFunc(func(ctx context.Context, inv *climux.Invocation) error {
		fmt.Fprintf(inv.Stdout, "Hello, %s!\n", flagName)
		return nil
	})

func main() {
	ctx, stop := climux.NotifyContext(context.Background())
	defer stop()
	os.Exit(climux.Run(ctx, App))
}
```

A flag holds its own value, read through the flag with no lookup by name, or
kept in a variable you own with `Bind`. Configuration errors — a duplicate flag, a positional argument
declared alongside subcommands — are reported when the command line is parsed.

A handler returns an error and `Run` turns it into an exit code: 0 for success
or `--help`, 1 for a handler that failed, 2 for a command line that was wrong.
An error may name its own code by implementing `ExitCoder`. The context comes
from `NotifyContext`, which cancels it on the first interrupt and restores
default signal handling so a second one kills a wedged process.

The `Invocation` tells the handler how it was called — which command ran, the
path it was reached by, and anything after a `--` terminator. A command is
usually mounted by whoever composes the binary rather than by the team that
wrote it, so its own path is not something it can know until it runs.

A flag also says where its value came from, which the value alone cannot: a
flag nobody named holds its default, and a flag named with that same value
looks identical afterwards. `State()` returns the half of a flag a handler
reads — `Value()`, `IsSet()`, and `Source()`, which is `SourceArgs`,
`SourceEnv` or `SourceDefault` in the precedence the parser applies them:

```go
var output = climux.String("output", "Output format").Default("table").State()

func Deploy(ctx context.Context, inv *climux.Invocation) error {
	format := output.Value()
	if template.Value() != "" && !output.IsSet() {
		format = "go-template" // --template picks the format nobody asked for
	}
	...
}
```

A state answers for its own declaration and no other, so two sibling commands
may both declare `--force` without either answering for the other.

`Command.Middleware` wraps a command's handler, and every handler beneath it,
in a function of your own — an authorization check, a timing trace, opening a
resource and closing it again — written once instead of at the top of every
handler. Middleware is inherited down the command path, the outermost wrapper
being the one declared highest in the tree, and a wrapper that returns without
calling the handler refuses the invocation.

```go
var App = climux.NewCommand("fleet", "Operate the fleet").
	Middleware(Authorize, Trace).
	Subcommands(RestartCommand, StatusCommand)

func Authorize(next climux.HandlerFunc) climux.HandlerFunc {
	return func(ctx context.Context, inv *climux.Invocation) error {
		if !allowed(inv.Cmd.FullName) {
			return climux.Exitf(climux.ExitCodeUsage, "not authorized")
		}
		return next(ctx, inv)
	}
}
```

A `Registry` is what a library contributes to a program that mounts it: flag
groups, middleware, and subcommands. A platform team registers a flag together
with the wrapper that honors it, so no binary can pick up one without the
other, and the program names none of it.

```go
package timeouts

func init() {
	climux.DefaultRegistry.
		FlagGroups(settings.FlagGroup()). // --timeout
		Middleware(settings.Wrap)         // honors it
}
```

```go
var App = climux.NewCommand("fleet", "Operate the fleet").
	Mount(climux.DefaultRegistry).
	Subcommands(RestartCommand, StatusCommand)
```

`DefaultRegistry` is the well-known one, for the packages making up a single
program; nothing limits a program to it, and an organization keeping a registry
per platform team mounts on each command the ones that command should carry. A
library published for programs it does not own should export a registry instead
of registering into `DefaultRegistry`, so linking a package in is not by itself
a decision about a program's command line. A registry is read when a command
that mounts it runs, so registration order never matters, and it is not a node
in the tree — mounting one writes nothing back, so two programs may mount the
same registry.

`Command.HelpFlag` adds the flag that prints a command's help message,
answering to `--help` and `-h`. A program that reports a version adds one or
both spellings of it, from the one string a build stamps into a constant:

```go
const version = "1.4.2"

var App = climux.NewCommand("orbital", "Operate the fleet").
	HelpFlag().              // --help, -h
	VersionFlag(version).    // --version
	VersionCommand(version)  // orbital version
```

Add them to the root. The help flag is persistent, so every command below
answers to it too, each printing its own help; `--version` answers on the
root alone. They excuse a missing required argument, so they answer a
half-typed command as well, but the rest of the line is still read and
checked: `app --bogus --help` reports the typo.

Declaring them first is a convention, not a rule — it heads the list of
options, which is where argparse puts them. The `HelpFlag`, `VersionFlag`
and `VersionCommand` constructors build the same things for a program that
wants them somewhere else: last, hidden, or under a heading of their own.

All three are *interrupts*: they run in place of the command that was
named, without its middleware, and answer even when the line leaves out an
argument the command requires. The rest of the line is read as usual, so
`app --version --format=json` still binds its format. That is the whole of
what makes `--help` special, and `Flag.Interrupt` makes one of your own,
often on a flag from `climux.Unbound`, which binds no value. Nothing is
mounted that a program did not ask for.

## Command line syntax

The dialect is the POSIX Utility Syntax Guidelines plus GNU long options —
what `getopt_long` accepts — rather than Go's `flag` package, where one dash
and two mean the same thing.

```
-f            --flag           a flag taking no value
-f=false      --flag=false     a boolean set false
              --no-flag        a boolean set false, spelled the other way
-fx  -f=x     --flag=x         a value attached to its flag
-f x          --flag x         a value in the next argument
-abc                           -a -b -c, while each takes no value
-abfx                          -a -b -f x, where -f takes one
```

Two arguments are not flags at all. A bare `-` is an ordinary operand, left
to the handler to interpret, and `--` ends option processing: every
argument after it is an operand, however many dashes it starts with. A
positional marked `EndOfOptions` does the same once it has taken its
token, which is how a command hands its remaining arguments to another
program without its user typing a terminator.

An argument beginning with `-` is never taken as a detached value, so
`--count -5` is a missing value rather than negative five; write
`--count=-5`. Flags may appear among the operands in any order. A flag is
legal from the point its own command is named until the line names a
subcommand; mark it `Persistent` and it stays legal beneath its command,
meaning the same thing there.

Every boolean also answers to `--no-flag`, for each of its long names,
which sets it false. Nothing declares it and nothing can switch it off: it
is a second spelling of `--flag=false`, not a feature a flag opts into.
The value negates with the flag, so `--no-flag=false` sets true. Short
names get no negated spelling, and help does not list the negated ones,
since every boolean has one.

Five departures from `getopt_long` are deliberate, and
[the ADR](docs/adr/posix-argument-conventions.md) argues each one:

- **Attached values follow Go, not getopt.** `-n=value` sets `value` rather
  than `=value`, and a boolean accepts an attached value, so `--flag=false`
  and `-f=false` both set false. Without it a boolean could not be turned
  off at all.
- **Negated booleans are generated, not declared.** `getopt_long` leaves
  `--no-flag` to each program to declare; here every boolean has one, so
  a user never has to find out which of a program's booleans got one.
- **Long options may not be abbreviated.** `getopt_long` accepts any unique
  prefix, but a command tree makes "unique" a moving target: adding a flag
  to a subcommand can break a script that never changed.
- **A program may end options itself.** `getopt` ends option processing
  only at `--`. A positional marked `EndOfOptions` ends it once it has
  taken its token, so `docker run [OPTIONS] IMAGE [COMMAND] [ARG...]` needs
  no terminator from its user. POSIX has no subcommands, and so no case of
  one program handing its arguments to another.
- **`-h` and `--help` are mounted**, not reserved, which is GNU practice
  rather than POSIX. They are an ordinary flag on the root of every tree,
  so a program may rename or drop them with `Command.HelpFlag`, and a
  command declaring either name collides with them like any other pair.

## Compiling a command

`Command.Compile` lowers a command tree into the implementation types in the
[ir](https://pkg.go.dev/go.hotsrc.dev/climux/ir) package: every
command, flag group and flag, with ancestry resolved and exported fields
holding everything that marshals. The default help formatter walks it, and
so can your own tooling. Most programs never need to call Compile
themselves, or import ir at all -- it is what Parse, Run and the help
output use internally.

See [the docs](https://pkg.go.dev/go.hotsrc.dev/climux) for
comprehensive examples.

## License

climux is licensed under the [Apache License, Version 2.0](LICENSE)
(`SPDX-License-Identifier: Apache-2.0`).
