# The description is a wire format with its own types

Status: accepted, 2026-08-30. Implemented 2026-08-31.

## Context

`ir.Command` marshals today, and that is not the same as having a schema.
The keys are Go field names, because nothing carries a `json` name, so
renaming a field is a wire break nobody signed up for. There is no version
marker, so a consumer holding a document cannot tell what it is holding.
Nothing is `omitempty`, so every node emits every zero field. And the
output cannot be read back: `Ancestry` and `Root` are excluded as
derivable, `Handler` is documented as never nil, and a decoded node would
have neither — it can be written but not parsed, which means it is a Go
value that happens to marshal rather than a format.

That gap matters because the compiled tree is meant to be a product rather
than an implementation detail. The consumers it exists for — a docs or man
page generator, static completion script generation, breaking-change
detection for a command line in CI, an agent reading a binary's surface in
one call instead of crawling `--help` — all read, and most of them are not
written in Go. Help rendering, the one consumer today, reads an `ir` tree
in process and hides every one of these problems.

## Decision

### Every concept has a third type

A concept has three types: a **configuration** type, which an author
builds with chained setters; an **implementation** type in `ir`, which is
parsed against and dispatched; and a **description** type, which is the
wire format. See `three-type-model.md`. `desc` holds `Document`,
`Command`, `FlagGroup` and `Flag`: plain data, no behavior, every field
carrying an explicit `json` name, and every one of them round-trippable.

Each implementation type lowers itself, as the configuration types
already do: `Command.Compile` produces the `ir` tree, and
`(*ir.Command).Describe`, `(*ir.FlagGroup).Describe` and
`(*ir.Flag).Describe` produce the description. One stage builds the next
the whole way down, and no name has to distinguish two things called
Command. `desc.NewDocument(*desc.Command) *desc.Document` wraps a
described root in the envelope, taking a description rather than a tree
so that the version stays owned by the package that defines it.

`Describe` describes the node it is called on and everything beneath it.
A half-populated description whose `subcommands` a caller has to fill in
would be a trap of the kind `ir` already refuses to set, and the whole
point of the document is that one call yields the surface. Full names
come along correctly wherever it is called, because `Compile` computed
them on every node.

The document a program publishes is nonetheless always rooted at
`Command.Root`. Describing a subtree is legal and says nothing false —
each node reports the `fullName` it really has — but it is lossy in a way
worth naming: a flag is in scope from the point its own command is named
onward, so a command's ancestors hold flags it accepts, and those
ancestors are not beneath it. A document rooted at a subcommand therefore
understates what that subcommand takes.

The repair this invites must not be taken. Folding every in-scope
ancestor flag into each command as it is described would make any subtree
honest, and it would repeat the root's flags at every level of the tree,
inflating the whole-binary document for the sake of the one case it is
not serving. What a single command accepts is a different projection and
belongs in one.

So `ir` imports `desc`, and `desc` imports nothing of ours. The arrow
points at the leaf deliberately. A consumer that only reads documents —
a tool diffing two of them, with no command tree anywhere in it — gets
the plain data and none of the implementation.

`desc` is a boundary, not a *stage*. Nothing internal reads a description:
the parser runs on `ir`, and the conversion happens once, on the way out.
A description used as a stage does not work, because a lexer reading a
projection that carries no `Value` cannot resolve an argument to anything
the applier can act on, and putting that back costs a private parallel
tree and a `bind()` pass — eighty lines spent keeping one field off a
struct. The package is not called `schema`, because the format is
published as a JSON Schema document and one word should not name both the
Go types and the thing that validates them.

Embedding the description into the implementation type is the other
candidate and does not work either: `encoding/json` flattens an anonymous
embedded struct, so
`ir.Command{desc.Command; Handler ...}` marshals to exactly the keys
`ir.Command` marshals on its own. It buys a type and not a format.

### `ir` does not marshal

Nothing in `ir` is encoded or decoded, and it carries no `json:"-"` tags.
The guarantee that behavior never leaks into a document is not a tag
discipline pinned by a test but a structural property:
`Describe` copies named fields, so a behavior field added to `ir` later is
absent from the output because nothing wrote it there, not because a tag
excluded it.

Protocol buffers were considered for the stability the schema needs and
are declined. Their guarantee is about field numbers on the wire, and JSON
output does not carry field numbers — a rename breaks a JSON consumer with
protojson exactly as it does with `encoding/json`, so the dependency buys
a schema language rather than a stability mechanism. It also costs the two
claims the package leads with, no dependencies beyond the standard library
and no code generation. What keeps a format stable is the same either way:
an explicit spec, a version, and goldens. Hand-written description types
fill the role a `.proto` would have.

The spec lives at docs/desc.schema.json and is served as
https://static.hotsrc.dev/climux/schema/v1.json, which is the `$id` a
document's consumer resolves. Serving it from a domain we control rather
than from a repository host keeps it addressable if the code moves, the
same reasoning that puts the module behind a vanity path.

### Documents are versioned and grow additively

The envelope carries the version, so the content does not have to. The
wire key stays `schemaVersion` whatever the Go package is called, since
that is what a consumer will look for:

```json
{
  "schemaVersion": 1,
  "command": {
    "name": "add",
    "fullName": "orbital remote add",
    "summary": "Add a remote",
    "flagGroups": [
      {
        "title": "Options",
        "flags": [
          {
            "name": "force",
            "options": [
              {"option": "--force"},
              {"option": "-f"},
              {"option": "--no-force", "effect": "negate"}
            ],
            "kind": "bool",
            "usage": "Overwrite an existing remote",
            "maxCount": 1
          }
        ]
      }
    ]
  }
}
```

Within a version the format only gains keys, and a consumer must ignore
keys it does not know. Anything else is a new version. This promise is
made for the description alone and not for the Go API: they break for
different reasons, and the point of the separate package is that an `ir`
field may be renamed without a document changing.

The promise is over parsed values rather than bytes. Whitespace and key
order are not part of the format, and a consumer comparing two documents
— which is what breaking-change detection in CI does — compares what it
decoded, never the text it received. Making the encoding contractual
would put a document's layout under the same additive-only promise as its
content, so reordering two fields in a `desc` struct, or an encoder
spelling the same value differently, would read as a break while saying
nothing about the program described. `SchemaCommand` indents its output
because a person runs it by hand too, and that is a courtesy rather than
a guarantee.

### A flag records the kind of value it takes

`ir.Flag` gains a `Kind`, because `Int(&n, ...)` and `String(&s, ...)`
compile to indistinguishable flags today and every consumer wants the
difference — to say what to pass, to know when not to offer filenames, to
notice `--port` becoming a string, to emit a parameter type. The kind is
known where the flag is constructed rather than where it is read, so
nothing needs to inspect a `Value` and `flag.Value` is untouched:

- Each typed constructor sets it.
- `FromFlagSet` recovers it through `flag.Getter`, whose `Get() any`
  returns the concrete value to switch on, so an imported `flag.FlagSet`
  is described as precisely as a native one.
- `Var` yields `KindOpaque`, unless the value declares itself by
  implementing an optional interface, in the manner `BoolValue` already
  establishes for `IsBoolFlag`.

The optional interface is the only way an author states a kind, and that
is deliberate. A configuration setter -- `Flag.Kind(...)` at the mount
site -- was considered and rejected: a custom value type is written once,
in a shared library, and mounted by teams that do not own it, so the type
must be fully formed in the one place that defines it rather than leaving
each mounter free to restate, and misstate, what its package already
knows. Writing a custom value is the advanced case, and its audience is
exactly the one comfortable implementing one more method.

For the same reason the callback constructor, `Func`, always describes as
`opaque`: it is the anonymous case, and anonymity is why it can claim no
kind. An author who knows the kind writes a small value type in the
library that owns the parsing.

The set is `bool`, `string`, `int`, `uint`, `float`, `duration` and
`opaque`, and is deliberately coarser than Go's types. A command line
accepts a decimal integer whether the variable behind it is an `int` or an
`int64`, and `ir` models what a program means rather than how it is built.
Adding a kind later is additive. The set is also closed to the program:
a kind is one of these words or it is not a kind, since a consumer
switching on it relies on the vocabulary being shared, the same reasoning
that keeps an option's effect a single opaque term rather than a
per-program vocabulary.

### A flag's name is undecorated, and identifies it

`ir.Flag` carries `NamedOptions` and `ClaimedOptions`, both already
written by `internal/argv`, and nothing else. A document needs an identity
that survives an alias being reordered, so `ir.Flag` gains the declared
canonical name and `desc` writes it as `name`.

It is not decorated by the dialect. The decorated forms are already in the
document, as `options`, so spelling `name` the same way would
put one string in it twice with no way to tell which is authoritative:
`name` answers which flag this is, and `options` answers what a user
types. A positional argument settles it. It is named by no option at all,
so a decorated `name` would be decorated for some flags and bare for
others, and a field has to mean one thing everywhere it appears.

Commands are identified by `fullName`, which they already have.

### An option says what typing it means

`ir` splits what a flag answers to in two: `NamedOptions`, the ones worth
showing a reader, and `ClaimedOptions`, every one the parser accepts,
each mapped to the declared option it was generated from. A document
carries one list instead, of every option with what naming the flag that
way means:

```json
"options": [
  {"option": "--force"},
  {"option": "-f"},
  {"option": "--no-force", "effect": "negate"}
]
```

Provenance comes out. That `--no-force` was generated from `--force` is
the parser's business — `generatedFrom` reports a collision from
whichever side generated it, and `resolvedOptionsInto` recovers negation
by running the generating rule forward against the source rather than
stripping the prefix back off. A consumer needs none of it, and is
actively misled by it: what it must know is that typing `--no-force`
sets the flag false, which the source alone does not say unless the
consumer already knows the convention.

That pair is not new. `internal/argv` already resolves a token to a flag
and what naming it that way means for the value, and says why in a
comment that is really this decision's argument: both are needed and
neither follows from the other, because a boolean answers to two options
that mean opposite things.

An effect is one string, not a list. It answers what typing this option
means for the value it binds, and one spelling has exactly one such
meaning — two would leave a consumer no rule for composing them, and the
parser, which resolves each token to a single meaning, could not honor
them either. A genuinely different question about an option, such as
whether it is deprecated, gets its own field rather than another entry
here, which is additive where widening this one to a list would not be.
Whether a command is read-only or idempotent is a question about the
command, and belongs there if it is answered at all.

Absent, the effect is the ordinary one: naming the flag this way sets it.
New effects may be added, so a consumer that meets one it does not
recognize must not offer that option, having no way to know what typing
it does. That rule ships with the first version or no effect added later
is additive.

The order is contractual, because flattening otherwise loses what
`NamedOptions` records by position: the canonical name, then the short
name, then any further aliases, then anything the dialect generated. It
costs nothing to promise and it keeps a formatter able to tell a short
name from an alias without counting dashes, which would be dialect
knowledge it is not supposed to need.

### Reading is closed off from execution

`desc` types unmarshal. `ir` types never do, and there is no function
converting a description back into an implementation tree. That is a
deliberate omission and not a gap to be filled later.

An `ir` tree exists to be parsed against and run, and both need a live
`Value` and a `Handler` that no document carries, so a decoded one would
satisfy the type without satisfying its contract. Keeping the two kinds of
tree distinct means a document — which arrives from elsewhere, out of a
file in CI or another team's release — can never be mistaken for a
program, and the compiler enforces it rather than a doc comment asking
nicely. It also leaves `Compile` the only way to obtain an `ir` tree, so
nothing reaches one that compiling has not vouched for, and the rule that
hand-built `ir` trees exist only in tests survives.

### A program mounts the description command itself

`climux.SchemaCommand() *Command` returns a command whose handler writes
the document for the whole binary to `inv.Stdout`. A program mounts it
where it wants:

```go
root := climux.NewCommand("orbital", "...").
    Subcommands(deploy.Command(client), climux.SchemaCommand())
```

This needs no new machinery, which is what `Invocation` carrying the
command it names was for. A handler receives `inv.Cmd.Root`, the compiled
root of whatever tree it was mounted in, so the whole of the handler is
to encode `desc.NewDocument(inv.Cmd.Root.Describe())` and the command
describes the binary it finds itself in without being wired to it.

It is a function returning a fresh command rather than an exported
variable, because a package-level `*Command` is mutable by anyone holding
it and a second mounting of the same value is already a configuration
error. Returning a command to `Subcommands` is the idiom packages
contributing a subcommand already use, which `examples/orbital`
demonstrates throughout. A builder shorthand exists beside it,
`Command.SchemaCommand`, mirroring `Command.VersionCommand` — a
convention that postdates this document's first draft, which had
rejected an `AddSchemaCommand`-style setter when there was no such
family for it to belong to.

The name is fixed rather than a parameter. The value of a convention is
that a packager or an agent knows where to look without being told.

## Consequences

The field lists in `ir` and `desc` largely duplicate each other. That is
the cost being paid for, and it is what a `.proto` and its generated
structs would have cost in a different currency: the duplication is the
decoupling, and without it a Go field rename is a break for every
consumer.

Nothing needs to change in how a phantom subcommand would be validated,
because there is no phantom. The description command is declared by the
program like any other, so a tree that mounts it on a command taking
operands is an ordinary configuration error naming the author's own
mistake. That question can stay open on its own merits rather than
blocking this.

`ir` gains two fields, neither of them behavior. A golden document for
`examples/orbital` is what will make an unintended wire change visible,
alongside the help goldens already there.

Machine-readable error output should project through the `desc` types
rather than defining a second vocabulary for a command and a flag, which
is what docs/adr/human-readable-errors.md leaves open for it.

Recording an option's effect is a change to `ir`, and the only one here
that is not additive. `ir.Flag`'s comment says today that what a
generated option does to the value it binds is the convention's own
business and is deliberately not recorded, on the reasoning that every
convention either generates options or does not while what generating
means is particular to each. That reasoning holds for the parser, which
knows its own convention, and fails for a document, whose reader does
not. So `ClaimedOptions` maps an option to both the source it was
generated from and the effect of naming it, and `internal/argv` fills in
the second where it already fills in the first.

`ir` still knows no dialect by this, and the distinction has to be exact,
because the narrower versions of the same field do not survive it. A
negated form on the compiled flag, or a `Negated` boolean on a claim, is a
dialect feature leaking into the type every dialect shares. The test is
whether a dialect lacking the feature leaves the field meaningless, and a
boolean fails it: a dialect without negation reports false everywhere, and
one whose modifier is a repeat or a reset needs a second field while the
first sits dead.

An effect names no feature. It is a term the dialect writes and a
consumer reads, over a vocabulary that is open, so a dialect with no
modifiers leaves it empty — which says there is no modifier, rather than
saying nothing — and a dialect with a repeat count writes that without
the shared type changing. Two rules keep it there:

- Nothing in `ir` or the root package may compare an effect against a
  value it knows. The moment anything branches on `negate`, it is an
  enum `ir` owns and the leak is real. An effect is written, copied and
  read out, never tested.
- The parser goes on deriving negation by running the generating rule
  forward against the source, as `resolvedOptionsInto` does today,
  rather than by reading the effect. The modifier still lives entirely
  inside the dialect, and the effect exists for the document alone.

Under those two rules an effect is not the field those narrower versions
would have been. What the type records is not that this dialect negates,
but that the dialect had something to say about an option, in a word it
and the reader share.

A flattened per-command projection is anticipated and deliberately not
built. Something calling one command wants that command's calling
convention — its own flags and every ancestor's, and none of its
descendants — rather than a document it must read the whole of.
`Ancestry` already holds the commands whose flags are in scope, so this
needs nothing new in `ir`, and it wants a type of its own rather than a
mode on `desc.Command`, so that a consumer can tell from what it holds
which of the two it has. A new shape is additive under the version
policy, and there is no consumer to design it against yet.

A document does not name the dialect it was written in. Every
reader-facing form in it is already written out, and a program has one
dialect for its lifetime. If alternative dialects arrive with matchers
that work by rule rather than by table — every unambiguous prefix of an
option, say — a generator reproducing that matching would have to know
which dialect wrote the document, and naming it then is an additive
change.

`HasDefault` does not appear in a document. It records that `Default` was
captured from a live value so that it can be re-applied on a repeat parse,
which is machinery for restoring state rather than something a command
line means.

A document is purely structural, and never reports runtime state. The
sharp case is a flag bound to an environment variable: the document says
the variable's name and never its resolved value, because an environment
variable may hold a secret, and a description that resolved it would
publish it to whatever asked. The same rule keeps a document stable
across environments -- it describes the program, not the process.

## References

Where a convention already exists for what a document should say, or for
how a program should be asked for one, this design follows it rather than
inventing a parallel. The alignment is deliberate and partial, and the
boundary is worth stating plainly: conformance is a property of a
program, not of the library it was built with. climux can supply the
description, the command that emits it and the streams it goes to; the
rest is the author's.

**The CLI Spec**, version 0.3 (a candidate, August 2026), Ruben
Jongejan, CC BY 4.0, at clispec.dev. It is one author's proposal rather
than a standard, and is followed here where it is right rather than for
conformance. It requires a `schema` subcommand that works "before
anything else does: no authentication, no configuration file, no
network", which `SchemaCommand` satisfies by construction, describing an
already-compiled tree and reaching nothing outside the process. Its rule
that data goes to stdout and diagnostics to stderr is the contract
`Invocation` already hands a handler. Two of its requirements belong to a
program rather than to us: a `--output` or `--format` flag selecting
JSON, which is a program's own output and not something climux owns; and
the per-command declarations `effects` — `read_only`, `idempotent`,
`non_idempotent` — and `cardinality`. Those are where a command-level
declaration would land if one is added, and the vocabulary to borrow when
it is. They are a different question from an option's effect, which says
what one spelling means for one value.

**The Algolia CLI**, which was rewritten around a `describe`/`schema`
command emitting a versioned JSON document of subcommands, flags and
usage. Precedent for the envelope carrying the version rather than the
content carrying it.

**Arcjet, "Designing a CLI for AI agents".** Its argument that commands,
flags and output fields become immutable contracts the moment something
automated depends on them is the argument for the additive-only policy
above. Its JSON errors carrying a code and a remediation belong to the
machine-readable error work rather than to this.

**cli/cli#12912**, proposing a CLI schema so that an agent's permissions
can be enforced against it. Evidence that a read-only declaration earns
its place, and that the reader of a document may be a policy rather than
a caller.

**`flag.Getter`** in the standard library, whose `Get() any` is what lets
an imported `flag.FlagSet` be described as precisely as a native one.
