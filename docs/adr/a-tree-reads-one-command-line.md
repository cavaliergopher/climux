# A tree reads one command line

Status: accepted, 2026-09-02. Revised 2026-09-26 by
docs/adr/a-flag-holds-its-own-value.md, which makes a second Parse against
one tree well-defined rather than unspecified: a reading begins by forgetting
the one before it, and writes each variable once. The rule below stands as
the shape of a program; what it no longer needs is a warning that reading
twice is undefined.

## Context

A command tree binds each flag to a variable the program owns, and
reading a command line writes to those variables. That is what the tree
is for: after a successful parse, the program's own variables hold what
the operator asked for.

Reading a second command line into the same tree would mean putting those
variables back the way they started. For simple values that is possible
-- capture what the constructor was given and assign it again -- but it
cannot be made general:

- A value that accumulates has no single prior state to return to. A
  repeatable flag appends, and a flag backed by a function calls that
  function once per occurrence, with whatever effect the program chose.
- A value that shares storage with others cannot be restored in
  isolation. Several bit-field flags toggle bits in one word, so
  restoring one means knowing which bits belong to it.
- A value the program supplies is opaque. `Set` may do anything, and the
  library cannot require a way to undo it from a type it did not
  construct.

So a restore pass can only cover the values the library builds itself. It
would hold for the typed constructors and quietly fail to hold for a
program's own -- a guarantee that is weakest exactly where a caller is
least equipped to notice, since the failure appears in their variable
rather than in a returned error.

Weighed against that, the case for reading twice is thin. A process
dispatches once: it reads `os.Args`, runs a handler, and exits. Nothing
in a command-line program's shape asks a tree to serve a second line.

The stdlib takes the same position. `flag` has no notion of parsing a set
of flags more than once and offers nothing to restore a value with:
`Flag.DefValue` is a string kept for the usage message. A second
`FlagSet.Parse` layers onto the first, leaving flags the new line never
mentioned at their old values and reporting through `Visit` and `NFlag`
the union of both lines.

## Decision

**A tree reads one command line.** `Parse` reads `args` against the tree
it is given. Calling it again with the same tree is not supported: what
the flags then hold is unspecified.

Three things follow, and each is a deliberate absence:

- **Nothing detects a second read.** No error, no panic, and no `Parsed`
  observable. An accessor answers no question a program that dispatches
  once can ask, and refusing the call requires state recording that a
  tree has been read -- which the compiled tree has no place for, since
  it models what a program means rather than what has been done to it,
  and which the configuration types have no place for either, as they
  build and validate configuration and carry no behavior.
- **Nothing is restored before a read.** A flag holds what its
  constructor gave it until a command line says otherwise.
- **A value need not be undoable.** There is no interface by which a
  value offers to reset itself, and none is required of a program's own.

## Consequences

A program that must read many command lines builds a tree for each one.
Tree construction is a few allocations and a `Compile`, so the cost is
small and the semantics are exact: independent trees write to whatever
variables their flags were bound to, with no shared residue.

The same applies to tests. A test covering several command lines builds a
tree per line; `Example_middleware` shows the shape.

Values are free to accumulate, because nothing has to be able to undo
them. A repeatable flag appends and a function-backed flag runs its
function per occurrence, and neither owes the library a way back.

The cost is that a program reading twice gets no signal. Its flags simply
hold something the library does not define, and the symptom surfaces in
the program's variables rather than as an error. This is accepted: it is
the same bargain the stdlib strikes, for the same reason, and the
alternative buys one message at the price of state that neither type
layer should carry.
