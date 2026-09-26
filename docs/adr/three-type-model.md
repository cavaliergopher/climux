# Three types per concept, and the ir package

Status: accepted, 2026-08-28. Revised 2026-09-23 by
docs/adr/a-flag-holds-its-own-value.md, which makes the configuration flag
generic and gives it a runtime half. The three layers and the package
boundary are unchanged.

## Context

Every concept in this package needs three shapes. A command as the program
author declares it is a builder: chained setters, defaults filled in,
half-built while the tree is assembled. A command as the parser sees it is
resolved: ancestry known, mounted flag groups flattened in, every name
settled. A command as a document is inert: plain data with explicit names,
carrying nothing a reader outside the process could not use.

The first two cannot be one type. The setters that make a builder usable
are the surface a parser must not reach, and a field that may or may not
have been resolved yet has no single correct reading. Why the third is a
type of its own rather than a stage in the middle takes more saying.

The tempting economy is to collapse the second into the third and parse
against the inert one, on the reasoning that an artifact is either inert
and portable or live and local and the package needs the portable one
anyway (see docs/adr/machine-readable-schema.md). The reasoning about
artifacts holds; using that artifact as a parsing stage does not.

Parsing splits into a lexing pass and an applier, and the lexer has to
resolve an argument to something the applier can act on. A projection that
drops all behavior -- no `Value`, no handler, no writers -- gives the lexer
nothing to resolve to, so the split grows a private parallel tree,
pairing each description with the source it was projected from, to put
back exactly what the projection took away. Roughly eighty lines and a
second tree walk, spent entirely on keeping one field off a struct.

What fails is a description as a *stage*, not as a type, and nothing about
field visibility changes it. Keeping behavior out of the output is the
easy half either way: Go does not marshal an unexported field, and a tag
excludes an exported one.

A description still earns its place, for requirements marshaling alone
does not raise -- keys that survive a Go field being renamed, a version,
and the ability to be read back -- but it belongs at the boundary, as a
conversion nothing internal reads. See
docs/adr/machine-readable-schema.md.

Keeping the behavior fields unexported carries its own price: no one
outside the package can set them, which costs a `FlagConfig`/`NewFlag` and
a `CommandConfig`/`NewCommand` to build a node, plus accessor methods --
`Path`, `Stdin`, `Stdout`, `Stderr` -- to read back what visibility
hides.
For a package with no users yet and no stability promise, that is a
permanent API surface bought for a marginal guarantee, and the guarantee
is free where it is actually wanted: nothing encodes an `ir` type, so
nothing on one has to be hidden from an encoder.

## Decision

Every concept has a **configuration** type, an **implementation** type
and a **description**. The configuration types stay in the root package.
The implementation types are public, and live in a new package, `ir`.
The descriptions are plain data, live in `desc`, and are specified in
docs/adr/machine-readable-schema.md; what follows settles the first two.

`ir` is the compiler's word for it, and the analogy is exact: the
configuration tree is the syntax the author writes, `Command.Compile`
is the lowering pass, and what comes out is an intermediate
representation that later passes -- validation, lexing, applying,
formatting, completion, marshaling -- all consume. The types keep their
plain names and take the qualifier for disambiguation, as
`ast.File` does against `types.Package`: `ir.Command` beside `Command`.

Those later passes do not all live in `ir`. The ones that read a command
line -- lexing, applying, completion, and the spelling of options they
share -- are in `internal/argv`, which imports `ir`; `ir` holds no
execution and cannot import back. Two compilations were sharing one
package, and the vocabulary said so: we `Compile()` a tree and then
`lex()` against it, where a compiler lexes first and produces an IR last.
`ir` is what the program means; `argv` is the machine that reads a
command line against it.

Every field on an `ir` type is exported, including the handful that carry
behavior -- `Command.Handler`, `.UsageFunc` and its three streams,
`Flag.Value` and `.ValidateFunc`. Nothing is hidden to keep it out of a
document, because an `ir` type is never encoded: what a program publishes
is a description, built by copying the fields that belong in one, so
behavior is absent from the output because nothing wrote it there.

`ir` is also the fully resolved form of a tree, not just the public one.
Compiling walks ancestry into `Ancestry`, renders a flag's default into a
string, and flattens a command's own flag groups with every mounted
group into one list -- and now, while lowering, computes each command's
full name and resolves its input and output streams too, so nothing about
a compiled node has to walk itself again afterward.

The bound tree is deleted. There is one compiled tree, and the lexer
reads it directly.

The package boundary follows one rule: **the root package is everything
you need to create a standard program.** A program that declares
commands and flags, writes handlers and calls `Run` never imports `ir`.
`Invocation` and `HandlerFunc` are therefore aliased into the root
package, since they appear in the signature of every handler. Everything
else an advanced consumer reaches for -- `ir.Command`, `ir.CompleteFunc`,
`ir.UsageFunc`, the error types --
is reached through the import, which keeps the root package's
documentation to the surface a standard program uses.

## Consequences

`Command.Describe` becomes `Command.Compile`, which lowers, validates and
returns the compiled node. `desc` stops being a stage on the way to
parsing; what it later becomes is the wire format, produced from a
compiled tree rather than standing between one and the parser.

Validation runs on the compiled tree rather than the configuration tree.
That is the compiler order -- lower, then check -- and it is what lets a
`ConfigError` carry the compiled node it concerns, which is the
"errors carry descriptions rather than sources" change arriving as a
consequence rather than as work.

The lexer can call `Set`, and must not. The compiled flag it reads carries
the value, so the rule is documented rather than made impossible by the
type -- a real if small loss, since completion evaluates a broken command
line and must apply nothing.

Help rendering belongs in `ir`. The argument for keeping a renderer out --
that a type earning its place by being data only invites every consumer
once it admits one -- does not reach `ir`, which is not data only.
Rendering reads the spellings `argv` put on the compiled flag; it does not
construct them.

Every `ir` field being public is what makes `internal/argv` possible: a
sibling package lexes and applies over the compiled tree with no accessors
added for it. Unexported fields would block that outright.

Two imports appear where one would otherwise do, for the advanced consumer
only.
That is the cost deliberately accepted for keeping the root package's
documentation legible.

The extra type per concept is the standing cost. It is the same cost the
`desc` package already charged; what changes is that the second type now
earns it, by being the thing the parser actually runs on. The third earns
it separately, by being the thing that leaves the process.
