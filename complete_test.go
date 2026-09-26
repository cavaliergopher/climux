package climux

import (
	"testing"

	"go.hotsrc.dev/climux/ir"
)

// compRootTree returns a command with a mix of flag kinds for the
// completion engine to exercise: a bare boolean, a hidden boolean, a
// string with Choices, and a string with neither Choices nor a
// CompleteFunc -- what a value falls back to when nothing can complete it.
// It also declares two subcommands, one hidden, and mounts a further flag
// on the visible one so descent can be asserted. Only the boolean is
// persistent, so descent drops the root's other flags.
func compRootTree() *Command {
	add := NewCommand("add", "").Flags(
		String(new(string), "tags", "").Aliases("t"),
	)
	hiddenSub := NewCommand("hidden-sub", "").Hidden()
	return NewCommand("app", "").
		HelpFlag().
		Flags(
			Bool(new(bool), "verbose", "").Aliases("v").Persistent(),
			Bool(new(bool), "extra", "").Aliases("x").Hidden(),
			String(new(string), "env", "").Aliases("e").
				Choices("dev", "staging", "prod"),
			String(new(string), "name", "").Aliases("n"),
		).
		Subcommands(add, hiddenSub)
}

// compPosSlotsTree returns a command with two positionals, so completion
// after the first is exhausted resumes on the second: BAZ, bounded to
// exactly two, and QUX, unbounded. Each declares its own Choices so a test
// can tell which slot answered.
func compPosSlotsTree() *Command {
	return NewCommand("app", "").Flags(
		Strings(new([]string), "baz", "").Positional().NArgs(2, 2).
			Choices("b1", "b2"),
		Strings(new([]string), "qux", "").Positional().NArgs(0, 0).
			Choices("q1", "q2"),
	)
}

// compOptionsEndedTree returns a command with one positional whose choices
// include a dash-prefixed value, so a test can tell whether a word after
// "--" was read as a flag prefix or as an ordinary operand.
func compOptionsEndedTree() *Command {
	return NewCommand("app", "").Flags(
		String(new(string), "file", "").Positional().
			Choices("-rf", "normal"),
	)
}

func TestComplete(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func() *Command
		args  []string
		word  string
		want  []string
		dir   ir.CompDirective
	}{
		{
			"OptionsOfferedHiddenExcludedHelpPresent",
			compRootTree, nil, "-",
			[]string{"--env", "--help", "--name", "--verbose", "-e", "-h", "-n", "-v"},
			ir.CompNoFileComp,
		},
		{
			"FlagPrefixFilteringHiddenStillExcluded",
			compRootTree, nil, "--e",
			[]string{"--env"},
			ir.CompNoFileComp,
		},
		{
			// Every boolean has a negation, so offering them beside the
			// forms would double the list for a spelling most users never
			// type. They arrive once the word is visibly reaching for one.
			"NegationsOfferedOnceTheWordReachesForOne",
			compRootTree, nil, "--no",
			[]string{"--no-verbose"},
			ir.CompNoFileComp,
		},
		{
			"NegationsNotOfferedBeforeThat",
			compRootTree, nil, "--n",
			[]string{"--name"},
			ir.CompNoFileComp,
		},
		{
			"SubcommandCompletionHiddenExcluded",
			compRootTree, nil, "",
			[]string{"add"},
			ir.CompNoFileComp,
		},
		{
			"OnlyPersistentAncestorFlagsOfferedAfterDescent",
			compRootTree, []string{"add"}, "-",
			[]string{"--help", "--tags", "--verbose", "-h", "-t", "-v"},
			ir.CompNoFileComp,
		},
		{
			"ChoicesCompleteADetachedValue",
			compRootTree, []string{"--env"}, "",
			[]string{"dev", "prod", "staging"},
			ir.CompNoFileComp,
		},
		{
			"ChoicesCompleteAnAttachedValue",
			compRootTree, nil, "--env=st",
			[]string{"--env=staging"},
			ir.CompNoFileComp,
		},
		{
			"AwaitingValueWithNoCompleterOffersNothing",
			compRootTree, []string{"--name"}, "",
			nil,
			ir.CompDefault,
		},
		{
			"ABrokenLineStillCompletesAfterIt",
			compRootTree, []string{"--bogus"}, "",
			[]string{"add"},
			ir.CompNoFileComp,
		},
		{
			"OptionsEndedReadsADashAsAnOperand",
			compOptionsEndedTree, []string{"--"}, "-",
			[]string{"-rf"},
			ir.CompNoFileComp,
		},
		{
			"FirstPositionalSlot",
			compPosSlotsTree, nil, "",
			[]string{"b1", "b2"},
			ir.CompNoFileComp,
		},
		{
			"SlotAdvancesOnceNArgsIsExhausted",
			compPosSlotsTree, []string{"a", "b"}, "",
			[]string{"q1", "q2"},
			ir.CompNoFileComp,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cands, dir := Complete(tt.build(), tt.args, tt.word)
			assertStrings(t, tt.want, cands)
			if got, want := dir, tt.dir; got != want {
				t.Errorf("directive = %v, want %v", got, want)
			}
		})
	}
}

// TestCompleteEmptyArgs asserts that completing an entirely empty command
// line does not panic and, for a command with no subcommands or
// positionals to offer, reports no candidates and lets the shell fall
// back to filename completion.
func TestCompleteEmptyArgs(t *testing.T) {
	cmd := NewCommand("app", "").Flags(
		Bool(new(bool), "verbose", "").Aliases("v"),
	)
	cands, dir := Complete(cmd, nil, "")
	assertStrings(t, nil, cands)
	if got, want := dir, ir.CompDefault; got != want {
		t.Errorf("directive = %v, want %v", got, want)
	}
}

// TestCompleteFuncSeesEarlierFlag asserts that a positional's CompleteFunc
// is given an Invocation on which a flag named earlier on the line
// already holds its value -- the git checkout <ref> case the callback
// signature is built around.
func TestCompleteFuncSeesEarlierFlag(t *testing.T) {
	var region string
	var gotInv *Invocation
	var sawRegion string
	fn := func(inv *Invocation, word string) ([]string, ir.CompDirective) {
		gotInv = inv
		sawRegion = region
		return []string{"i-2", "i-1"}, ir.CompNoFileComp
	}
	cmd := NewCommand("app", "").Flags(
		String(&region, "region", "").Aliases("r"),
		String(new(string), "instance", "").Positional().Complete(fn),
	)

	cands, dir := Complete(cmd, []string{"--region", "us-east"}, "i-")

	if gotInv == nil {
		t.Fatal("CompleteFunc was not called")
	}
	if got, want := gotInv.Cmd.String(), cmd.String(); got != want {
		t.Errorf("inv.Cmd = %v, want %v", got, want)
	}
	if got, want := sawRegion, "us-east"; got != want {
		t.Errorf("region seen by CompleteFunc = %q, want %q", got, want)
	}
	assertStrings(t, []string{"i-1", "i-2"}, cands)
	if got, want := dir, ir.CompNoFileComp; got != want {
		t.Errorf("directive = %v, want %v", got, want)
	}
}

// TestCompleteSource covers a CompleteFunc reading where an earlier flag
// on the line got its value. Completion reads argv and no environment
// variable, so what it records is what has been typed so far.
func TestCompleteSource(t *testing.T) {
	t.Setenv("APP_REPO", "from-env")
	var seen Source
	cmd := NewCommand("app", "").Flags(
		String(new(string), "repo", "").Env("APP_REPO"),
		String(new(string), "branch", "").
			Complete(func(inv *ir.Invocation, word string) ([]string, ir.CompDirective) {
				seen = inv.Source("repo")
				return []string{"main"}, ir.CompNoFileComp
			}),
	)

	Complete(cmd, []string{"--repo", "origin", "--branch"}, "")
	if got, want := seen, SourceArgs; got != want {
		t.Errorf("Source(%q) = %v, want %v", "repo", got, want)
	}

	seen = SourceArgs
	Complete(cmd, []string{"--branch"}, "")
	if got, want := seen, SourceDefault; got != want {
		t.Errorf("Source(%q) = %v, want %v", "repo", got, want)
	}
}
