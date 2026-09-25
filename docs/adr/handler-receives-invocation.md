# Handlers receive the invocation

Status: accepted, 2026-08-23.

## Context

climux exists so that many teams can compose one binary. A team ships a
`*Command`; somebody else decides where in the tree it hangs, and may rename
or move it later. That is the premise the whole package is built on.

The v0 handler signature was `func(args []string) int`, and the first draft
of the execution model kept its shape as
`func(ctx context.Context, args []string) error`. Two problems with it:

- `args` held only the arguments the parser left unread, so it was empty
  for nearly every command. The signature spent its one payload parameter
  on a minority feature.
- A handler cannot learn where it is mounted. Its own path is decided in
  another package, after registration, by someone who may not be its author.
  Closing over the path when the command is constructed only works when the
  same person writes the command and mounts it. `mitchellh/cli` forces
  exactly that workaround, and Terraform pays it.

Meanwhile `Parse` had just been changed to return an
`*Invocation`, which `Run` was deconstructing in order to pass `Args`
alone. The information was being assembled and then discarded.

The alternative considered was to leave the signature at `(ctx, args)` and
attach the invocation to the context, with a `FromContext` accessor. Its
real merit is that it survives intermediaries: a house wrapper that adapts
handlers to some other signature passes the context along without having to
know what is inside it.

## Decision

    type HandlerFunc func(ctx context.Context, inv *Invocation) error

The invocation is passed as a parameter, not carried in the context.

- Context values are for request-scoped data crossing API boundaries, not
  for the parameters of a call this package makes itself.
- The signature is the documentation. An author, or an agent authoring a
  CLI, sees `inv.` and finds the whole surface; `FromContext` has to be
  discovered in prose first.
- `FromContext` would have to answer for a context that never passed through
  `Run` and return nil or a zero value that lies. Tests, where handlers are
  called with a bare context, are where that bites.
- The signature is breaking in this release regardless, so the parameter
  costs nothing now and would cost compatibility after v1.

Prior art agrees on handing the handler something that knows its position:
cobra's `RunE(cmd *cobra.Command, args []string) error` with
`cmd.CommandPath()`, urfave/cli v3's `func(context.Context, *cli.Command)
error`, and kong injecting a `*kong.Context` whose `Command()` is the
resolved path. None of them put it in the context; cobra does the inverse
and hangs the context off the command. Where we differ is that they hand
over the configuration object, which then needs getters, and we hand over a
parse result, which keeps the source type sealed.

## Consequences

- Every handler in every program changes. The package is pre-v1 with no
  meaningful adoption, so no migration is offered.
- Naming the command that is running becomes reachable from the one place
  that wants it, through `Invocation.Cmd`: `Cmd.FullName` reads as
  "app remote add" and `Cmd.Ancestry` is the commands themselves. An
  `Invocation.Path` holding the same names was tried and cut, since it
  answered a question `Cmd` already answers.
- Middleware (`func(HandlerFunc) HandlerFunc`) inherits the invocation for
  free, which is where a telemetry label naming the command belongs.
- Handlers that ignore `inv` carry one unused parameter, as they carried an
  unused `args` before.
- `Invocation` is now load-bearing API, so its fields are part of the v1
  surface. Adding fields stays compatible; renaming them will not be.
- Attaching the invocation to the context remains available later as a pure
  addition, if a wrapper that cannot be changed ever needs it.
- `Path[0]` is whatever `NewCommand` was given, commonly `os.Args[0]`, so a
  message built from it may show a full binary path. That is the composing
  author's call, not the package's.

## The streams travel with the invocation

Where a handler's output goes is the second thing its author cannot know.
The premise above is that a command is mounted by someone other than the
person who wrote it, and the stream that command writes to is decided in
the same place, and at the same time, as the path it is reached by. So
`Invocation` carries `Stdin`, `Stdout` and `Stderr` alongside `Path`, and
a handler looks in one place for both. Where those streams are set is
docs/adr/streams-are-run-environment.md.

Anything narrower fails the party doing the mounting. Streams reachable
only by help and error messages let a program point a command at a buffer
and capture the `Argument error:` line while missing everything the
handler printed, which is most of what a CLI emits. A handler writing to
`os.Stdout` cannot be tested by the very party that mounted it.

Consequences, beyond those above:

- `Invocation` is not only "what the command line said". It is what the
  handler needs in order to run, which is the wider promise and the one
  worth keeping: anything else the package resolves on a handler's behalf
  belongs here too.
- Three more field names join the v1 surface, and handlers should use them.
  Nothing enforces it, as nothing can: `fmt.Println` in a handler is a
  defect the package can document but not detect. This mirrors `flag`,
  which offers `SetOutput` and cannot stop anyone printing past it.
- Each stream is set and resolved on its own, so redirecting stdout leaves
  errors on stderr. Taking both writers at once would inherit them as a
  set: a call like `Output(&buf, nil)` has to resolve a nil stderr or
  panic on the first error message, and saying "leave this one alone"
  means passing a nil.
- One word follows each stream from the option that supplies it to
  `Invocation.Stdout`. That is worth more than matching
  `flag.FlagSet.SetOutput`, which has a single output stream and so never
  had to name a second one.
