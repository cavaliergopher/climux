package argv

import (
	"os"

	"go.hotsrc.dev/climux/ir"
)

// Parse reads args against the compiled command cmd and stores the value
// of each argument in each flag's target. The rules for each flag are
// checked and any errors are returned.
//
// Parse reads one command line against cmd. Calling it again with the
// same tree is not supported: a flag keeps whatever the previous call
// gave it, and a value that accumulates keeps accumulating. It does not
// validate the tree: a tree produced by (*climux.Command).Compile is
// already validated.
//
// The returned Invocation names cmd, or one of its subcommands if the
// arguments specified one.
//
// If an interrupt is specified, such as the flag asking for help,
// parsing stops there and the returned Invocation names it. That is not
// an error: it is for the caller to run what the interrupt asks for.
func Parse(cmd *ir.Command, args []string) (*ir.Invocation, error) {
	return apply(cmd, lex(cmd, args))
}

// apply walks res's instructions in order against the commands and flags
// they name, and returns the resulting Invocation. It is what is left of
// parsing once lex has resolved argv: Set, environment variables and
// NArgs validation. Everything with an effect happens here, and nothing
// here decides what argv means -- lex has already done that, before apply
// ever runs.
//
// An interrupt anywhere in res wins over every recorded lex error,
// discarding them: this is what lets "cmd --bogus --help" print help
// instead of failing, since a user who is asking for help does not need
// their typo reported too. apply walks instructions up to the first
// interrupt it finds -- earlier Set and Dispatch instructions still run,
// so flags given before it take effect -- and stops there: env vars are
// not read and NArgs is not checked. The tokens after the interrupt were
// never lexed; they ride on its instruction and land on the invocation
// as Forwarded.
//
// A command that is itself an interrupt (ir.Command.Interrupt) is
// answered the same way once it is the command the line named: every
// recorded lex error is discarded, env vars are not read and NArgs is
// not checked, and the tokens after its name arrive through the
// instForward instruction lex emitted when it stopped there.
//
// Otherwise, any recorded lex error stops apply before it starts: nothing
// is mutated unless every argument in argv resolved. An error from Set or
// a ValidateFunc can still stop apply partway through, since undoing a
// caller-owned variable it already wrote is not this package's to do.
func apply(root *ir.Command, res lexResult) (*ir.Invocation, error) {
	interruptAt := -1
	var interrupt instruction
	for i, instr := range res.instructions {
		if instr.kind == instInterrupt {
			interruptAt, interrupt = i, instr
			break
		}
	}
	// res.active is the command lex ended on, which is also the command
	// the walk below ends on whenever no flag interrupt cuts it short --
	// checking it here, before the walk, is what lets a command interrupt
	// win over a recorded lex error the same way a flag interrupt does.
	cmdInterrupt := interruptAt == -1 && res.active.Interrupt != nil
	if interruptAt == -1 && !cmdInterrupt && len(res.errs) > 0 {
		return nil, res.errs[0]
	}

	limit := len(res.instructions)
	if interruptAt != -1 {
		limit = interruptAt
	}

	active := root
	scope := []*ir.Command{root}
	counts := make(map[*ir.Flag]int)
	sources := make(map[*ir.Flag]ir.Source)
	var forwarded []string
	for _, instr := range res.instructions[:limit] {
		switch instr.kind {
		case instSet:
			if err := setFlag(active, instr.flag, instr.value); err != nil {
				return nil, err
			}
			counts[instr.flag]++
			sources[instr.flag] = ir.SourceArgs
		case instDispatch:
			active = instr.cmd
			scope = append(scope, active)
		case instForward:
			forwarded = instr.forwarded
		}
	}

	if interruptAt != -1 {
		// The interrupt binds no value, so nothing Set it, but the
		// command line named it: recording it is what lets a program ask
		// whether it was given the same way it asks about any other flag.
		sources[interrupt.flag] = ir.SourceArgs
		return invocationFor(interrupt.cmd, interrupt.forwarded, interrupt.flag, sources), nil
	}
	if cmdInterrupt {
		return invocationFor(active, forwarded, nil, sources), nil
	}
	if err := applyEnvVars(scope, counts, sources); err != nil {
		return nil, err
	}
	if err := validateNArgs(active, scope, counts); err != nil {
		return nil, err
	}
	return invocationFor(active, forwarded, nil, sources), nil
}

// setFlag sets f's value to token, wrapping a failure the same way it
// always has: the flag alone, or the flag followed by the error it wraps.
// active is the command reported on the error, the one in scope when the
// instruction naming f was lexed.
func setFlag(active *ir.Command, f *ir.Flag, token string) error {
	if err := f.Set(token); err != nil {
		return ir.NewArgumentErrorf(err, active, f, token, "%s", f)
	}
	return nil
}

// applyEnvVars fills every flag in scope from its environment variable,
// for whatever counts has no occurrence of yet, then counts it as seen so
// validateNArgs sees it satisfied and records it in sources as having
// come from the environment. A flag the command line already set is
// skipped, which is what leaves its recorded source ir.SourceArgs.
//
// scope is the commands dispatched through, beginning at the one Parse was
// called on rather than at the root: a flag an ancestor of that command
// declares was never matchable, so it is not checked here either. Scope
// order, then group order, then declaration order within each command:
// this is deterministic.
func applyEnvVars(scope []*ir.Command, counts map[*ir.Flag]int, sources map[*ir.Flag]ir.Source) error {
	for _, cmd := range scope {
		for _, group := range cmd.FlagGroups {
			for _, f := range group.Flags {
				if f.EnvVar == "" || counts[f] > 0 {
					continue
				}
				s, ok := os.LookupEnv(f.EnvVar)
				if !ok {
					continue
				}
				if err := setFlag(cmd, f, s); err != nil {
					return err
				}
				counts[f]++
				sources[f] = ir.SourceEnv
			}
		}
	}
	return nil
}

// validateNArgs verifies each flag in scope was given as many times as it
// requires. Every flag that became active along the descent is checked, so
// an ancestor's Required flag still binds when a subcommand is invoked.
//
// active, the deepest command reached, is what every error here is
// reported against, whichever command in scope actually declared the
// offending flag -- the same command instructions were applied against
// throughout, since checking counts is the last of the work apply does.
//
// The offending flag goes at the end of these messages, which is where
// Go's flag package, argparse and getopt all put it when nothing follows
// it. A flag leads the message only when a wrapped error follows, as in
// "--ip: invalid IP: 256.0.0.1", where it scopes what comes after the
// colon; see docs/adr/human-readable-errors.md.
func validateNArgs(active *ir.Command, scope []*ir.Command, counts map[*ir.Flag]int) error {
	for _, cmd := range scope {
		for _, group := range cmd.FlagGroups {
			for _, f := range group.Flags {
				n := counts[f]
				if f.MinCount > 0 && n < f.MinCount {
					switch {
					case f.MinCount == 1:
						return ir.NewArgumentErrorf(nil, active, f, "",
							"missing required argument: %s", f)
					case f.MinCount == f.MaxCount:
						return ir.NewArgumentErrorf(nil, active, f, "",
							"expected %d arguments, got %d: %s",
							f.MinCount, n, f)
					default:
						return ir.NewArgumentErrorf(nil, active, f, "",
							"expected at least %d arguments, got %d: %s",
							f.MinCount, n, f)
					}
				}
				if f.MaxCount > 0 && n > f.MaxCount {
					return ir.NewArgumentErrorf(nil, active, f, "",
						"argument specified too many times: %s", f)
				}
			}
		}
	}
	return nil
}

// invocationFor returns the Invocation apply reports for cmd having become
// active, naming every command in path from the one Parse was called on to
// cmd itself.
//
// sources is the record of where each flag set along the way got its
// value, and is handed over rather than copied: apply is done with it by
// the time it builds the Invocation, and nothing else holds a reference.
//
// Its streams are left nil. Streams are process environment, so the entry
// point holding them fills them in.
func invocationFor(cmd *ir.Command, forwarded []string, interrupt *ir.Flag, sources map[*ir.Flag]ir.Source) *ir.Invocation {
	return &ir.Invocation{
		Cmd:       cmd,
		Forwarded: forwarded,
		Interrupt: interrupt,
		Sources:   sources,
	}
}
