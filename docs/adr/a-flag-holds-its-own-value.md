# A flag holds its own value

Status: accepted, 2026-09-23. Not yet implemented.

Revises docs/adr/a-tree-reads-one-command-line.md,
docs/adr/three-type-model.md and
docs/adr/configuration-types-carry-no-behavior.md; each says where.

## Context

A flag's value is read through one path and everything else about the flag
through another. A program that needs both holds three handles on one flag:

    var output string
    var outputFlag = climux.String(&output, "output", "json", "")

    if template != "" && !outputFlag.IsSetIn(inv) { ... }
    fmt.Println(inv.Source("output"))

The variable knows the value. The declaration knows the name, the default and
the usage. The invocation knows whether the flag was given and where its value
came from. None knows all three, so a handler that needs two of them says the
same flag three ways.

The third path is the worst of them. A name is unique only along one command
path, so two sibling commands may both declare `force` and a lookup by name
answers for whichever is in scope — the right answer to a question the caller
did not ask. Keying on the declaration instead requires linking a declaration
to the node it compiled to, which is machinery that exists only because the
declaration does not hold the answer itself.

Abseil takes the other position: a flag holds its value, and `absl::GetFlag`
reads it. Its flags are also unavoidably global, which is a separate decision
and not one worth copying. Kingpin, in Go, hands back storage the flag owns
(`app.Flag("v", "").Bool()` returns `*bool`) and is well liked for it. Against
those, cobra and pflag offer `flags.GetString(name)`, which is the shape being
rejected here.

## Decision

**A flag holds its configuration and its runtime state, and answers for
both.**

### Two types and an interface

Go forbids type parameters on methods, so a `Bind[T]` method cannot exist. The
type carries the parameter instead, and the command tree holds an interface:

    type Flag interface{ ... }   // sealed by an unexported method

    type FlagBuilder[T any]      // declaration-time vocabulary only
    type Setting[T any]          // Value() T · IsSet() bool
                                 // Source() Source · Count() int

    func (c *FlagBuilder[T]) Setting() *Setting[T]
    func (c *Command) Flags(flags ...Flag) *Command

`Command.Flags(...Flag)` reads as it always has, so the most common call site
does not change. The fluent style survives generics: `Hidden()` on a
`*FlagBuilder[string]` returns a `*FlagBuilder[string]`, so a chain keeps its
type to the end.

### The terminal call is optional

`Setting()` is both the terminal call and the accessor, and returns the same
object every time. Where it goes is the author's choice rather than a tax the
package charges:

    // Hoisted: the program keeps the Setting, and reads are depth one.
    var outputFlag = climux.String("output", "json", "Output format").
            Env("APP_OUTPUT").Setting()

    cmd.Flags(outputFlag)
    outputFlag.Value()

    // Not hoisted: a flag nothing reads by handle pays no terminal call.
    cmd.Flags(climux.String("log-level", "info", "...").
            Choices("debug", "info"))

`Flags` takes either, because both satisfy `Flag`. A program that reads a flag
cannot forget the terminal call, because there is nothing to read without it.

### Storage, and Bind

A flag stores `*T` and returns `T`. The pointer is its own cell unless
`Bind(p *T)` redirects it at a variable the program owns. Redirection, never a
mirror: there is one cell, so there is no question which copy is
authoritative and no moment where they disagree.

**The package does not own a caller's variable until parsing.** Declaring a
flag writes nothing anywhere. Defaults are applied by `Parse`, to the flags in
scope, before argv is applied over them. Constructing a declaration has no
business touching memory the program has not handed over yet.

A handler that assigns a bound variable behind the flag's back desynchronizes
`Value` from `Source`. The warning is in the name `Bind`, and it is the same
rope `flag.StringVar` already hands out; nothing is added to prevent it.

### Defaults are a setter

Because defaults land at parse rather than at construction, a layer that wants
to override one — a configuration file, in the manner of kubectl's kuberc —
says so declaratively, and the command line still wins:

    outputFlag.Default(cfg.Output)

### Repetition needs no type of its own

A repeated flag is a `FlagBuilder[[]string]`. The bound value does the
accumulating, as it does today, so `Value()` returns the whole slice.

There is no `Occurrences`. An occurrence in this dialect carries exactly one
option-argument, so for a slice flag `Value()` **is** the occurrence list, one
element per naming. For a scalar declared last-wins the overwritten values are
gone, but choosing last-wins is choosing to discard them, and an author who
wants them declares a slice. Frameworks that let one occurrence carry several
values have a real question here; this one does not.

`Count() int` is the whole of what survives, and is the one fact `Value`
cannot carry: how many times a scalar was overwritten, and how many times a
boolean was named, which is what `-vvv` counts.

### The split is one-way

`Setting` offers no route back to the declaration. `Command.Flags` reaches the
configuration through the sealed interface's unexported method, so the package
has what it needs without publishing the way.

Configuration is already hard to relitigate once parsing has happened. That
property is worth defending rather than merely enjoying, and an accessor
pointing back from runtime to configuration would erode it for the sake of
symmetry. It is far easier to add one later, against a real case, than to
withdraw one that programs have begun to lean on.

### What the invocation keeps

Only what describes the line: the command, the forwarded arguments, the
interrupt and the three streams. It records nothing about flags. Anything else
would be the second path returning, which is the whole of what this removes.

### A tree reads one command line, and now says so

`IsSet()` without an argument is meaningful only because a tree reads one
command line. That rule already holds, but it was deliberately unenforced:
refusing a second read required state recording that a tree had been read, and
the compiled tree was no place for it. The declaration is exactly that place.
`Parse` against a tree already read now returns an error rather than leaving
the result unspecified.

## Alternatives considered

**Accessors on the compiled flag.** Fails twice. `ir.Flag` cannot be generic,
because lowering builds one heterogeneous tree, so the accessor returns `any`
and the typing that motivates the whole design evaporates. And a program
holding a declaration has no compiled flag without a lookup, which is the
friction being removed.

**Free functions**, which is abseil's literal shape: `climux.Value(outputFlag)`.
Legal, since a free function may be generic where a method may not, and it
keeps the declaration type purely declarative. Rejected for reading badly at
every call site and for spending package-level names on what are plainly
methods.

**A phase-typed builder with a mandatory terminal call**, which is kingpin's
shape. Its guarantee is worth having and is kept: the split is enforced by
type rather than documented. Its cost is not — a terminal call on every
declaration, including the many that nothing ever reads. Making the call
optional keeps the guarantee where it applies and drops the ceremony where it
does not.

**Two interface views over one struct**, narrowing the surface by declaring
the variable's type. Elegant at the call site and rejected for missing the
point: the generated documentation still shows both vocabularies on the
concrete type, which is where the confusion starts.

**A separate builder and setting pair for repeated flags**, so that an
occurrence accessor could be typed differently for one value and for many.
Rejected once that accessor was found to be reporting something `Value`
already reports.

**Keeping the bound variable canonical and letting `Bind` mirror into the
flag.** Rejected: two cells raise a synchronization question and an
authority question, and neither has a good answer.

## Consequences

The declaration type gains state, so
docs/adr/configuration-types-carry-no-behavior.md is false as stated. What
survives of it is the part that mattered: a declaration carries no *behavior*
a parser or a formatter must reach through. It carries a value, which is data.

`Parse` grows a pass that applies defaults to the flags in scope, and a check
that refuses a second read. Both are small, and the second closes a gap the
earlier decision accepted on the grounds that it had nowhere to put the state.

The machinery linking a declaration to its compiled flag is deleted, along
with the invocation's record of flag sources and the accessors that read it by
name. Provenance stops being a thing the invocation reports and becomes a
thing a flag knows, which is roughly a net deletion.

Every constructor changes shape, since the bound pointer leaves the argument
list and becomes `Bind`. Flag groups, registries, lowering, the example
program, the test suite, the package documentation and the README all follow.
This is the largest single change to the package's surface so far, and it is
affordable only because nothing depends on the package yet.

`FromFlagSet` imports values whose storage lives elsewhere and whose type is
not known statically. They cannot be a `FlagBuilder[T]`, so an untyped escape
remains: a constructor returning something that satisfies `Flag` without a
type parameter, readable only through the variable the caller already owns.

Distinguishing an argv-sourced value from an environment-sourced one still
needs a seam the parser calls, since `Set` alone cannot tell the difference.

A flag may be declared at package scope, and then it is global, or built
inside a constructor, and then it is not. Unlike abseil, nothing forces the
first.
