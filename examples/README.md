# Examples

`orbital` is a fictional platform team's deploy and operations CLI, built
to show how a large engineering org would put climux together: several
teams' packages, each owning its own commands, flags and handlers, wired
into one binary by `main`.

## Running it

```
go run ./examples/orbital --help
ORBITAL_ACTOR=alice go run ./examples/orbital deploy status api
ORBITAL_ACTOR=alice go run ./examples/orbital deploy run --service api --release abcd123
ORBITAL_ACTOR=alice go run ./examples/orbital exec --service api -- echo hello
```

Most commands require an identity for the audit trail: pass `--actor` or
set `ORBITAL_ACTOR`. `--help` does not.

## Where each feature lives

| Feature | Where |
| --- | --- |
| Nested subcommands 3+ deep | `orbital deploy status`, `orbital config get\|set` |
| Registry: shared flags and the middleware honoring them | `internal/telemetry` (`--log-level`, `--trace`, timing trace), mounted in `main.go` |
| Registry: a subcommand a library contributes | `internal/debug`, blank-imported by `main.go` |
| `FromFlagSet` importing a legacy `flag.FlagSet` | `internal/legacy` |
| Positional arguments | `internal/config` (`KEY`, `VALUE`), `internal/deploy` (`SERVICE`) |
| Unbounded positional argument | `internal/logscmd` (`orbital logs SERVICE...`) |
| `EndOfOptions` | `internal/execcmd` (`orbital exec --service NAME CMD ARGS...`) |
| `Required`, `Env` | root `--actor` in `main.go`; `internal/telemetry` `--trace` |
| `Choices` | `internal/telemetry` `--log-level`; `internal/deploy` `--env`, `--strategy` |
| `Validate` | `internal/config` (`validKey`); `internal/deploy` (`validVersion`) |
| `Duration` | `internal/deploy` `--timeout`; `internal/logscmd` `--since` |
| `Strings` | `internal/deploy` `--tag`; `internal/logscmd` `SERVICE` |
| `Aliases` | `internal/deploy` `-s`; `internal/logscmd` `-f` |
| `Hidden` command | `internal/debug` |
| `Hidden` flag | `internal/deploy` `--unsafe-skip-health-checks` |
| `ShowDefault` | `internal/telemetry`, `internal/deploy`, `internal/logscmd` |
| `NArgs` | `internal/deploy` `--tag` (`NArgs(0, 5)`); `internal/logscmd` `SERVICE` (`NArgs(1, 0)`) |
| `Middleware` inherited from the root | `internal/middleware.Timing`, declared once in `main.go`, times every command |
| `Middleware` on one command | `internal/middleware.Audit`, declared by `config set`, `deploy run` and `exec` only |
| Dependency injection via closures | `internal/fleet.Client`, threaded into `deploy.Command` |
| Handlers honoring `ctx` cancellation | `internal/deploy` (`status`, `run`), `internal/logscmd` |
| Handlers writing to `inv.Stdout`/`inv.Stderr` | all of them; never `os.Stdout` |
| Exit codes: custom (`Exitf`) | `internal/deploy` `run`, refusing an unconfirmed production deploy (exit 4) |
| Exit codes: plain error | `internal/config` `get`, unknown key |
| `Command.Description` | root command in `main.go`; `internal/deploy` |
| Shell completion (`EnableCompletion`, `Flag.Complete`) | root in `main.go`; `internal/logscmd` `SERVICE` completes from the fleet client |

## Tests

`main_test.go` runs the program as a child process, asserting on real stdout,
stderr and exit codes: the help message in full, the exit-code contract
(0, 1, 2 and a handler's own 4), operand and terminator handling, the
unrecognized-option hint, and the shell completion protocol including a
dynamic callback. An example whose output has drifted is worse than no
example, and only running it end to end catches that.

## Layout

```
examples/orbital/
    main.go                    assembles the tree, NotifyContext, Run
    internal/
        identity/               the --actor audit identity, shared by value
        telemetry/               shared flag group: --log-level, --trace
        legacy/                  a stdlib flag.FlagSet imported with FromFlagSet
        fleet/                   a stand-in fleet API client
        middleware/              the Audit and Timing middleware
        config/                  config get|set
        deploy/                  deploy status|run
        logscmd/                 logs SERVICE...
        execcmd/                 exec -- CMD ARGS...
        debug/                   a hidden diagnostics command
```
