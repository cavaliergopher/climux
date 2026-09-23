# Configuration types carry no behavior

Status: accepted, 2026-08-29; implemented 2026-08-31. Revised 2026-09-23 by
docs/adr/a-flag-holds-its-own-value.md, under which a flag declaration holds
its own value and so carries state. What survives is the claim that matters:
a configuration type carries no behavior a parser or a formatter must reach
through.

## Context

`climux.Command` and `climux.Flag` are the builder tier of the three-type
model (see `three-type-model.md`): what a program declares, with chained
setters, before anything is resolved. They had also accumulated the verbs
that act on a declared tree — `Parse`, `Run`, `Dispatch`, `Complete`, and
the unexported error reporting behind `Run`.

That made one type answer two questions. A reader looking for "what can I
declare?" waded through dispatch; a reader looking for "how do I run
this?" found it on the object they were still building. It also made the
type look runnable, which invited `cmd.Run(ctx, args)` in programs whose
tree had never been validated as a whole.

The package already had the answer in front of it: `Run(ctx, cmd)` and
`RunWithArgs(ctx, cmd, args...)` were package-level functions taking a
`*Command`, sitting beside methods that did the same work.

## Decision

Everything on the configuration types that is not building or validating
configuration comes off them, and becomes a package-level function taking
the command:

    func Run(ctx context.Context, cmd *Command) int
    func RunWithArgs(ctx context.Context, cmd *Command, args ...string) int
    func Dispatch(ctx context.Context, cmd *Command, args ...string) error
    func Parse(cmd *Command, args ...string) (*Invocation, error)
    func Complete(cmd *Command, args []string, word string) (
        []string, ir.CompDirective)

`*Command` keeps its chainable setters, `NewCommand`, `String`, and
`Compile`. `*Flag` keeps its setters. `FlagGroup` and `Registry` are
configuration only and keep everything they have.

- **The args parameter is variadic** wherever a call takes only a command
  line, matching `RunWithArgs`, which had the shape first. `Complete` takes
  a slice because it also takes the word under the cursor, so there is
  nothing to be variadic about.
- **`Compile` stays a method.** Lowering is the one thing a configuration
  tree exists to do — it is the type's own transformation into its
  implementation form, not an operation a consumer performs on it.
  Everything that moved is about reading a *command line*; `Compile` is
  about the *tree*.
- **No entry point takes an `*ir.Command`.** The standard case is `Run` on
  the configuration, parsing internally; handling compiled values stays an
  advanced path reached through `Compile`. An interface satisfied by both
  forms was considered and rejected, because it would put the compiled type
  into the signature every ordinary program reads.

## Consequences

- `command.go` is now legibly the configuration type: a struct, a
  constructor, `Compile`, `lower`, and setters. The entry points and the
  error reporting live together in `climux.go`, which is what a reader
  wanting to run a tree opens.
- The exit-code contract moved with the code, from `Command.Run`'s doc to
  `RunWithArgs`, which is now the implementation rather than a wrapper.
- Where a new operation goes has a rule rather than a precedent: if it
  builds or validates configuration it is a method, and if it acts on a
  declared tree it is a function.
- Spelling a flag is not a configuration type's to do. `ir.Flag.String`
  renders the options the dialect wrote, which is what every error and
  help message wants; a `String` on the configuration `*Flag` would have
  to lower a throwaway `ir.Flag` to read one field back, making it both
  behavior on a configuration type and a dialect leak, since spelling
  belongs to `internal/argv`.
