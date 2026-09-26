# A flag holds its own value

Status: accepted, 2026-09-23; implemented 2026-09-26.

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

    type Flag interface{ lower(...) }   // sealed, unexported

    type FlagBuilder[T any]      // declaration-time vocabulary only
    type FlagState[T any]        // Value() T · IsSet() bool
                                 // Source() Source · Count() int

    func (c *FlagBuilder[T]) State() *FlagState[T]
    func (c *Command) Flags(flags ...Flag) *Command

`FlagState[T]` is the typed reading of `ir.FlagState`, and `ir.Flag.State`
points at it: one word for one thing across the packages. It is not
`Setting`, because authors name their own configuration structs
`FooSettings`, and it is not `Flag`, because the sealed interface is.

`Command.Flags(...Flag)` reads as it always has, so the most common call site
does not change. The fluent style survives generics: every setter --
`Hidden`, `Positional`, `EndOfOptions`, `Interrupt` and the rest -- returns
the receiver's own type, so a chain keeps its `T` to the end.

The interface is one method, because lowering is the whole of what a command
needs from a flag. Defaults land at parse rather than at construction, but
writing one needs a `T` that a `[]Flag` cannot supply, so the flag lowers a
closure over its own and the parser calls it: `ir.FlagState.SetDefault`,
beside `Get`. `FlagState[T]`'s public surface is read-only; the write is
reached only through the compiled node.

### Presence rides on the flag, not on the value

The declaration allocates the state one reading writes, and every node
lowered from it points at that one object:

    // ir
    type FlagState struct {
        Source     Source
        Count      int
        Get        func() any   // typed in a package ir cannot name
        SetDefault func()       // the one write for a flag never named
    }

    // ir.Flag
    State *FlagState

The applier writes `Source` and `Count` at every site that names a flag. It
cannot ride on `Value.Set`: `Set` never runs for a flag that binds no value,
so an unbound flag's presence would go unrecorded, and even where it does
run it cannot tell argv from the environment, because both arrive as a
token.

A node points at state and holds none. The compiled tree models meaning,
not history, and may be compiled more than once; two compiles of one
declaration agree because they share the object rather than copying it. So
do two nodes in one tree: a registry group mounted at two depths of a path
lowers to a node at each, and the line naming either names the declaration
once. That identity also replaces the `Origin` the compiled flag used to
carry.

### The terminal call is optional

`State()` is both the terminal call and the accessor, and returns the same
object every time. Where it goes is the author's choice rather than a tax the
package charges:

    // Hoisted: the program keeps the FlagState, and reads are depth one.
    var outputFlag = climux.String("output", "Output format").
            Default("json").Env("APP_OUTPUT").State()

    cmd.Flags(outputFlag)
    outputFlag.Value()

    // Not hoisted: a flag nothing reads by handle pays no terminal call.
    cmd.Flags(climux.String("log-level", "...").Default("info").
            Choices("debug", "info"))

`Flags` takes either, because both satisfy `Flag`. A program that reads a flag
cannot forget the terminal call, because there is nothing to read without it.

### Storage, and Bind

A flag stores `*T` and returns `T`. The variable is its own unless
`Bind(p *T)` points it at one the program owns. Redirection, never a mirror:
there is one variable, so there is no question which copy is authoritative
and no moment where they disagree.

**A variable is written once per reading, or not at all.** Declaring a flag
writes nothing anywhere; constructing a declaration has no business touching
memory the program has not handed over yet. Parsing writes what the line
says, else the declared default, else nothing. A reading begins by forgetting
the one before it -- `Source` and `Count` go back to zero, and no variable is
touched -- and ends by writing the default of every flag it never named,
through `ir.FlagState.SetDefault`. The pass covers the whole tree rather
than the scope the line reached, because a flag out of scope holding its
default is what a program expects of it, and the count it consults is the
declaration's, so a flag mounted twice in one path is named once for both
nodes.

A flag with no `Default` is left as the program had it, which is what the
`flag` package's `Var` has always done with the variable it is given.

A handler that assigns a bound variable behind the flag's back desynchronizes
`Value` from `Source`. The warning is in the name `Bind`, and it is the same
rope `flag.StringVar` already hands out; nothing is added to prevent it.

### Defaults are a setter

`Default(v T)` is the one way to declare a default; no constructor takes one.
The zero-valued `""`, `0` and `false` that the `flag` package's constructors
carry in their middle are gone from every call site that has no default,
which is most of them, and kingpin's `Flag(name, help).Default(x)` is the
precedent.

Because defaults land at parse rather than at construction, a layer that wants
to override one — a configuration file, in the manner of kubectl's kuberc —
says so declaratively, and the command line still wins:

    outputFlag.Default(cfg.Output)

### A VarType describes a type; the flag holds the value

An author binding a type this package has no constructor for describes it
once -- how a value is decoded, how it is shown, and what the schema reports
-- and the flag does the rest:

    type VarType[T any] interface {
        Decode(value *T, s string) error
        Format(value T) string
        Kind() ir.Kind
    }

    func Var[T any](name, usage string, t VarType[T]) *FlagBuilder[T]

    var ipFlag = climux.Var("ip", usage, ipType{}).Default(net.IPv6zero).State()

    ipFlag.Value()   // net.IP

`Decode` is `flag.Value.Set` with the storage passed in rather than captured,
which is what lets the type stay stateless and the flag own the variable:
`Default` and `Bind` then work for a custom type exactly as for a `String`.
Abseil's `AbslParseFlag(text, T* dst, err)` is the same shape, and so is
`encoding.TextUnmarshaler`. `Format` and `Kind` are required rather than
optional, because an author describing a type says what it is; pflag's
`Value` carries the same three -- `Set`, `String`, `Type` -- minus the
storage. `Var` takes one argument beside the name and usage, as `flag.Var`
does, and the built-in constructors are each `Var` with a type of their own.

It is `Decode` rather than `Parse` because `Parse` is already this package's
word for reading the whole line, and rather than `Unmarshal` because an
option-argument is not a wire format. `json.Decoder.Decode(v)` is the shape
being named.

**A flag's first naming replaces its default, whatever `T`.** Before the
first `Decode` the variable is zeroed; every later naming folds into what
earlier ones wrote. A scalar overwrites and never notices. A slice appends
and a map initialises on its nil check, so an accumulator carries no
state:

    Strings("tag", usage).Default([]string{"stable"})

    never named        ->  [stable]
    --tag a --tag b    ->  [a b]        // first Decode found nil, not [stable]

The alternative, an accumulator that decides for itself whether the
variable holds a default or its own earlier output, cannot decide: the two
look the same. Only the parser knows, from `Count`, and the zero is how it
says so. This is pflag's and clap's rule, and it retires the `hot` flag the
slice value carries today to make the same decision with private state.

`BitField` is a `Bool` with a second place to write: its own value is its
bit, and its `Decode` and its default both OR a mask into a word several
flags share. The word is a pointer parameter, the one that survives, because
it is not the flag's variable; `Bind` is for the bit. Nothing clears a bit,
as nothing did before.

### Repetition needs no type of its own

A repeated flag is a `FlagBuilder[[]string]`: `Var` with a decoder that
appends. `Value()` returns the whole slice.

There is no `Occurrences`. An occurrence in this dialect carries exactly one
option-argument, so for a slice flag `Value()` **is** the occurrence list, one
element per naming. For a scalar declared last-wins the overwritten values are
gone, but choosing last-wins is choosing to discard them, and an author who
wants them declares a slice. Frameworks that let one occurrence carry several
values have a real question here; this one does not.

`Count() int` is the whole of what survives, and is the one fact `Value`
cannot carry: how many times a scalar was overwritten, and how many times a
boolean was named, which is what `-vvv` counts.

### Where a flag's state is reachable, Value reports what was bound

There is no empty `None` type and no flag whose `Value` is vacuous.

`Unbound` takes no argument, so what it binds is whether it was named:

    var dryRun = climux.Unbound("dry-run", "Report, do not act").State()

    dryRun.Value()   // true, given --dry-run
    dryRun.IsSet()   // the same fact, said the other way

`Value` and `IsSet` agreeing is the point rather than a redundancy, and it
costs nothing in the parser: the state reads its own `Count`, so the
compiled flag still carries no `ir.Value` and the lexer's rule that
`--dry-run` takes no argument is untouched. `climux.Bool` and `Unbound` share
`FlagBuilder[bool]` because `T` describes what is read back; what argv does
stays a property of the compiled flag, where it already lived.

`FromFlagSet` needs no rule. It builds its flags inside its own loop and
returns a group, so no state it makes is reachable and no `Value` it holds
can be called.

`Func` is the one flag that takes an argument and binds nothing, having given
it to a callback instead. It follows `Unbound`: the author routed the value
somewhere, so the package stores none on their behalf and the state reports
that the flag was named.

### An interrupt reads its own value

An interrupt still binds a value, so its handler reads it from the flag
instead of from a variable the declaration had to share:

    var helpTopic = climux.String("help", "", "Show help for a topic").
            Interrupt(showHelp).State()

    func showHelp(ctx context.Context, inv *climux.Invocation) error {
        return topicHelp(inv.Stdout, helpTopic.Value())
    }

    app --help deploy   ->  showHelp runs, Value() is "deploy"

### The split is one-way

`FlagState` offers no route back to the declaration. `Command.Flags` reaches the
configuration through the sealed interface's unexported method, so the package
has what it needs without publishing the way.

Configuration is already hard to relitigate once parsing has happened. That
property is worth defending rather than merely enjoying, and an accessor
pointing back from runtime to configuration would erode it for the sake of
symmetry. It is far easier to add one later, against a real case, than to
withdraw one that programs have begun to lean on.

### What the invocation keeps

The command and the three streams. Both are per-run and can live nowhere else:
a command cannot know its own mounted path until it runs, and the streams are
whatever `Run` was given, so parking them on the compiled tree would put
per-run state on something every run shares.

`Interrupt` stays, because it is how the parser tells `dispatch` which
handler runs in place of the command's. A handler that wants to know asks the
flag instead:

    if helpFlag.IsSet() { ... }

Beyond that the invocation records nothing about flags. Anything else would
be the second path returning, which is the whole of what this removes.

A machine consumer -- the conformance harness, a walker holding nothing but
the compiled tree -- reads through the node instead, untyped and keyed by
declared name:

    for _, f := range group.Flags {
        if f.State == nil || f.State.Source == ir.SourceDefault {
            continue
        }
        got[f.Name] = f.State.Get()
    }

It never sees a `FlagState[T]` and never knows a `T`; `ir.FlagState.Get` is the
one untyped read, and it exists because `ir` cannot name the type that
holds the typed one.

This changes what the type is for. It stops being the result of parsing a
command line and becomes the environment a handler runs in, which is what
docs/adr/handler-receives-invocation.md now has to say: that ADR distinguishes
climux from cobra on the grounds that peers "hand over the configuration
object, which then needs getters" while climux hands over a parse result. The
distinction survives but moves. We hand over the command and the streams, as
cobra's `RunE(cmd, args)` does; what we avoid is `cmd.Flags().GetString`,
because the flag is a handle of its own and needs no getter reached through
the command.

### A tree reads one command line, and a second read is well-defined

`IsSet()` without an argument is meaningful because a tree reads one command
line. The rule stands as the shape of a program. It is not enforced, and no
longer needs to be: a reading begins by forgetting the previous one, so a
second `Parse` against one tree answers for the second line alone rather
than leaving the result unspecified, which is what the examples that run one
command twice rely on.

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

**A separate builder and state pair for repeated flags**, so that an
occurrence accessor could be typed differently for one value and for many.
Rejected once that accessor was found to be reporting something `Value`
already reports.

**A value that owns its storage**, which is `flag.Value` with a typed
`Get() T` added. Typed at compile time, and the parser never notices. But the
value holds its variable privately, so `Default` and `Bind` -- two setters every
other `FlagBuilder[T]` has -- cannot reach a flag declared this way, and
fixing that puts a typed setter on the interface and two storage models under
one type. Passing the variable in turns the interface around and keeps one.

**`Get() any` on `ir.Value`**, the stdlib's `flag.Getter`. `Value() T` becomes
a type assertion, so a wrong `Get` panics at first read instead of failing to
compile, which is the typing this decision exists for, lost.

**A plain function in place of `VarType`.** Nothing about decoding needs an
object once the variable is passed in. Rejected because a function can say
nothing else: how the value is shown and what kind it is would have nowhere
to live, and both are what a schema consumer and a help page need from a
type they have never seen.

**Resetting every variable to its default before argv**, then letting
argv overwrite. Two writes where one will do, and wrong for a variable the
program shares between runs or between flags: a default landing on it
clobbers what another flag or an earlier run put there.

**Keeping the bound variable canonical and letting `Bind` mirror into the
flag.** Rejected: two variables raise a synchronization question and an
authority question, and neither has a good answer.

## Consequences

The declaration type gains state, so
docs/adr/configuration-types-carry-no-behavior.md is false as stated. What
survives of it is the part that mattered: a declaration carries no *behavior*
a parser or a formatter must reach through. It carries a value, which is data.

The applier grows two passes, one forgetting the previous reading before argv
and one writing defaults after the environment. Both are small.

The machinery linking a declaration to its compiled flag is deleted, along
with the invocation's record of flag sources and the accessors that read it by
name. Provenance stops being a thing the invocation reports and becomes a
thing a flag knows, which is roughly a net deletion.

Every constructor changes shape, since the bound pointer leaves the argument
list and becomes `Bind`, and the default leaves it and becomes `Default`.
Flag groups, registries, lowering, the example program, the test suite, the
package documentation and the README all follow. This is the largest single
change to the package's surface so far, and it is affordable only because
nothing depends on the package yet.

No constructor needs a shape outside `FlagBuilder[T]`. `Unbound` and `Func`
declare `bool` and report whether they were named; `Var` declares the caller's
own type; the flags `FromFlagSet` builds are never reachable.

The applier keeps its three recording sites and the environment pass; what
changes is where they write. `ir.Flag.State` is the seam, and it is the only
addition the compiled type takes.

`ir.Value` shrinks to the parser's seam: one package-private adapter
implements it over a `VarType`, and `FromFlagSet` remains the sole door for a
`flag.Value`. The value types behind the built-in constructors become
`VarType`s, and the slice value's `hot` flag goes with them.

A flag may be declared at package scope, and then it is global, or built
inside a constructor, and then it is not. Unlike abseil, nothing forces the
first.
