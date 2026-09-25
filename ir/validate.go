package ir

import (
	"slices"
	"strings"
)

// validateTree implements (*Command).Validate: it checks c and,
// recursively, each of its subcommands, joining every error found.
//
// Every rule here reads a command or a flag on its own terms, so the
// recursion carries nothing down it. The rules that read a name as the
// command line writes it -- whether two flags collide on one, and what a
// name may contain at all -- live in internal/argv, which owns how a name
// is written.
func validateTree(c *Command) error {
	var errs []error
	if err := validateSelf(c); err != nil {
		errs = append(errs, err)
	}
	for _, sub := range c.Subcommands {
		if err := validateTree(sub); err != nil {
			errs = append(errs, err)
		}
	}
	return JoinErrors(errs)
}

// validateSelf checks c for the configuration errors it can answer on its
// own terms: flag syntax, how positionals follow one another, an interrupt
// command's subcommands, and two of its children answering to one name. It does not descend into
// subcommands.
func validateSelf(c *Command) error {
	var errs []error

	// A subcommand beneath an interrupt would answer in its place without
	// its properties -- a missing required argument reported again, its
	// ancestors' middleware run again -- so "app help topic" would behave
	// unlike "app help". A topic is an argument the handler reads instead.
	if c.Interrupts && len(c.Subcommands) > 0 {
		errs = append(errs, newConfigErrorf(nil, c, nil,
			"an interrupt command declares no subcommands"))
	}

	// Dispatch resolves a name to one command, so two subcommands
	// answering to the same word leave the second unreachable and the
	// tree saying nothing about which was meant. The pairing that makes
	// this worth catching is a command's own children against those a
	// mounted Registry contributed: without the check, a name a program
	// declared can be taken over by a package it merely links in.
	named := make(map[string]bool, len(c.Subcommands))
	for _, sub := range c.Subcommands {
		if named[sub.Name] {
			errs = append(errs, newConfigErrorf(nil, c, nil,
				"more than one subcommand named %q", sub.Name))
		}
		named[sub.Name] = true
	}

	hasUnboundedPositional := false
	// A positional argument is shown by its value name alone, so two of
	// them sharing one is the collision that matters: "missing required
	// argument: FILE" cannot say which was meant. Options are checked
	// elsewhere, against the names they claim, which a positional has
	// none of.
	operands := make(map[string]bool)
	for _, group := range c.FlagGroups {
		for _, flag := range group.Flags {
			if err := validateFlag(flag); err != nil {
				errs = append(errs, err)
			}
			if flag.EndOfOptions && !flag.Positional {
				errs = append(errs, newConfigErrorf(nil, c, flag,
					"only a positional argument may end option processing"))
			}
			if flag.Positional {
				if hasUnboundedPositional {
					errs = append(errs, newConfigErrorf(nil, c, flag, "positional arguments cannot follow unbounded positional arguments"))
				}
				if flag.MaxCount == 0 {
					hasUnboundedPositional = true
				}
				if operands[flag.ValueName] {
					errs = append(errs, newConfigErrorf(nil, c, flag,
						"operand declared more than once: %s", flag))
				}
				operands[flag.ValueName] = true
			}
		}
	}
	return JoinErrors(errs)
}

// validateFlag implements the flag half of (*Command).Validate: it verifies
// that f is configured correctly, independent of the command it belongs
// to, reporting every rule it breaks.
func validateFlag(f *Flag) error {
	var errs []error
	fail := func(format string, a ...any) {
		errs = append(errs, newConfigErrorf(nil, nil, f, format, a...))
	}
	// ClaimedOptions is what matches and NamedOptions is what is shown,
	// so an option missing from it is one the flag advertises and then
	// refuses. That is a bug in the convention that lowered the flag
	// rather than in the program declaring it, which is why it is checked
	// here rather than trusted: a second argv dialect is a replacement
	// for exactly this pairing. What the names are allowed to contain is
	// settled before this, while they are still undecorated.
	for _, option := range f.NamedOptions {
		if option == "" {
			continue // an empty slot, which climux.Flag.Aliases documents
		}
		if !f.Claims(option) {
			fail("option is not matchable: %s", option)
		}
	}
	// A flag may be bound to nothing only when it interrupts, since
	// running is then the whole of what it does. An interrupt fires when
	// the line names it, and a positional argument is never named.
	if f.Handler != nil {
		if f.Positional {
			fail("positional argument must not interrupt")
		}
	} else if f.Value == nil {
		fail("flag must be bound to a value")
	}
	if f.MinCount < 0 {
		fail("minimum count must not be negative: %d", f.MinCount)
	}
	if f.MaxCount < 0 {
		fail("maximum count must not be negative: %d", f.MaxCount)
	}
	// A max of 0 is unbounded, so it is never exceeded by the min.
	if f.MaxCount > 0 && f.MinCount > f.MaxCount {
		fail("minimum count %d exceeds maximum count %d", f.MinCount, f.MaxCount)
	}
	// Defaults are applied without going through Set, so a default outside
	// the choices is never rejected at parse time: the flag would hold, and
	// help would advertise, a value the same program refuses to accept on
	// the command line.
	//
	// Only a flag that takes one value is checked. A repeatable flag
	// accumulates, so its default renders as the whole collection -- "[]"
	// for an empty one -- which is not a value any single choice could
	// match. An empty default is left alone either way: it is how a flag
	// says it has no default.
	if len(f.Choices) > 0 && f.MaxCount == 1 && f.Default != "" &&
		!slices.Contains(f.Choices, f.Default) {
		fail("default %q is not one of: %s", f.Default, strings.Join(f.Choices, ", "))
	}
	return JoinErrors(errs)
}
