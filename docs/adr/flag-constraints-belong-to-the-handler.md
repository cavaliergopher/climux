# Flag constraints belong to the handler

Status: accepted, 2026-09-27.

## Context

Many command lines are wrong only because of how their flags combine:

```
app create --body "text" --body-file notes.md   # pick one
app build --release --profile dev               # --release means a profile
app deploy --canary                             # --canary requires --percent
```

Every large CLI checks rules like these, and some frameworks declare them.
clap has `conflicts_with`, `requires`, `requires_if`,
`required_unless_present`, `required_if_eq`, `overrides_with` and
`default_value_if`, plus groups that carry constraints. cobra has three
flag-group helpers: `MarkFlagsMutuallyExclusive`,
`MarkFlagsRequiredTogether` and `MarkFlagsOneRequired`.

A handler can already check any of these rules. `IsSet` tells a typed value
from a default, and `NewArgumentErrorf` reports the mistake the way the
parser does, with an `Argument error:` line, the command's usage and exit
code 2:

```go
var (
	body     = climux.String("body", "Body text").State()
	bodyFile = climux.String("body-file", "Read the body from a file").State()
)

func create(ctx context.Context, inv *climux.Invocation) error {
	if body.IsSet() && bodyFile.IsSet() {
		return climux.NewArgumentErrorf(nil, inv.Cmd, nil, "",
			"--body and --body-file cannot be used together")
	}
	// ...
}
```

A declared vocabulary is never complete. cobra's three helpers draw
requests for more relations, and clap's seven still leave rules that
depend on a value, a count or the environment. Whatever set climux chose
would be judged by the rules it cannot express.

## Decision

climux does not declare constraints between flags. A handler checks them
and reports a violation with `NewArgumentErrorf`.

A rule that concerns one flag's own value, such as its type or `NArgs`,
is still declared on the flag, because the parser has to read it anyway.

## Consequences

- Help, the schema and completion do not know about a handler's rules.
  Completion offers `--body-file` after `--body`. A flag's usage text can
  state its rule in words.
- A rule fires only if the handler runs. Interrupts such as `--help`
  answer first, which is what a user asking for help wants anyway.
- A program with many rules writes its own helper. gh does this on top of
  cobra, with `cmdutil.MutuallyExclusive`.
- A rule is plain Go, so it can depend on anything, and its message can
  say what to type instead.
- This would be revisited only if some rule could not be checked in a
  handler. The inputs are `IsSet`, `Source` and `Count` on each flag.
