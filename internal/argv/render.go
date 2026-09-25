package argv

import (
	"strings"
	"unicode/utf8"
)

// optionOf returns the option a flag name is shown as: one character
// takes a single dash and anything longer takes two, which is POSIX
// guideline 3 and the GNU long-option convention between them. The shape
// of the name decides, not the slot it was declared in.
//
// One name yields one option because this dialect accepts one spelling of
// it, so here what is shown and what is accepted happen to be the same
// string. They are still different questions: OptionsFor answers what a
// flag answers to, and a dialect taking both "/x" and "-x" would show one
// of them and accept both.
//
// This is the one place a name gains its decoration, which is what a
// second argv dialect would replace; see
// docs/adr/posix-gnu-argv-dialect.md.
func optionOf(name string) string {
	if name == "" {
		return ""
	}
	if utf8.RuneCountInString(name) == 1 {
		return "-" + name
	}
	return "--" + name
}

// ValueNameFor returns how the name of a flag's value is written where
// the flag is shown to a reader: upper-cased, with dashes as
// underscores, so that "log-level" reads as LOG_LEVEL. That is how a
// POSIX synopsis writes an operand or an option-argument, and it is this
// dialect's answer rather than a universal one -- Go's own flag package
// writes a value's type in lower case, and other conventions bracket it
// as <value>.
//
// name is the flag's own name and valueName what the program asked its
// value be called, empty when it asked for nothing. Which of the two is
// rendered is this dialect's to decide, since a convention free to write
// a value's type instead is free to default differently. A flag that
// takes no value has nothing to name and gets "".
//
// An option's one-character name is not written out, since it was chosen
// to be terse to type and says nothing about the value: "-c C" tells a
// reader less than "-c VALUE" does. Every comparable parser reaches for
// something generic there rather than the letter -- Go's flag package
// writes the value's type, clap writes <VALUE> -- and a program that
// wants better says so with climux.Flag.ValueName, which is honored
// however short it is.
//
// An operand keeps its name however short. It is named rather than
// spelled, so nothing pressed that name to be terse and it was chosen to
// describe the value: a program writing "app A B" means those letters.
func ValueNameFor(name, valueName string, positional, takesValue bool) string {
	if !takesValue {
		return ""
	}
	if valueName == "" {
		if !positional && utf8.RuneCountInString(name) == 1 {
			return genericValueName
		}
		valueName = name
	}
	return strings.ToUpper(strings.ReplaceAll(valueName, "-", "_"))
}

// genericValueName stands for a value the flag's own name cannot describe.
const genericValueName = "VALUE"
