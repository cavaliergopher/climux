# Flags are local by default

Status: accepted, 2026-09-23. Not yet implemented.

## Context

A command with both a handler and subcommands has flags that mean something
only to its own handler:

    git remote --verbose           # list remotes with their URLs
    git remote add --verbose ...   # means nothing to add

Other flags mean the same thing everywhere beneath the command that
declares them: a root `--actor`, or a `--timeout` a registry contributes. A
tree needs both, and the program author is the one who knows which is
which.

A name also needs one meaning wherever it can be written, or one spelling
sets whichever variable the parser happened to reach last. The question is
how wide that namespace is. Making it binary-wide, as absl does, lets a flag
go anywhere on the line, but sibling commands can then never both have
`--force`, and a collision between two teams' packages lands on whoever
composes the binary, who cannot rename either. Commands and flag groups are
written by teams that do not control where they are mounted, so the
namespace has to be narrower than the binary.

Established parsers make local the default and inheritance the opt-in.
cobra splits `Flags` from `PersistentFlags`, clap marks an argument
`global(true)`, and argparse gives each subparser its own options.

## Decision

A flag is local by default. It is valid from its own command's name until
the line dispatches to a subcommand, and unknown after that. A flag marked
`.Persistent()` stays valid, with the same meaning, everywhere beneath its
command.

    git remote --verbose add ...   # valid: written in remote's scope
    git remote add --verbose ...   # local: unknown flag
                                   # persistent: sets remote's --verbose

Scope ends at dispatch, not at a word that names a subcommand. A command
with positionals takes such a word as data until its positionals are full,
and its local flags stay valid while it does:

    app FILE run --verbose         # "run" fills FILE; --verbose is app's

Scope governs where a flag may be written, not who reads it or whether it
resolves. A flag binds a variable any handler may read, so a local flag on
`remote` still steers `add`'s handler when written before `add`, and its
default and environment variable apply whichever command runs.

A positional is always local, since it is filled before dispatch and no
descendant can write it. Marking one persistent is a configuration error.

A name must be unique among the flags writable at any one position. A
persistent flag's names cannot be redeclared anywhere beneath it, a local
flag's names may be reused by descendants, and no command may declare a
name twice. The check runs where the whole tree is in view, since a command
cannot know its ancestors until it is mounted.

`HelpFlag` declares a persistent flag, so `--help` on the root reaches every
command. `VersionFlag` declares a local one. A persistent interrupt answers
for the command whose scope it was written in, so `app sub --help` is help
for `sub`.

A command's help lists its own flags and its ancestors' persistent flags,
which is exactly the set it accepts. The description records scope:
`desc.Flag` carries `persistent`, true for a persistent flag and omitted
otherwise.

## Consequences

- Help and parsing agree. A flag is listed wherever it is accepted and
  nowhere else.
- Programs mark their global flags `.Persistent()`. In orbital that is
  `--actor`, `--out`, telemetry's `--log-level` and `--trace`, and the
  legacy group's flags. A flag group decides per flag, so a registry
  contributing globals marks them.
- A persistent flag group mounted at two depths of one path is an error.
  The fix is to mount it once, higher up. A local group may be mounted at
  any depth.
- Ordering matters in both directions. `app --sub-only sub` is an error,
  and so is `app sub --parent-local`. The unknown-flag error says where the
  name would work, since the tree is static and fully known: "defined by
  subcommand `sub`", or "an option of `app`".
- `git remote --verbose add --verbose` is legal when `remote`'s flag is
  local, and sets two variables, one per command.
- `orbital deploy --version` is an unknown flag.
- A required local flag must be written before dispatch, unless its
  environment variable supplies it.
- A package that exports a flag group cannot prove on its own that mounting
  it will succeed, since names are checked against the tree.
- A description consumer that ignores `persistent` reads every ancestor
  flag as in scope.
