package argv

import (
	"unicode/utf8"

	"go.hotsrc.dev/climux/ir"
)

// terminator ends option processing. Only the first one seen is special;
// see lex's doc comment.
const terminator = "--"

func isShortOption(arg string) bool {
	if len(arg) < 2 {
		return false
	}
	return arg[0] == '-' && arg[1] != '-'
}

func isLongOption(arg string) bool {
	if len(arg) < 3 {
		return false
	}
	return arg[0] == '-' && arg[1] == '-'
}

// isOperand reports whether arg is an operand rather than an option.
// Guideline 14: if it parses as an option, it is one, so the syntax alone
// decides -- a token is an operand only when it looks like neither form.
func isOperand(arg string) bool {
	return !isShortOption(arg) && !isLongOption(arg)
}

// instructionKind names what one instruction asks apply to do.
type instructionKind int

const (
	instSet instructionKind = iota
	instDispatch
	instGiven
	instInterrupt
)

// instruction is one resolved step of argv, produced by lex and applied in
// order by apply. Each names everything apply needs to act on it without
// resolving anything further.
type instruction struct {
	kind instructionKind

	// set, given, interrupt: the flag the instruction names.
	flag     *ir.Flag
	value    string
	attached bool
	argIndex int // index into argv of the token the value came from

	// dispatch, interrupt: the command the instruction concerns -- the
	// one descended into, or the one active when the interrupt was named.
	cmd *ir.Command
}

// lexResult is what lex returns: the instructions and errors it found,
// plus enough of where it stopped for a completion engine to resume from --
// the command active at the end of argv, the positional flag still open to
// receive a value there, whether a final detached option is still waiting
// on a value that argv never supplied, and whether option processing had
// already ended.
type lexResult struct {
	instructions   []instruction
	errs           []error
	active         *ir.Command
	openPositional *ir.Flag

	// awaitingValue is set when the last token in argv was an option that
	// takes a value and no value followed -- "option requires an argument"
	// at the end of the line specifically, not a mid-line miss such as
	// --foxtrot followed by another option, which stays a plain error with
	// nothing for completion to resume from.
	awaitingValue *ir.Flag

	// optionsEnded reports whether the first "--" terminator, if any, had
	// already been seen by the end of argv -- see lexOne's doc comment on
	// what a command line does after seeing it.
	optionsEnded bool
}

// lex resolves argv against root -- the command Parse was called on -- into
// a flat instruction list. It is the schema-aware pass: -abc cannot be
// tokenized without knowing whether -a takes a value, so lex reads root's
// flags and subcommands to decide.
//
// lex must never call Set: a completion engine evaluates a command line
// that may be broken or only half typed, and must be able to do so without
// applying anything to the flags it inspects. The compiled tree lex reads
// is the one apply mutates -- it holds the same pointers -- so this is a
// rule the type system does not enforce on lex's behalf; apply, not lex,
// is where Set is called.
//
// lex never stops at the first error. An unrecognized or malformed token
// consumes only itself and lexing continues in the same command, so a
// command line with several mistakes reports all of them -- though today
// apply only ever surfaces the first, see apply's doc comment.
func lex(root *ir.Command, argv []string) lexResult {
	lx := &lexer{argv: argv}
	lx.enterCommand(root)
	for lx.pos < len(lx.argv) {
		lx.lexOne()
	}
	var openPositional *ir.Flag
	if len(lx.positionals) > 0 {
		openPositional = lx.positionals[0]
	}
	return lexResult{
		instructions:   lx.instructions,
		errs:           lx.errs,
		active:         lx.cmd,
		openPositional: openPositional,
		awaitingValue:  lx.awaitingValue,
		optionsEnded:   lx.optionsEnded,
	}
}

// lexer holds lex's working state. optionsByName accumulates across a
// descent rather than being rebuilt at each command: a name declared by an
// ancestor stays matchable after its subcommand is named, which is what
// lets a parent's flag still bind once the command line has moved past the
// token that named the subcommand. positionals and subcommandsByName, by
// contrast, are rebuilt fresh on every descent, since only the current
// command's own positionals and subcommands are ever legal there.
type lexer struct {
	argv []string
	pos  int // index of the next unread argument in argv

	cmd               *ir.Command
	optionsByName     map[string]resolvedOption
	subcommandsByName map[string]*ir.Command

	positionals []*ir.Flag
	posCount    int // set instructions already queued for positionals[0]

	// posBound reports that a positional of this command has taken a
	// token. From then until every positional is full, a word naming a
	// subcommand is data rather than a subcommand; see lexOperand.
	posBound bool

	optionsEnded bool

	// awaitingValue mirrors lexResult's field of the same name; see there.
	// It can be set at most once: reaching the end of argv is what sets
	// it, and reaching the end of argv is also what ends lex's loop.
	awaitingValue *ir.Flag

	instructions []instruction
	errs         []error
}

// enterCommand descends the lexer into cmd, growing the option table with
// cmd's own flags and replacing the positional and subcommand tables with
// cmd's. A positional flag never enters the option table, so it cannot be
// set as if it were one.
func (lx *lexer) enterCommand(cmd *ir.Command) {
	lx.cmd = cmd
	if lx.optionsByName == nil {
		lx.optionsByName = make(map[string]resolvedOption)
	}
	lx.positionals = nil
	for _, group := range cmd.FlagGroups {
		for _, f := range group.Flags {
			if f.Positional {
				lx.positionals = append(lx.positionals, f)
				continue
			}
			resolvedOptionsInto(lx.optionsByName, f)
		}
	}
	lx.posCount = 0
	lx.posBound = false
	lx.subcommandsByName = make(map[string]*ir.Command)
	for _, sub := range cmd.Subcommands {
		lx.subcommandsByName[sub.Name] = sub
	}
}

// lexOne resolves one token from argv: the terminator, an operand, or an
// option.
//
// Guideline 10 gives "--" two readings and this one takes the first: it
// ends option processing, so every argument after it is an operand however
// many dashes it starts with -- the escape hatch that lets a command take
// an operand named "-rf". It is the user's to write and nothing a program
// declares, so no command reads it differently.
//
// Only the first "--" is special. Once options have ended, a later one is
// an ordinary operand, as is a "-h" that would otherwise ask for help.
// Nothing past the terminator names an option, so nothing past it can
// interrupt either.
func (lx *lexer) lexOne() {
	idx := lx.pos
	tok := lx.argv[idx]
	lx.pos++

	if !lx.optionsEnded {
		if tok == terminator {
			lx.optionsEnded = true
			return
		}
		if !isOperand(tok) {
			lx.lexOption(tok, idx)
			return
		}
	}
	lx.lexOperand(tok, idx)
}

// lexOperand resolves an operand to the next positional flag awaiting one,
// or, when the command takes none, to a subcommand name.
func (lx *lexer) lexOperand(tok string, idx int) {
	// A command's first operand decides between its subcommands and its
	// positionals: a word naming a subcommand dispatches, and any other
	// word binds the first positional. Once one has bound, the command is
	// filling its positionals, so a later word that happens to name a
	// subcommand is data -- "docker compose run" hands run to compose --
	// until every positional is full. Dispatch starts the choice over:
	// the next word is the subcommand's first operand.
	//
	// Ending options does not change any of this. It stops "-" meaning
	// an option, and nothing else.
	if !lx.posBound || len(lx.positionals) == 0 {
		if sub, ok := lx.subcommandsByName[tok]; ok {
			lx.dispatch(sub)
			return
		}
	}

	if len(lx.positionals) > 0 {
		f := lx.positionals[0]
		// An operand names no option, so it resolves to the flag alone
		// and its value is bound as written.
		lx.emitSet(resolvedOption{flag: f}, tok, false, idx)
		lx.posCount++
		lx.posBound = true
		if f.MaxCount > 0 && lx.posCount == f.MaxCount {
			// all done with this positional flag
			lx.positionals = lx.positionals[1:]
			lx.posCount = 0
		}
		return
	}

	if len(lx.cmd.Subcommands) == 0 {
		// This isn't a lookup miss like the two "unrecognized" cases below:
		// the operand is well understood, there is just no positional slot
		// left to take it. See rm(1)'s "extra operand".
		lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, nil, tok,
			"extra operand: %s", tok))
		return
	}
	lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, nil, tok,
		"unrecognized subcommand: %s", tok))
}

// dispatch descends into sub, which every route to a subcommand ends by
// doing.
func (lx *lexer) dispatch(sub *ir.Command) {
	lx.instructions = append(lx.instructions, instruction{kind: instDispatch, cmd: sub})
	lx.enterCommand(sub)
}

// lexOption resolves one option to its option-argument. The two forms are
// spelled differently enough to be worth separate readers.
func (lx *lexer) lexOption(tok string, idx int) {
	if isShortOption(tok) {
		lx.lexShortOptions(tok, idx)
		return
	}
	lx.lexLongOption(tok, idx)
}

// lexLongOption resolves one long option to its option-argument: one
// attached with "=", the next argument for a flag that takes a value, or an
// implicit "true" for a boolean.
func (lx *lexer) lexLongOption(tok string, idx int) {
	name, value, attached := splitLongOption(tok)
	o, ok := lx.optionsByName[name]
	if !ok {
		lx.unrecognizedOption(name)
		return
	}
	// A flag bound to no value is given by name alone; what it does is
	// its effects.
	if o.flag.Value == nil {
		lx.emitUnbound(o.flag, name, attached)
		return
	}

	// An attached value is unambiguous by construction, so it binds
	// whatever it looks like and whatever the flag's type. A boolean takes
	// one here and nowhere else, which is how it is set false.
	if attached {
		lx.emitSet(o, value, true, idx)
		return
	}
	if !o.flag.TakesValue {
		lx.emitSet(o, "true", false, idx)
		return
	}
	lx.lexDetachedValue(o, name, idx)
}

// lexShortOptions resolves one argument's worth of short options.
// Guideline 5: consumption continues while each name is a flag that takes
// no value, and the first that takes one takes the whole remainder of the
// argument as its attached value, so -abfx is -a -b -f x. This is why the
// argument cannot be split before the option table is known.
func (lx *lexer) lexShortOptions(arg string, idx int) {
	for i, r := range arg {
		if i == 0 {
			continue // the delimiter
		}
		// The one place an option is written from something other than a
		// declared name: a rune read out of a cluster.
		name := optionOf(string(r))
		o, ok := lx.optionsByName[name]
		if !ok {
			// A malformed token consumes only itself: the rest of this
			// argument is not guessed at, but later arguments still lex.
			lx.unrecognizedOption(name)
			return
		}
		rest := arg[i+utf8.RuneLen(r):]

		// A flag bound to no value reads nothing, so the names after it in
		// the same argument are read as usual -- unless one was attached
		// with "=", which is malformed and consumes the rest of it.
		if o.flag.Value == nil {
			attached := len(rest) > 0 && rest[0] == '='
			lx.emitUnbound(o.flag, name, attached)
			if attached {
				return
			}
			continue
		}

		if !o.flag.TakesValue {
			// Guideline 5 spends the whole remainder on further names,
			// which would leave a boolean no short spelling for false. A
			// short name can never be "=", so reading one as a delimiter
			// costs no ambiguity; see
			// docs/adr/posix-argument-conventions.md.
			if len(rest) > 0 && rest[0] == '=' {
				lx.emitSet(o, rest[1:], true, idx)
				return
			}
			lx.emitSet(o, "true", false, idx)
			continue
		}

		// This flag takes a value, so the rest of the argument is it and
		// no further name is read. A leading "=" is again the delimiter
		// rather than the value.
		if rest == "" {
			lx.lexDetachedValue(o, name, idx)
			return
		}
		if rest[0] == '=' {
			rest = rest[1:]
		}
		lx.emitSet(o, rest, true, idx)
		return
	}
}

// unrecognizedOption records an option that resolved to nothing. A name
// declared deeper in the tree becomes legal only after its own command is
// named, so when a command below the current one declares it, the message
// says which rather than leaving the user to guess; see
// docs/adr/path-scoped-flag-names.md.
func (lx *lexer) unrecognizedOption(name string) {
	if sub := findDescendantWithFlag(lx.cmd, name); sub != nil {
		lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, nil, name,
			"unrecognized option: %s (defined by subcommand %q)",
			name, sub.Name))
		return
	}
	lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, nil, name,
		"unrecognized option: %s", name))
}

// lexDetachedValue resolves the next argument as f's value. Guideline 14:
// an argument that parses as an option is one, so a detached value may
// never begin with "-". A value that must, such as -5, is given attached
// instead.
//
// A missing or malformed detached value is itself a malformed token, so it
// consumes nothing further: the argument that would have been the value,
// if there is one, is left for lex to resolve on its own next. Only the
// end-of-argv case records awaitingValue -- a completion engine can resume
// there, offering f's value for the word under the cursor, but a mid-line
// miss has a further token lex will still resolve on its own, so there is
// nothing for completion to wait on.
func (lx *lexer) lexDetachedValue(o resolvedOption, name string, idx int) {
	if lx.pos >= len(lx.argv) {
		lx.awaitingValue = o.flag
		lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, o.flag, name,
			"option requires an argument: %s", name))
		return
	}
	if !isOperand(lx.argv[lx.pos]) {
		lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, o.flag, name,
			"option requires an argument: %s", name))
		return
	}
	valIdx := lx.pos
	value := lx.argv[valIdx]
	lx.pos++
	lx.emitSet(o, value, false, valIdx)
}

// emitSet records an instruction binding value to the flag o names,
// reading value the way o's claim says to. What that means is the
// dialect's to decide and this does not ask; the instruction carries the
// result, so nothing downstream of lex knows a modifier was applied --
// and, since this rewrites a string rather than calling Set, lexing a
// half-typed command line still applies nothing.
func (lx *lexer) emitSet(o resolvedOption, value string, attached bool, argIndex int) {
	lx.instructions = append(lx.instructions, instruction{
		kind:     instSet,
		flag:     o.flag,
		value:    o.valueFor(value),
		attached: attached,
		argIndex: argIndex,
	})
	// Option processing ends once a flag that says so has been given --
	// a positional taking its token, or an option such as git's
	// --end-of-options -- which is the author's half of what a user
	// writes as "--": everything after it is an operand.
	if o.flag.EndOfOptions {
		lx.optionsEnded = true
	}
	// A flag that interrupts still binds its value, so its handler can
	// read what it was given; naming it records the interrupt too.
	if o.flag.Handler != nil {
		lx.instructions = append(lx.instructions, instruction{
			kind: instInterrupt, flag: o.flag, cmd: lx.cmd,
		})
	}
}

// emitUnbound records that the option name reached a flag bound to no
// value, and applies its effects. An interrupt runs in place of the
// handler of the command active here: "app remote --help" concerns remote
// and "app --help remote" concerns app, since a flag that takes no value
// names no other command. A flag with no interrupt is still recorded as
// given, so a handler can ask.
//
// Such a flag takes no argument, so one attached to it is a malformed
// token rather than a value to bind, and is reported like any other.
// Nothing about an interrupt ends the lex: the rest of the line is read
// as usual, so "app --version --format=json" still binds its format.
func (lx *lexer) emitUnbound(f *ir.Flag, name string, attached bool) {
	if attached {
		lx.errs = append(lx.errs, ir.NewArgumentErrorf(nil, lx.cmd, f, name,
			"option takes no argument: %s", name))
		return
	}
	if f.Handler != nil {
		lx.instructions = append(lx.instructions, instruction{kind: instInterrupt, flag: f, cmd: lx.cmd})
	} else {
		lx.instructions = append(lx.instructions, instruction{kind: instGiven, flag: f})
	}
	if f.EndOfOptions {
		lx.optionsEnded = true
	}
}

// findDescendantWithFlag returns the first descendant of cmd to answer to
// the option -- a "--name" or "-s" -- searching depth first in
// declaration order, or nil when none does. A name declared below the
// current command is legal only once its own command is named, so
// unrecognizedOption uses this to say where the name would work; see
// docs/adr/path-scoped-flag-names.md.
//
// A hidden command, and its whole subtree, is skipped: it is deliberately
// unadvertised, so the hint must not name it either. The flag stays
// usable; the error just falls back to the plain "unrecognized option"
// message.
func findDescendantWithFlag(cmd *ir.Command, option string) *ir.Command {
	for _, sub := range cmd.Subcommands {
		if sub.Hidden {
			continue
		}
		for _, group := range sub.FlagGroups {
			for _, flag := range group.Flags {
				if flag.Positional {
					continue
				}
				if option != "" && flag.Claims(option) {
					return sub
				}
			}
		}
		if found := findDescendantWithFlag(sub, option); found != nil {
			return found
		}
	}
	return nil
}

// splitLongOption divides a long option argument into its name and any
// option-argument attached to it, reporting whether one was attached at
// all. Guideline 6's attached form for a long option is "--name=value",
// which splits at the first "=".
//
// arg must be a long option; isLongOption decides that.
func splitLongOption(arg string) (name, value string, attached bool) {
	// From index 3, because a long name is at least one character wide and
	// "--=value" therefore names nothing.
	for i := 3; i < len(arg); i++ {
		if arg[i] == '=' {
			return arg[:i], arg[i+1:], true
		}
	}
	return arg, "", false
}
