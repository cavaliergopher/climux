package argv

import (
	"slices"
	"strings"

	"go.hotsrc.dev/climux/ir"
)

// Complete resolves shell completion candidates for a command line that is
// still being typed against the compiled command cmd. args is the command
// line so far, excluding the program name and the word currently under the
// cursor; word is that fragment, possibly empty.
//
// Complete is best-effort: a broken or half-typed command line still
// yields whatever can be offered for the position the cursor is in --
// required flags need not be present, and an unrecognized token earlier on
// the line does not stop completion at the position after it. It calls
// Set on every flag named earlier in args, since a value's CompleteFunc
// may need to see them, so it must run in a throwaway process built for
// completion and never beside a program's live state.
func Complete(cmd *ir.Command, args []string, word string) ([]string, ir.CompDirective) {
	res := lex(cmd, args)

	sources := make(map[*ir.Flag]ir.Source)
	for _, instr := range res.instructions {
		if instr.kind != instSet {
			continue
		}
		// Best-effort: a value a CompleteFunc would reject outright is
		// still worth setting, since completion is not validation.
		_ = instr.flag.Set(instr.value)
		// Recorded even when Set failed, for the same reason: what the
		// line named is what a CompleteFunc is completing against.
		sources[instr.flag] = ir.SourceArgs
	}

	// No environment variable is read here, so nothing a CompleteFunc
	// sees reports ir.SourceEnv: completion answers what has been typed.
	inv := invocationFor(res.active, nil, sources)

	cands, dir := completeCandidates(res, res.active.ScopedFlags(), inv, word)
	return finalizeCandidates(cands, word), dir
}

// completeCandidates chooses which of the four candidate rules applies at
// the cursor and returns its result, unfiltered; Complete filters,
// deduplicates and sorts every rule's output the same way, so no rule does
// it for itself.
func completeCandidates(res lexResult, scoped []*ir.Flag, inv *ir.Invocation, word string) ([]string, ir.CompDirective) {
	if res.awaitingValue != nil {
		return completeValue(res.awaitingValue, inv, word)
	}

	if strings.HasPrefix(word, "-") && !res.optionsEnded {
		if key, frag, ok := strings.Cut(word, "="); ok {
			o, ok := optionTable(scoped)[key]
			if !ok {
				return nil, ir.CompNoFileComp
			}
			cands, dir := completeValue(o.flag, inv, frag)
			prefixed := make([]string, len(cands))
			for i, cand := range cands {
				prefixed[i] = key + "=" + cand
			}
			return prefixed, dir
		}
		return offeredOptions(scoped, word), ir.CompNoFileComp
	}

	active := res.active
	if len(active.Subcommands) > 0 {
		return subcommandNames(active), ir.CompNoFileComp
	}
	if res.openPositional != nil {
		return completeValue(res.openPositional, inv, word)
	}
	return nil, ir.CompDefault
}

// completeValue resolves f's own value candidates: its Choices if it
// declared any, since an enumerable list always wins, otherwise its
// CompleteFunc if it declared one, otherwise no candidates with the shell
// left free to fall back to filename completion.
func completeValue(f *ir.Flag, inv *ir.Invocation, word string) ([]string, ir.CompDirective) {
	if len(f.Choices) > 0 {
		return f.Choices, ir.CompNoFileComp
	}
	if f.CompleteFunc != nil {
		return f.CompleteFunc(inv, word)
	}
	return nil, ir.CompDefault
}

// optionTable returns every non-positional flag in scoped, keyed by every
// option they answer to, the same table lexer.enterCommand builds while
// lexing, so an option resolves here exactly as it would resolve on the
// command line.
func optionTable(scoped []*ir.Flag) map[string]resolvedOption {
	table := make(map[string]resolvedOption)
	for _, f := range scoped {
		resolvedOptionsInto(table, f)
	}
	return table
}

// offeredOptions returns the options offered for word: every option --
// "--name" and "-s" -- of every flag in scoped that is neither positional
// nor Hidden.
//
// A generated negation is offered only once word reaches for one. Every
// boolean has one, so offering them always would double the candidate
// list with spellings most users never type; offering them from "--no"
// onward puts them in front of the user who is typing one. They match at
// every point either way -- what a shell offers and what the command line
// accepts are different questions.
func offeredOptions(scoped []*ir.Flag, word string) []string {
	negating := strings.HasPrefix(word, negatedFragment)
	var names []string
	for _, f := range scoped {
		if f.Positional || f.Hidden {
			continue
		}
		for option, claim := range f.ClaimedOptions {
			if !negating && option != claim.Source {
				continue
			}
			names = append(names, option)
		}
	}
	return names
}

// subcommandNames returns the name of every subcommand of active that is
// not Hidden.
func subcommandNames(active *ir.Command) []string {
	var names []string
	for _, sub := range active.Subcommands {
		if sub.Hidden {
			continue
		}
		names = append(names, sub.Name)
	}
	return names
}

// finalizeCandidates applies the one rule every candidate list shares:
// keep what has word as a prefix, drop duplicates, and sort what remains,
// so a shell always sees a stable, minimal reply regardless of which rule
// produced it.
func finalizeCandidates(cands []string, word string) []string {
	seen := make(map[string]bool, len(cands))
	var out []string
	for _, cand := range cands {
		if !strings.HasPrefix(cand, word) || seen[cand] {
			continue
		}
		seen[cand] = true
		out = append(out, cand)
	}
	slices.Sort(out)
	return out
}
