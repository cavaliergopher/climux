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
// If an interrupt is given, such as the flag asking for help, the
// returned Invocation names it. That is not an error: it is for the
// caller to run what the interrupt asks for.
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
// Any recorded lex error stops apply before it starts: nothing is mutated
// unless every argument in argv resolved. An error from Set or a
// ValidateFunc can still stop apply partway through, since undoing a
// caller-owned variable it already wrote is not this package's to do.
//
// An interrupt changes one thing: a required argument left out is not an
// error. That is how "app --help" answers someone who does not yet know
// what the command requires. Every other rule still holds, so a mistyped
// option beside --help is reported rather than excused. The first
// interrupt flag on the line is the one the Invocation names, together
// with the command it was given on, whose handler it runs in place of. A
// command that is itself an interrupt (ir.Command.Interrupts) is forgiven
// the same way once it is the command the line reached.
func apply(root *ir.Command, res lexResult) (*ir.Invocation, error) {
	if len(res.errs) > 0 {
		return nil, res.errs[0]
	}
	resetStates(root)

	active := root
	scope := []*ir.Command{root}
	counts := make(map[*ir.Flag]int)
	var interrupt *ir.Flag
	var interrupted *ir.Command
	for _, instr := range res.instructions {
		switch instr.kind {
		case instSet:
			if err := setFlag(active, instr.flag, instr.value); err != nil {
				return nil, err
			}
			counts[instr.flag]++
			setSource(instr.flag, ir.SourceArgs)
		case instDispatch:
			active = instr.cmd
			scope = append(scope, active)
		case instGiven:
			// Bound to no value, so nothing Set it, but the command line
			// named it, which is the whole of what it can report.
			setSource(instr.flag, ir.SourceArgs)
		case instInterrupt:
			if interrupt == nil {
				interrupt, interrupted = instr.flag, instr.cmd
			}
			// An interrupt bound to no value was never Set, but the command
			// line named it: recording it is what lets a program ask
			// whether it was given the same way it asks about any other
			// flag.
			setSource(instr.flag, ir.SourceArgs)
		}
	}

	if err := applyEnvVars(scope, counts); err != nil {
		return nil, err
	}
	applyDefaults(root)
	forgiveMissing := interrupt != nil || active.Interrupts
	if err := validateNArgs(active, scope, counts, forgiveMissing); err != nil {
		return nil, err
	}
	if interrupt != nil {
		return invocationFor(interrupted, interrupt), nil
	}
	return invocationFor(active, nil), nil
}

// resetStates forgets the previous reading of every flag under cmd, so
// that this one starts from nothing named. It writes no variable.
func resetStates(cmd *ir.Command) {
	for _, group := range cmd.FlagGroups {
		for _, f := range group.Flags {
			if f.State != nil {
				f.State.Reset()
			}
		}
	}
	for _, sub := range cmd.Subcommands {
		resetStates(sub)
	}
}

// setSource notes that the command line or the environment named f, in
// the state its declaration owns. It runs for a flag that binds no value
// too, which is what an unbound flag's presence rides on.
func setSource(f *ir.Flag, src ir.Source) {
	if f.State != nil {
		f.State.Source = src
		f.State.Count++
	}
}

// applyDefaults writes the default of every flag under cmd that the
// reading never named. It covers the whole tree rather than the scope
// the line reached, because a flag out of scope holding its default is
// what a program expects of it. The count is the declaration's, so a
// flag mounted twice in one path -- two nodes here, one variable there
// -- is named once for both.
func applyDefaults(cmd *ir.Command) {
	for _, group := range cmd.FlagGroups {
		for _, f := range group.Flags {
			if s := f.State; s != nil && s.Count == 0 && s.SetDefault != nil {
				s.SetDefault()
			}
		}
	}
	for _, sub := range cmd.Subcommands {
		applyDefaults(sub)
	}
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
// validateNArgs sees it satisfied and records on its state that the
// value came from the environment. A flag the command line already set
// is skipped, which is what leaves its recorded source ir.SourceArgs.
//
// scope is the commands dispatched through, beginning at the one Parse was
// called on rather than at the root: a flag an ancestor of that command
// declares was never matchable, so it is not checked here either. Scope
// order, then group order, then declaration order within each command:
// this is deterministic.
func applyEnvVars(scope []*ir.Command, counts map[*ir.Flag]int) error {
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
				setSource(f, ir.SourceEnv)
			}
		}
	}
	return nil
}

// validateNArgs verifies each flag in scope was given as many times as it
// requires. Every flag that became active along the descent is checked, so
// an ancestor's Required flag still binds when a subcommand is invoked.
// forgiveMissing skips the check for too few, which an interrupt excuses;
// too many is still an error.
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
func validateNArgs(active *ir.Command, scope []*ir.Command, counts map[*ir.Flag]int, forgiveMissing bool) error {
	for _, cmd := range scope {
		for _, group := range cmd.FlagGroups {
			for _, f := range group.Flags {
				n := counts[f]
				if !forgiveMissing && f.MinCount > 0 && n < f.MinCount {
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
// Its streams are left nil. Streams are process environment, so the entry
// point holding them fills them in.
func invocationFor(cmd *ir.Command, interrupt *ir.Flag) *ir.Invocation {
	return &ir.Invocation{
		Cmd:       cmd,
		Interrupt: interrupt,
	}
}
