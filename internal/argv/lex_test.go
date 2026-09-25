package argv

import (
	"context"
	"fmt"
	"maps"
	"testing"

	"go.hotsrc.dev/climux/ir"
)

// lexStep is a comparable projection of one instruction, so a golden test
// can assert against it without comparing pointers.
type lexStep struct {
	kind     instructionKind
	flag     string // how the flag names itself, e.g. "--name" or "OPERAND"
	value    string
	attached bool
	cmd      string // dispatch, interrupt: the target command's name
}

func summarize(instrs []instruction) []lexStep {
	steps := make([]lexStep, len(instrs))
	for i, instr := range instrs {
		s := lexStep{kind: instr.kind, value: instr.value, attached: instr.attached}
		if instr.flag != nil {
			s.flag = instr.flag.String()
		}
		if instr.cmd != nil {
			s.cmd = instr.cmd.Name
		}
		steps[i] = s
	}
	return steps
}

func errMessages(errs []error) []string {
	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = humanMessage(err)
	}
	return msgs
}

// humanMessage renders an error the way a program reports it, preferring
// String() over Error() so that a lex error reads as the sentence a user
// sees rather than the "climux: " tagged form. Both ir and the root
// package keep their own unexported copy of this; the assertions below
// compare against the sentence, so the test needs one too.
func humanMessage(err error) string {
	if s, ok := err.(fmt.Stringer); ok {
		return s.String()
	}
	return err.Error()
}

func (s lexStep) equal(o lexStep) bool {
	return s.kind == o.kind && s.flag == o.flag && s.value == o.value &&
		s.attached == o.attached && s.cmd == o.cmd
}

func assertLexSteps(t *testing.T, want, got []lexStep) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("instructions = %+v, want %+v", got, want)
	}
	for i := range want {
		if !want[i].equal(got[i]) {
			t.Errorf("instruction[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func assertLexErrs(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("errors = %q, want %q", got, want)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("error[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// newOpt returns an option flag written the way Compile would write one,
// keeping NamedOptions and ClaimedOptions consistent so that a fixture
// matches on the command line exactly as a lowered flag does.
func newOpt(names []string, takesValue bool) *ir.Flag {
	named, claimed := OptionsFor(names, false, takesValue, false)
	return &ir.Flag{
		NamedOptions:   named,
		ClaimedOptions: claimed,
		TakesValue:     takesValue,
	}
}

// opt is newOpt for a boolean flag, which takes no value.
func opt(names ...string) *ir.Flag { return newOpt(names, false) }

// valueOpt is newOpt for a flag that takes a value.
func valueOpt(names ...string) *ir.Flag { return newOpt(names, true) }

// interruptOpt is newOpt for an interrupt, the flag that runs in place of
// the command's handler.
// The handler is never called here -- lex only ever reads whether there
// is one -- but it is what makes the flag an interrupt, so the fixture
// declares one.
func interruptOpt(names ...string) *ir.Flag {
	named, claimed := OptionsFor(names, false, false, true)
	return &ir.Flag{
		NamedOptions:   named,
		ClaimedOptions: claimed,
		Handler: func(ctx context.Context, inv *ir.Invocation) error {
			return nil
		},
	}
}

// lexOptTree returns a command with options only: two bare booleans
// (alpha/bravo), a boolean with a long and short spelling (verbose), a
// string flag with a long and short spelling (name), a value-taking short
// (foxtrot), an int (count), and the interrupt that asks for help, which
// Compile mounts on every root. It declares no positionals or
// subcommands, so an operand is always "extra".
func lexOptTree() *ir.Command {
	return &ir.Command{
		Name: "app",
		FlagGroups: []*ir.FlagGroup{{
			Name: "options",
			Flags: []*ir.Flag{
				opt("alpha", "a"),
				opt("bravo", "b"),
				opt("verbose", "v"),
				valueOpt("name", "n"),
				valueOpt("foxtrot", "f"),
				valueOpt("count", "c"),
				interruptOpt("help", "h"),
			},
		}},
	}
}

// lexNegTree returns a command with the three shapes negation has to tell
// apart: a boolean with a short name, a boolean with a second long name
// and no short one, and a flag that takes a value, which has no opposite
// to spell.
func lexNegTree() *ir.Command {
	return &ir.Command{
		Name: "app",
		FlagGroups: []*ir.FlagGroup{{
			Name: "options",
			Flags: []*ir.Flag{
				opt("verbose", "v"),
				opt("loud", "", "noisy"),
				valueOpt("name", "n"),
			},
		}},
	}
}

// lexPosTree returns a command with two positionals: BAZ, bounded to
// exactly two, and QUX, unbounded.
func lexPosTree() *ir.Command {
	return &ir.Command{
		Name: "app",
		FlagGroups: []*ir.FlagGroup{{
			Name: "options",
			Flags: []*ir.Flag{
				{ValueName: "BAZ", Positional: true, TakesValue: true, MinCount: 2, MaxCount: 2},
				{ValueName: "QUX", Positional: true, TakesValue: true},
			},
		}},
	}
}

// lexSubTree returns a root command with its own "name" flag and one
// subcommand, "sub", with its own "sub-name" flag -- for asserting descent
// and that a root flag stays matchable once the parser has descended.
func lexSubTree() *ir.Command {
	sub := &ir.Command{
		Name: "sub",
		FlagGroups: []*ir.FlagGroup{{
			Flags: []*ir.Flag{valueOpt("sub-name", "s")},
		}},
	}
	root := &ir.Command{
		Name: "app",
		FlagGroups: []*ir.FlagGroup{{
			Flags: []*ir.Flag{valueOpt("name")},
		}},
		Subcommands: []*ir.Command{sub},
	}
	return root
}

// lexHintTree returns a root command with one subcommand, "add", declaring
// a flag the root does not -- for asserting the unrecognized-option hint.
func lexHintTree() *ir.Command {
	add := &ir.Command{
		Name: "add",
		FlagGroups: []*ir.FlagGroup{{
			Flags: []*ir.Flag{opt("tags", "t")},
		}},
	}
	root := &ir.Command{Name: "app", Subcommands: []*ir.Command{add}}
	return root
}

// TestLex is the golden test for lex alone: argv in, the instructions and
// errors it resolves out. See the root package's parser tests for the
// token-shape rules this exercises; TestLex checks that lex records the
// right instruction or error for each, not the rules themselves.
func TestLex(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func() *ir.Command
		args  []string
		want  []lexStep
		errs  []string
	}{
		{
			"LongAttached",
			lexOptTree, []string{"--name=bar"},
			[]lexStep{{kind: instSet, flag: "--name", value: "bar", attached: true}},
			nil,
		},
		{
			"LongDetached",
			lexOptTree, []string{"--name", "bar"},
			[]lexStep{{kind: instSet, flag: "--name", value: "bar"}},
			nil,
		},
		{
			"BoolBareIsTrue",
			lexOptTree, []string{"--verbose"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "true"}},
			nil,
		},
		{
			"BoolAttachedFalse",
			lexOptTree, []string{"--verbose=false"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "false", attached: true}},
			nil,
		},
		{
			// Negation is a spelling of the value, not of the flag: the
			// instruction names --verbose and carries the inverse, so
			// nothing downstream of lex knows a key was negated.
			"NegatedBoolIsFalse",
			lexNegTree, []string{"--no-verbose"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "false"}},
			nil,
		},
		{
			// Both halves negate, so the value is inverted once by the
			// key and set as written by the "=".
			"NegatedBoolAttachedFalseIsTrue",
			lexNegTree, []string{"--no-verbose=false"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "true", attached: true}},
			nil,
		},
		{
			"NegatedBoolAttachedTrueIsFalse",
			lexNegTree, []string{"--no-verbose=1"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "false", attached: true}},
			nil,
		},
		{
			// Not a boolean, so there is nothing to invert. It passes
			// through for the flag's own Value to reject, which is what
			// reports an unparseable boolean everywhere else.
			"NegatedBoolAttachedNonBoolPassesThrough",
			lexNegTree, []string{"--no-verbose=banana"},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "banana", attached: true}},
			nil,
		},
		{
			// Every long spelling of a boolean is negated, aliases
			// included, or --loud and --noisy would disagree about what
			// the same flag can be told.
			"NegatedAlias",
			lexNegTree, []string{"--no-noisy"},
			[]lexStep{{kind: instSet, flag: "--loud", value: "false"}},
			nil,
		},
		{
			// A short name is not negated: -v=false is already the short
			// spelling for false.
			"ShortNameHasNoNegation",
			lexNegTree, []string{"--no-v"},
			nil,
			[]string{"unrecognized option: --no-v"},
		},
		{
			"ValueFlagHasNoNegation",
			lexNegTree, []string{"--no-name", "bar"},
			nil,
			[]string{
				"unrecognized option: --no-name",
				"extra operand: bar",
			},
		},
		{
			"NegativeNumberAttached",
			lexOptTree, []string{"--count=-5"},
			[]lexStep{{kind: instSet, flag: "--count", value: "-5", attached: true}},
			nil,
		},
		{
			"DetachedValueLooksLikeOption",
			lexOptTree, []string{"--foxtrot", "-5"},
			nil,
			[]string{
				"option requires an argument: --foxtrot",
				"unrecognized option: -5",
			},
		},
		{
			"ShortGroupEndsAtValueTaker",
			lexOptTree, []string{"-abfx"},
			[]lexStep{
				{kind: instSet, flag: "--alpha", value: "true"},
				{kind: instSet, flag: "--bravo", value: "true"},
				{kind: instSet, flag: "--foxtrot", value: "x", attached: true},
			},
			nil,
		},
		{
			"ShortGroupFirstNameTakesRemainder",
			lexOptTree, []string{"-fab"},
			[]lexStep{{kind: instSet, flag: "--foxtrot", value: "ab", attached: true}},
			nil,
		},
		{
			"ShortGroupMissingValue",
			lexOptTree, []string{"-abf"},
			[]lexStep{
				{kind: instSet, flag: "--alpha", value: "true"},
				{kind: instSet, flag: "--bravo", value: "true"},
			},
			[]string{"option requires an argument: -f"},
		},
		{
			"ShortBoolEmptyAttached",
			lexOptTree, []string{"-v="},
			[]lexStep{{kind: instSet, flag: "--verbose", value: "", attached: true}},
			nil,
		},
		{
			"ShortValueEmptyAttached",
			lexOptTree, []string{"-n="},
			[]lexStep{{kind: instSet, flag: "--name", value: "", attached: true}},
			nil,
		},
		{
			"TerminatorEndsOptionsByDefault",
			lexOptTree, []string{"--name=x", "--", "-rf"},
			[]lexStep{{kind: instSet, flag: "--name", value: "x", attached: true}},
			[]string{"extra operand: -rf"},
		},
		{
			"HelpAlone",
			lexOptTree, []string{"--help"},
			[]lexStep{{kind: instInterrupt, flag: "--help", cmd: "app"}},
			nil,
		},
		{
			"HelpShortSpelling",
			lexOptTree, []string{"-h"},
			[]lexStep{{kind: instInterrupt, flag: "--help", cmd: "app"}},
			nil,
		},
		{
			"HelpBesideAnEarlierError",
			lexOptTree, []string{"--bogus", "--help"},
			[]lexStep{{kind: instInterrupt, flag: "--help", cmd: "app"}},
			[]string{"unrecognized option: --bogus"},
		},
		{
			"HelpAfterTerminatorIsAnOperand",
			lexOptTree, []string{"--", "-h"},
			nil,
			[]string{"extra operand: -h"},
		},
		{
			"InterruptInAShortCluster",
			lexOptTree, []string{"-vh"},
			[]lexStep{
				{kind: instSet, flag: "--verbose", value: "true"},
				{kind: instInterrupt, flag: "--help", cmd: "app"},
			},
			nil,
		},
		{
			"InterruptLeavesTheClusterReading",
			lexOptTree, []string{"-hv"},
			[]lexStep{
				{kind: instInterrupt, flag: "--help", cmd: "app"},
				{kind: instSet, flag: "--verbose", value: "true"},
			},
			nil,
		},
		{
			"InterruptLeavesTheLineReading",
			lexOptTree, []string{"--help", "--name=x"},
			[]lexStep{
				{kind: instInterrupt, flag: "--help", cmd: "app"},
				{kind: instSet, flag: "--name", value: "x", attached: true},
			},
			nil,
		},
		{
			"InterruptTakesNoAttachedValue",
			lexOptTree, []string{"--help=false"},
			nil,
			[]string{"option takes no argument: --help"},
		},
		{
			"InterruptTakesNoAttachedValueWhenShort",
			lexOptTree, []string{"-h=false"},
			nil,
			[]string{"option takes no argument: -h"},
		},
		{
			"InterruptHasNoNegatedSpelling",
			lexOptTree, []string{"--no-help"},
			nil,
			[]string{"unrecognized option: --no-help"},
		},
		{
			"ExtraOperand",
			lexOptTree, []string{"nope"},
			nil,
			[]string{"extra operand: nope"},
		},
		{
			"UnrecognizedOptionHint",
			lexHintTree, []string{"--tags"},
			nil,
			[]string{`unrecognized option: --tags (defined by subcommand "add")`},
		},
		{
			"Descent",
			lexSubTree, []string{"sub", "--sub-name=z"},
			[]lexStep{
				{kind: instDispatch, cmd: "sub"},
				{kind: instSet, flag: "--sub-name", value: "z", attached: true},
			},
			nil,
		},
		{
			"AncestorFlagStaysMatchableAfterDescent",
			lexSubTree, []string{"sub", "--name=after"},
			[]lexStep{
				{kind: instDispatch, cmd: "sub"},
				{kind: instSet, flag: "--name", value: "after", attached: true},
			},
			nil,
		},
		{
			"PositionalBindingIncludingUnbounded",
			lexPosTree, []string{"a", "b", "c", "d"},
			[]lexStep{
				{kind: instSet, flag: "BAZ", value: "a"},
				{kind: instSet, flag: "BAZ", value: "b"},
				{kind: instSet, flag: "QUX", value: "c"},
				{kind: instSet, flag: "QUX", value: "d"},
			},
			nil,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := tt.build()
			res := lex(root, tt.args)
			assertLexSteps(t, tt.want, summarize(res.instructions))
			assertLexErrs(t, tt.errs, errMessages(res.errs))
		})
	}
}

// TestLexArgIndex asserts that a set instruction names the argv index its
// value came from, for both an attached and a detached value -- what a
// completion engine will need to tell which word on the line it is
// completing.
func TestLexArgIndex(t *testing.T) {
	root := lexOptTree()
	res := lex(root, []string{"--verbose", "--name", "bar"})
	if len(res.errs) != 0 {
		t.Fatalf("unexpected errors: %v", res.errs)
	}
	want := []int{0, 2}
	for i, idx := range want {
		if got := res.instructions[i].argIndex; got != idx {
			t.Errorf("instructions[%d].argIndex = %d, want %d", i, got, idx)
		}
	}
}

// TestLexOpenPositional asserts that lexResult reports the positional flag
// still open to receive a value, or nil once every positional is filled --
// what completion will need to know what a trailing word would bind to.
func TestLexOpenPositional(t *testing.T) {
	root := lexPosTree()

	res := lex(root, nil)
	if res.openPositional == nil || res.openPositional.String() != "BAZ" {
		t.Errorf("openPositional = %v, want BAZ", res.openPositional)
	}

	res = lex(root, []string{"a", "b"})
	if res.openPositional == nil || res.openPositional.String() != "QUX" {
		t.Errorf("openPositional = %v, want QUX (unbounded, never closes)", res.openPositional)
	}
}

// TestLexActiveCommand asserts that lexResult reports the deepest command
// argv descended into, which is what completion will resume lexing from.
func TestLexActiveCommand(t *testing.T) {
	root := lexSubTree()
	res := lex(root, []string{"sub"})
	if res.active == nil || res.active.Name != "sub" {
		t.Errorf("active = %v, want sub", res.active)
	}
}

// TestSplitLongOption covers splitLongOption's contract directly; TestLex
// exercises it end to end through lex.
func TestSplitLongOption(t *testing.T) {
	for _, tt := range []struct {
		arg      string
		name     string
		value    string
		attached bool
	}{
		{"--x", "--x", "", false},
		{"--xVar", "--xVar", "", false}, // long options have no remainder form
		{"--x=Var", "--x", "Var", true},
		{"--x=", "--x", "", true},
		{"--foo=bar=baz", "--foo", "bar=baz", true}, // splits at the first "="
		{"--foo=-5", "--foo", "-5", true},
		{"--=foo", "--=foo", "", false}, // names nothing, so it is not a split
	} {
		t.Run(tt.arg, func(t *testing.T) {
			name, value, attached := splitLongOption(tt.arg)
			if name != tt.name || value != tt.value || attached != tt.attached {
				t.Errorf(
					"expected (%q, %q, %v), got (%q, %q, %v)",
					tt.name, tt.value, tt.attached, name, value, attached,
				)
			}
		})
	}
}

// TestNegationIsRecognizedNotStored asserts the seam the lexer relies on:
// the compiled flag maps each option it answers to back to a Claim naming
// the option it came from, and this dialect works out for itself which of
// those it generated by running its own rule forward against Source --
// never by reading Claim.Effect, which exists for a document to read
// rather than for parsing.
func TestNegationIsRecognizedNotStored(t *testing.T) {
	f := opt("verbose", "v")
	want := map[string]ir.Claim{
		"--verbose":    {Source: "--verbose"},
		"-v":           {Source: "-v"},
		"--no-verbose": {Source: "--verbose", Effect: "negate"},
	}
	if got := f.ClaimedOptions; !maps.Equal(got, want) {
		t.Fatalf("ClaimedOptions = %v, want %v", got, want)
	}

	table := map[string]resolvedOption{}
	resolvedOptionsInto(table, f)
	for option, wantNegated := range map[string]bool{
		"--verbose":    false,
		"-v":           false,
		"--no-verbose": true,
	} {
		o, ok := table[option]
		if !ok {
			t.Errorf("%s does not resolve", option)
			continue
		}
		if o.negated != wantNegated {
			t.Errorf("%s resolved with negated=%v, want %v", option, o.negated, wantNegated)
		}
	}

	// A value-taking flag has no opposite, so it claims only what it names
	// and every claim points at itself.
	name := valueOpt("name", "n")
	if got, want := len(name.ClaimedOptions), 2; got != want {
		t.Errorf("value flag claims %d options, want %d", got, want)
	}
	for option, claim := range name.ClaimedOptions {
		if option != claim.Source {
			t.Errorf("%s was generated from %s, want nothing generated", option, claim.Source)
		}
	}
}
