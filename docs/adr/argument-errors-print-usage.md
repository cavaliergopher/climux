# Argument errors print the command's usage

Status: accepted, 2026-08-26.

## Context

A user who mistyped a command line got one line back — `Argument error:
unrecognized option: --nope` — and exit code 2. The line said what was
wrong but not what would have been right; for that, the user had to run
the command again with `--help`. The one exception was a command invoked
with no handler, which exists only to group its subcommands: it printed
its usage on stderr with no error line at all, so that reader saw what to
type but was never told what was wrong with what they typed. Neither
report was complete, and they disagreed about which half to keep.

Splitting dispatch from error reporting forced the question.
`Run` is now `Dispatch` — resolve the path, parse, call
the handler, return the raw error — plus error reporting, and anything
`Dispatch` cannot do, because it must not print error text, has to become
a returned error. That turned the no-handler case from a usage dump
inside dispatch into an error like any other, and left one place,
`Run`'s reporting, to decide what an argument error prints.

Prior art favors both lines, diagnostic first. Go's `flag` package
prints the parse error and then the defaults. getopt-based tools print
the diagnostic and then a usage line. Git does the same for an unknown
option.

## Decision

Every `*ArgumentError` that `Run` reports is followed by the usage of
the command the error names, on the same stderr stream. The error line
comes first: the reader's first question is what went wrong, the usage
message is the reference for fixing it, and getopt, `flag` and git all
order it this way.

The command is found on the error's `Cmd` field, resolved with
`errors.As`, falling back to the command `Run` was called on when the
error carries none — a handler-constructed `*ArgumentError` need not
name one. Both lines go to that command's resolved stderr, so
redirecting a command redirects its whole report.

A command invoked with no handler returns an ordinary `*ArgumentError`
reading `missing subcommand`, not a bare usage dump and not a dedicated
error type. Its report gains the error line it never had, and loses
nothing: the usage still follows.

Two error classes deliberately do not print usage:

- `ConfigError`. The command tree is malformed, so no description of it
  can be trusted — a tree that fails to compile has no compiled form to
  render a help message from. The report stays one line.
- A handler's error. The command line was right: the parser accepted it
  and the handler ran, so usage has nothing to correct. A handler that
  discovers an argument problem of its own and wants the full report
  returns one from `NewArgumentErrorf`, which is exported precisely so it
  can; see `docs/adr/human-readable-errors.md`.

Help is not error reporting and is untouched: `-h` and `--help` print
usage on the command's stdout and exit 0, and a caller who wants that
output formatted differently has `UsageFunc`.

Exit codes and prefixes are untouched. The three-code contract in
`docs/adr/exit-code-contract.md` and the `Argument error:` /
`Program error:` / `Error:` prefixes in
`docs/adr/human-readable-errors.md` stand as decided; only what follows
the error line is settled here.

## Consequences

- Every bad command line shows usage, so misuse costs more output.
  A program that wants the terse behavior back reports errors itself:
  `Dispatch` returns the raw error and prints nothing.
- The error line stays the first line on stderr, which is what scripts
  and tests that read only the head of the stream already observe.
- A long usage message can scroll the error line away on a terminal.
  Printing usage first would keep the diagnostic nearest the prompt, but
  reads backwards and contradicts the prior art; rejected.
- The no-handler report is two lines: anything scraping that output sees
  `Argument error: missing subcommand` before the usage message.
- The shape of the report is wording, not API, with the same caveat
  `docs/adr/human-readable-errors.md` attaches to the prefixes: a caller
  must branch on `errors.As` and the exit code, never on stderr's
  layout.
- A handler-constructed `*ArgumentError` gets the same treatment, usage
  included, which is the reason to construct one rather than return
  `Exitf(ExitCodeUsage, ...)`.
