package argv

import (
	"strconv"
	"strings"

	"go.hotsrc.dev/climux/ir"
)

const (
	// negatedPrefix is how this dialect writes the negation of a boolean:
	// the "--no-" that getopt_long programs have used for long enough
	// that no other spelling would be read as one.
	negatedPrefix = negatedFragment + "-"

	// negatedFragment is the shortest word that is visibly reaching for a
	// negation, which is when completion starts offering them.
	negatedFragment = "--no"

	// effectNegate is the word this dialect writes into a Claim's Effect
	// for an option it generated as a boolean's opposite. It is this
	// package's own vocabulary: ir stores it and compares it to nothing.
	effectNegate = "negate"
)

// OptionsFor returns both halves of how a flag with the given declared
// names meets a command line: the option each name is shown as, parallel
// to the names so an empty slot stays empty, and every option that
// resolves to the flag, generated ones included. The first is for
// printing and the second for matching, and they part company as soon as
// a dialect writes an option no name asked for.
//
// Both halves are this dialect's to decide, so a caller hands over what
// the program declared and takes back the whole answer rather than
// knowing when to ask. A positional argument is identified by position,
// so it is written as no option and claims none; only a boolean option
// has an opposite worth naming. Nothing outside this package settles
// either question.
//
// Negation is not declared. Every boolean can already be set false with
// "--verbose=false", so "--no-verbose" adds a spelling rather than a
// capability, and a spelling the package guarantees everywhere should not
// be something a program remembers to switch on. Compile calls this once,
// while lowering, so the result is what everything downstream reads;
// nothing rebuilds it.
func OptionsFor(names []string, positional, takesValue, valueless bool) (named []string, claimed map[string]ir.Claim) {
	if positional || len(names) == 0 {
		return nil, nil
	}
	named = make([]string, len(names))
	claimed = make(map[string]ir.Claim, 2*len(names))
	for i, name := range names {
		named[i] = optionOf(name)
		if named[i] == "" {
			continue // an empty slot, which climux.Flag.Aliases documents
		}
		claimed[named[i]] = ir.Claim{Source: named[i]}
	}
	if negatable(positional, takesValue, valueless) {
		for _, option := range named {
			if negation := negationOf(option); negation != "" {
				claimed[negation] = ir.Claim{Source: option, Effect: effectNegate}
			}
		}
	}
	return named, claimed
}

// negationOf returns how the negation of the given option is written, or
// "" when it has none.
//
// Only a long option is negated. Nobody types "-no-v", and a short boolean
// already has "-v=false" for the same job, so generating one would widen
// the collision surface for a spelling that would never be used.
func negationOf(option string) string {
	if !strings.HasPrefix(option, "--") {
		return ""
	}
	return negatedPrefix + strings.TrimPrefix(option, "--")
}

// negatable reports whether this dialect writes a negation for a flag of
// this shape: a boolean option. A flag that takes a value has no opposite
// to name, a positional argument answers to no option at all, and a flag
// bound to no value has nothing that could be set either way, so none of
// the three has one.
func negatable(positional, takesValue, valueless bool) bool {
	return !positional && !takesValue && !valueless
}

// generatedFrom returns the declared option that option was generated
// from, looking at both flags of a collision since either may be the
// generated side, or "" when both declared it outright.
func generatedFrom(option string, a, b *ir.Flag) string {
	for _, f := range []*ir.Flag{a, b} {
		if source := f.ClaimedOptions[option].Source; source != "" && source != option {
			return source
		}
	}
	return ""
}

// resolvedOption is what one token on the command line resolves to: the
// flag it named, and what naming the flag that way means for the value.
// Both are needed and neither follows from the other -- a boolean answers
// to two options that mean opposite things -- so the option table carries
// the pair.
//
// The modifier lives here rather than on the compiled flag because it is
// this dialect's alone: a convention with no negation, or with some other
// modifier, has no use for a field named for this one. A second modifier
// is a second field here and nothing in ir changes.
type resolvedOption struct {
	flag    *ir.Flag
	negated bool
}

// valueFor returns what to set the flag to for the raw value s given
// through this option, applying whatever naming the flag that way means.
// Today that is negation and nothing else; a modifier added later is
// honored here, and the lexer, which calls this without knowing what it
// does, needs no change for one.
//
// A value that does not parse as a boolean passes through unchanged, so
// the flag's own Value is what reports it. A modifier decides what a value
// means, not what counts as a valid one.
func (o resolvedOption) valueFor(s string) string {
	if !o.negated {
		return s
	}
	v, err := strconv.ParseBool(s)
	if err != nil {
		return s
	}
	return strconv.FormatBool(!v)
}

// resolvedOptionsInto adds every option f answers to into table, each
// paired with what naming f that way means. A positional flag adds
// nothing: it claims no option, so it can never be set as if it were one.
//
// Nothing is generated here. The options were settled when the tree was
// compiled, and each records the declared option it came from, so this
// recognizes the ones this dialect wrote by running negationOf forward
// against that source -- never by taking the prefix back off, which would
// be a second rule free to disagree with the first.
func resolvedOptionsInto(table map[string]resolvedOption, f *ir.Flag) {
	for option, claim := range f.ClaimedOptions {
		table[option] = resolvedOption{
			flag:    f,
			negated: negationOf(claim.Source) == option,
		}
	}
}
