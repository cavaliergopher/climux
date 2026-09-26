package argv

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.hotsrc.dev/climux/ir"
)

// Validate checks the compiled tree rooted at root for the one
// configuration error that is a fact about a decorated name rather than
// about the model: two flags that would answer to the same one. It
// reports every error found in one run.
//
// This is the half of validation that has to ask whatever authority the
// lexer asks. A collision is a collision only once names are spelled --
// two flags collide because their forms are equal, not their names -- so
// checking it anywhere else would be a second opinion on a question this
// package settles. (*ir.Command).Validate covers the rest.
func Validate(root *ir.Command) error {
	return validateTree(root, nil)
}

// claimant records who claimed a name: the command that declared it and
// the flag it reached the name space through. A collision error needs
// both -- which command a name came from, and, when the name was
// generated rather than declared, what generated it, since an author
// cannot see a name that is nowhere in their source.
type claimant struct {
	cmd  *ir.Command
	flag *ir.Flag
}

// validateTree checks c and, recursively, each of its subcommands.
// claimed maps each option spelling claimed by c's ancestors' persistent
// flags to who claimed it. A name must be unique among the flags writable
// at any one position, and a persistent flag is writable everywhere
// beneath its command, so no descendant may reuse its names; a local
// flag's names are free again once the line dispatches. The check runs
// here because a command cannot know its ancestors until the whole tree
// is in view. See docs/adr/flags-are-local-by-default.md.
func validateTree(c *ir.Command, claimed map[string]claimant) error {
	var errs []error
	if err := validateSelf(c, claimed); err != nil {
		errs = append(errs, err)
	}
	if len(c.Subcommands) == 0 {
		return ir.JoinErrors(errs)
	}
	// Descendants see c's names claimed in a copy, so sibling subtrees may
	// still reuse names freely.
	claims := make(map[string]claimant, len(claimed))
	for option, by := range claimed {
		claims[option] = by
	}
	for _, group := range c.FlagGroups {
		if group.Mounted {
			// A mounted group's flags are the registering library's, not
			// this command's, and a command is often mounted somewhere its
			// author did not choose. Claiming them would make a subcommand
			// that mounts the same registry as an ancestor -- the ordinary
			// way two teams both reach DefaultRegistry -- a configuration
			// error.
			continue
		}
		for _, flag := range group.Flags {
			if !flag.Persistent {
				continue
			}
			for option := range flag.ClaimedOptions {
				claims[option] = claimant{cmd: c, flag: flag}
			}
		}
	}
	for _, sub := range c.Subcommands {
		if err := validateTree(sub, claims); err != nil {
			errs = append(errs, err)
		}
	}
	return ir.JoinErrors(errs)
}

// validateSelf checks c's own flags for options claimed twice -- within
// c, or by the ancestors whose claims are passed in. It does not descend
// into subcommands. Whether a name may be declared at all is settled
// while it is still undecorated; see ValidateName.
func validateSelf(c *ir.Command, claimed map[string]claimant) error {
	var errs []error

	claimedHere := make(map[string]*ir.Flag)
	for _, group := range c.FlagGroups {
		for _, flag := range group.Flags {
			// A collision is reported by the colliding option as it is
			// written rather than by the name behind it: it is a fact
			// about a decoration, so the written option is what the
			// reader needs. A positional claims nothing, so this loop
			// never runs for one.
			// Sorted, because these errors are batched and joined: map
			// order would shuffle a multi-collision message between
			// runs.
			for _, option := range slices.Sorted(maps.Keys(flag.ClaimedOptions)) {
				if held, ok := claimedHere[option]; ok {
					if err := collisionError(c, flag, option, held, ""); err != nil {
						errs = append(errs, err)
					}
				}
				if a, ok := claimed[option]; ok {
					if err := collisionError(c, flag, option, a.flag, a.cmd.Name); err != nil {
						errs = append(errs, err)
					}
				}
				claimedHere[option] = flag
			}
		}
	}
	return ir.JoinErrors(errs)
}

// ValidateNames reports every rule the given declared names break under
// this dialect, reading them undecorated, as the program wrote them.
// Lowering calls it before decorating them, which is the only point at
// which the undecorated forms are still in hand. Every violation is
// returned rather than just the first, so a program sees all of them in
// one run.
//
// positional says the names belong to an operand. An operand answers to
// no option, so a second name for one could never be matched -- reported
// rather than ignored, since it is a mistake the program cannot see fail.
// That rule is here rather than with the caller because whether a name
// reaches a flag at all is this dialect's to say.
func ValidateNames(names []string, positional bool) []error {
	var errs []error
	if positional && len(names) > 1 {
		errs = append(errs, errors.New("positional arguments do not support aliases"))
	}
	for _, name := range names {
		if err := validateName(name); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// validateName reports the first rule name breaks on its own terms.
//
// The rules are this dialect's own. "=" reads as the delimiter of an
// attached value and whitespace as an argument break, so a name holding
// either could never be matched; a leading "-" would decorate into
// something the parser reads as two options; POSIX guideline 3 confines a
// one-character name to one alphanumeric character, which is what lets a
// short boolean read "=" as a delimiter.
func validateName(name string) error {
	if name == "" {
		return nil // an empty slot, which climux.Flag.Aliases documents
	}
	switch {
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("flag name must not start with '-': %q", name)
	case strings.ContainsRune(name, '='):
		return fmt.Errorf("flag name must not contain '=': %q", name)
	case strings.ContainsFunc(name, unicode.IsSpace):
		return fmt.Errorf("flag name must not contain whitespace: %q", name)
	case utf8.RuneCountInString(name) == 1 && !isShortName(name):
		return fmt.Errorf("short name must be one character from [A-Za-z0-9]: %q", name)
	}
	return nil
}

// isShortName reports whether s is a legal short name. POSIX guideline 3
// confines one to a single character from the portable character set, and
// the parser leans on that: reading "=" as a delimiter after a short
// boolean costs no ambiguity only because "=" can never be a name.
//
// Measured in characters rather than bytes, so a multi-byte rune is
// rejected for falling outside the set rather than for its length.
func isShortName(s string) bool {
	r, size := utf8.DecodeRuneInString(s)
	if size != len(s) {
		return false
	}
	return ('a' <= r && r <= 'z') ||
		('A' <= r && r <= 'Z') ||
		('0' <= r && r <= '9')
}

// collisionError reports that flag and held both answer to option, or nil
// when that collision only shadows one already reported. ancestor names
// the command that also claims it, or is empty when both flags are on the
// same command.
//
// It names no offender. Which of two flags was declared first is an
// accident of import and mount order, and in a tree composed from several
// teams it is not something either author can see, so the error states
// the collision and leaves the fix to whoever can make it. Ancestry is
// different and is named: it survives any reordering, and it is what
// tells a reader which end to change.
//
// A collision the author cannot find anywhere in their own source is the
// worst kind, so an option one side generated says where it came from --
// including when the other side declared it outright, since it is the
// generated half that is invisible. When both sides generated it from an
// option they also collide on, it is the same mistake twice: two booleans
// that both declare --force both generate --no-force, and only --force,
// the name they actually wrote, is worth reporting.
func collisionError(c *ir.Command, flag *ir.Flag, option string, held *ir.Flag, ancestor string) error {
	source := generatedFrom(option, flag, held)
	if source != "" && flag.Claims(source) && held.Claims(source) {
		return nil
	}
	var from string
	if source != "" {
		from = fmt.Sprintf(" (generated from %s)", source)
	}
	if ancestor != "" {
		return ir.NewConfigErrorf(nil, c, flag,
			"flag declared on both %q and %q: %s%s", ancestor, c.Name, option, from)
	}
	return ir.NewConfigErrorf(nil, c, flag, "flag declared more than once: %s%s", option, from)
}
