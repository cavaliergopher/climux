# Interrupt flags

Status: accepted, 2026-08-30.

## Context

`--help` was not a flag. The lexer matched `-h` and `--help` as string
literals before consulting the option table, emitted an instruction kind of
their own, and `argv.ValidateNames` reserved both names so nothing could
declare them. `ir.Invocation` carried a `HelpRequested` bool no other flag
had, completion appended `--help` to its candidate list by hand, and the
help formatter never mentioned it -- the one flag every command accepted was
the one flag help did not list.

The cost was not the duplication. It was that the flag was invisible to
anything reading the model: a machine-readable description of a program's
command line surface omitted `--help` entirely, a second argv dialect would
have had to reimplement the mechanism rather than only the spelling, and a
program could neither rename `-h` for something of its own nor drop it.

What actually made `--help` special was never its name. It was that
`app --help` has to answer someone who does not yet know what the command
requires, so a required argument they left out cannot stop it. Nothing
about that is peculiar to help: `--version` wants it too, and so does any
flag that reports on the program rather than operating it.

## Decision

A flag with an `ir.Flag.Handler` is an **interrupt**. Naming it does three
things and no more:

- it **excuses a missing required argument**, anywhere on the line;
- it **runs its handler in place of** the handler of the command it was
  given on;
- it **runs no middleware**.

The rest of the line is read and checked as usual, so
`app --version --format=json` binds its format and `app --bogus --help`
reports the typo. `ir.Invocation.Interrupt` names the first interrupt the
line gave, and replaces `HelpRequested`. A command built with
`InterruptCommand` answers the same way once the line reaches it, and may
declare flags and arguments of its own: `app help --format=json`. It may
not declare subcommands, which would answer in its place without its
properties; a help topic is an argument its handler reads,
`app help TOPIC`.

The first design ended the parse at the interrupt, discarded the errors it
outran, and handed the rest of the line to the handler unread. It was
dropped because it bought nothing help needs and cost two things users do:
a later flag the interrupt should honour never bound, and a real mistake
beside `--help` was silently excused. Excusing only the missing argument
is the whole of what `--help` requires.

`--help` is one interrupt among others, and nothing about it is privileged.
It is lexed through the option table, validated by the ordinary collision
check, offered by completion because it is in the table, and listed in help
like any other flag. Nothing mounts it: the program does, which is the only
dependency left between help and the command tree.

That dependency is a builder method rather than a default, so the tree is
what a program asked for and nothing else:

    NewCommand("orbital", "").
        HelpFlag().              // --help, -h
        VersionFlag(version).    // --version
        VersionCommand(version)  // orbital version

Declaring them first is a convention and nothing enforces it. It heads the
list of options, which is what argparse does with the same structure -- its
`options:` group is where ungrouped arguments go, exactly like the implicit
group here, and `-h, --help` is its first entry. The alternative worth
knowing is clap's, which puts them last in the last group: `uv pip install`
ends its eighth heading, `Global options`, with `-h, --help`. That works
because the group has real content, which a heading holding only these two
would not. Either shape is a program's to build, since the constructors are
exported and a flag goes wherever a program puts it.

Adding the help flag to the root is enough for the whole tree, because
`HelpFlag` declares a persistent flag: the root's reaches every command
under it and answers for whichever one was named. A command below may not
add its own, since the root's is already writable there; see
`docs/adr/flags-are-local-by-default.md`.

Any flag may interrupt, through `Flag.Interrupt`, and still binds its
value, so the author chooses what a help flag means by its type. A flag
built with `Unbound` takes no value, so `app --help sub` is help for
`app`: it names no other command. A `String` flag that interrupts takes
the next word as its value, so `app --help sub` hands `sub` to a handler
that dispatches on it. The parser does not guess between them.

A flag bound to no value has no default to restore and no negated
spelling, and an attached value -- `--help=false` -- is a malformed token
rather than something to set. `Unbound` builds one, and an effect is
chained onto it -- `Unbound("help", usage).Interrupt(printHelp)`,
`Unbound("end-of-options", usage).EndOfOptions()` -- rather than each
effect having a constructor of its own, which would put every effect in
the API twice. With nothing chained it is a flag a handler asks about.

An interrupt runs no middleware. It answers and takes no other action: a
program whose middleware redirects output to a file writes no file for
`--help`, and someone who wants the help message in a file redirects the
shell. This is a guarantee, not a consequence of middleware happening to
wrap handlers -- an interrupt that ran its ancestors' wrappers could be
refused by an authorization check before it answered.

The version builders take the string the program supplies, since the
library has none to report, and print it beside the root command's name.

## Consequences

- A program that adds no help flag has no `--help`, where before the parser
  answered one unconditionally. That is the cost of the tree being what was
  asked for, and `Command.HelpFlag` is one call.
- The help flag is listed in help output, and its command's usage line
  gains `[OPTIONS]`, because both are now true. Whether it is *shown* is
  the author's, through `Flags(HelpFlag(...).Hidden())`; hiding also drops
  it from completion, as it does for any hidden flag.
- A command may not declare `-h` or `--help` alongside the help flag: the
  ordinary collision check reports it, naming both ends. Without the help
  flag the names are simply free.
- `VersionCommand` and `VersionFlag` answer alike: neither needs an
  ancestor's `Required()` flag.
- An interrupt answers a wrong command line with the error, not with its
  own output. The user fixes the line and asks again.
- A description marshaled from the compiled tree carries the interrupt's
  options, its usage and whether it takes a value, but not that it
  interrupts: `Handler` is behavior, tagged `json:"-"`. If a consumer ever
  needs that fact, it wants a separate marshaled field rather than an
  exported func.
