package climux

import (
	"context"
	"encoding/json"
	"slices"

	"go.hotsrc.dev/climux/desc"
	"go.hotsrc.dev/climux/internal/argv"
	"go.hotsrc.dev/climux/ir"
)

// An Invocation is the result of parsing a command line. It records which
// command the arguments named, what was left for its handler, and the
// streams the handler should read and write.
type Invocation = ir.Invocation

// A Source names where the value a flag holds came from, which is how a
// handler tells a value the operator typed from the default the program
// gave the flag. Ask Invocation.Source for one, or Invocation.IsSet for
// the yes-or-no form.
type Source = ir.Source

const (
	// SourceDefault is a flag that neither the command line nor the
	// environment set, which holds whatever its constructor gave it.
	SourceDefault = ir.SourceDefault

	// SourceEnv is a flag set from the environment variable it declared,
	// which happens only where the command line did not set it. See
	// Flag.EnvVar.
	SourceEnv = ir.SourceEnv

	// SourceArgs is a flag the command line set, which wins over the
	// environment.
	SourceArgs = ir.SourceArgs
)

// A HandlerFunc runs a command. Register one with Command.HandleFunc.
//
// ctx is the context given to Run, so a handler that does anything
// cancelable should honor it.
//
// inv describes how the command was called: which command was named, any
// arguments forwarded past a "--" terminator, and the streams to use. A
// handler should write inv.Stdout and inv.Stderr rather than the process
// streams, so that a caller redirecting the command captures its output.
//
// Returning nil exits 0 and returning an error exits 1, unless the error
// implements ExitCoder. See RunWithArgs.
type HandlerFunc = ir.HandlerFunc

// A Middleware wraps a command's handler in another handler, which runs
// first and decides whether to call the one it wrapped. Declare one with
// Command.Middleware.
//
//	func timing(next climux.HandlerFunc) climux.HandlerFunc {
//	    return func(ctx context.Context, inv *climux.Invocation) error {
//	        start := time.Now()
//	        err := next(ctx, inv)
//	        fmt.Fprintf(inv.Stderr, "%s took %s\n", inv.Cmd.FullName, time.Since(start))
//	        return err
//	    }
//	}
//
// Do the work in the handler returned, not in the wrapper itself: the
// wrapper may be called more than once per run. An interrupt, such as
// --help, runs no wrapper: it runs in place of the handler rather than
// as one.
type Middleware func(HandlerFunc) HandlerFunc

// Command configures a command that users may invoke from the command line.
//
// Programs should not create Command directly and instead use NewCommand to
// construct one.
type Command struct {
	parent      *Command
	name        string
	summary     string
	description string
	hidden      bool
	forwardArgs bool

	// interrupt, if set, is what makes the command an interrupt, the
	// way a Handler makes a flag one: the callback is the marker, and
	// invoking the command runs it in place of any handler, skipping
	// the checks an ordinary command line must pass. There is no
	// chained setter for it -- a program wanting one of its own reaches
	// for InterruptCommand, the same constructor this package's own are
	// built with.
	interrupt HandlerFunc

	// forwardedValueName and forwardedUsage name and explain the
	// arguments the command forwards unparsed; see Command.Forwarded.
	forwardedValueName string
	forwardedUsage     string

	flagGroups  []*FlagGroup
	registries  []*Registry
	subcommands []*Command
	usageFunc   ir.UsageFunc
	handlerFunc HandlerFunc
	middleware  []Middleware

	// completionEnabled records that EnableCompletion was called on this
	// command, so Run consults the shell completion environment variable
	// before doing anything else. See EnableCompletion.
	completionEnabled bool

	// defaultGroup is the implicit "options" flag group that Flags appends
	// to. NewCommand creates it eagerly, so every command carries one from
	// construction; it stays out of help output regardless, since Usage
	// skips a group with no flags in it.
	defaultGroup *FlagGroup
}

// NewCommand returns a new Command with the given name and summary.
//
// summary is the one-line description of the command, shown beside its name
// where a parent lists its subcommands, and beneath the usage line in the
// command's own help message. See Command.Description for the longer prose
// that follows it.
func NewCommand(name, summary string) *Command {
	return (&Command{
		name:    name,
		summary: summary,
	}).Flags()
}

// InterruptCommand returns a Command that ends the program with fn
// before any handlers or middleware run, the way --help and --version
// do, even when the rest of the command line is incomplete or wrong.
//
// Everything after the command's name reaches fn unparsed, as
// Invocation.Forwarded; an interrupt declares no flags and no
// subcommands of its own. VersionCommand and SchemaCommand are two of
// these, ready made.
func InterruptCommand(name, summary string, fn HandlerFunc) *Command {
	c := NewCommand(name, summary)
	c.interrupt = fn
	return c
}

// VersionCommand returns a Command named "version" that prints version,
// alongside the name of the program it is mounted in.
//
// Like HelpFlag, it ends the program before any handlers run. See
// VersionFlag for the same thing spelled as a flag.
//
// Mount it like any other subcommand. Command.VersionCommand is the
// shorthand, and this is the way to mount it somewhere that shorthand
// cannot -- under a subcommand rather than the root, or renamed.
func VersionCommand(version string) *Command {
	return InterruptCommand("version", "Show the version", printVersion(version))
}

// SchemaCommand returns a Command named "schema" that writes a JSON
// description of the program it is mounted in to standard output. The
// name is fixed by convention, so tooling can find it without being
// told.
//
// Like VersionCommand, it ends the program before any handlers run.
//
// Mount it like any other subcommand. Command.SchemaCommand is the
// shorthand, and this is the way to mount it somewhere that shorthand
// cannot -- under a subcommand rather than the root, or renamed.
func SchemaCommand() *Command {
	return InterruptCommand("schema", "Describe this program as JSON",
		func(ctx context.Context, inv *Invocation) error {
			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(desc.NewDocument(inv.Cmd.Root.Describe()))
		})
}

func (c *Command) String() string { return c.name }

// root returns the root of the command tree c belongs to, which is c itself
// if it has no parent, and reports a configuration error if the parent
// links form a cycle instead of reaching a root.
//
// Nothing else walks the source tree: a cycle is representable until
// Compile has rejected it, so every other reader takes its ancestry from
// the compiled tree. See Compile.
func (c *Command) root() (*Command, error) {
	seen := map[*Command]struct{}{c: {}}
	root := c
	for root.parent != nil {
		root = root.parent
		if _, ok := seen[root]; ok {
			return nil, ir.NewConfigErrorf(nil, nil, nil,
				"%q is its own ancestor", root.name)
		}
		seen[root] = struct{}{}
	}
	return root, nil
}

// validate compiles the whole command tree c belongs to and reports only
// whether it is valid, discarding the compiled tree. See Compile.
func (c *Command) validate() error {
	_, err := c.Compile()
	return err
}

// effectiveGroups returns the flag groups the command presents: its own,
// followed by every group of every mounted Registry, in registration
// order. Validation, parsing and Compile all read flags through this
// helper, so a mounted flag behaves exactly like a declared one. The
// result is assembled afresh on each call, never written back into
// flagGroups: that is what keeps Compile pure and a repeated Parse from
// mounting the same groups twice. Everything else a registry carries is
// assembled the same way, in lower.
func (c *Command) effectiveGroups() []*FlagGroup {
	if len(c.registries) == 0 {
		return c.flagGroups
	}
	groups := make([]*FlagGroup, len(c.flagGroups), len(c.flagGroups)+len(c.registries))
	copy(groups, c.flagGroups)
	for _, registry := range c.registries {
		groups = append(groups, registry.groups...)
	}
	return groups
}

// effectiveMiddleware returns the wrappers the command applies to its own
// handler and to every handler beneath it, outermost first: those of every
// mounted Registry, in registration order, followed by the command's own.
//
// A registered wrapper is outside a declared one because a wrapper
// registered beside the flag it reads has to bound the handlers the
// mounting program wrote, or a program declares its way out of the wrapper
// while keeping the flag. Ordering within the result is all that says so:
// chainMiddleware applies a list outermost-first, so the layering needs no
// second pass.
//
// The result is assembled afresh on each call and never written back, for
// the reason effectiveGroups is.
func (c *Command) effectiveMiddleware() []Middleware {
	if len(c.registries) == 0 {
		return c.middleware
	}
	wrappers := make([]Middleware, 0, len(c.middleware)+len(c.registries))
	for _, registry := range c.registries {
		wrappers = append(wrappers, registry.middleware...)
	}
	return append(wrappers, c.middleware...)
}

// Compile validates the whole command tree c belongs to and returns c in
// its compiled ir.Command form, with its ancestry, inherited flags and
// resolved streams filled in.
//
// Reach for it to check a tree for configuration errors before running it,
// or to walk or marshal a program's whole command line surface. Errors
// anywhere in the tree are reported, not only those in c.
//
// It mutates nothing, so it is safe to call at any time.
func (c *Command) Compile() (*ir.Command, error) {
	root, err := c.root()
	if err != nil {
		// The tree has no root to lower from, so this error stands alone
		// rather than joining the batch the rest of the run collects.
		return nil, err
	}
	nodeMap := make(map[*Command]*ir.Command)
	var errs []error
	rootNode := root.lower(nil, nil, nodeMap, &errs)
	if err := rootNode.Validate(); err != nil {
		errs = append(errs, err)
	}
	// Whether two flags collide is a question about the options they are
	// written as rather than about the model, so it is argv's to answer;
	// see internal/argv.Validate. What a name may be in the first place
	// is argv's too, and was settled during lowering, while the names
	// were still undecorated.
	if err := argv.Validate(rootNode); err != nil {
		errs = append(errs, err)
	}
	if err := ir.JoinErrors(errs); err != nil {
		return nil, err
	}
	return nodeMap[c], nil
}

// lower builds the compiled ir.Command for c and, recursively, each of its
// subcommands, copying across every
// field the ir type keeps, including those inherited from an ancestor so
// nothing about the compiled tree has to walk itself again: FullName,
// computed from parent's own FullName plus c's name, and the three
// streams and the usage renderer, each c's own where it named one and
// the parent's otherwise, and Ancestry, the parent's with c appended.
// Inheritance reads the node being built rather than the source tree's
// parent links, which are not to be trusted until Compile has checked
// them.
//
// Handler is assembled rather than copied: c's own handler is wrapped in
// the middleware c declared and in everything it inherited, which arrives
// as inherited because it is scaffolding for building Handler rather than
// anything a compiled command carries. A command that declared no handler
// gets missingSubcommand's instead, so Handler is never nil and nothing
// downstream needs to know either mechanism exists. An interrupt wraps in
// neither: see where middleware is applied below.
//
// It records the node for c in nodeMap so Compile can look up the node
// for any source *Command after lowering from the root.
//
// It also collects into errs the three configuration checks that cannot
// move onto the compiled tree, because they depend on the source tree's
// own bookkeeping rather than on anything a lowered node carries:
//
// Whether any middleware c applies is nil, which cannot be applied to a
// handler and which chainMiddleware therefore skips. Composing leaves
// nothing on the node to check afterwards, so it is checked here, where
// it is still a list of what one command contributes.
//
// Whether a subcommand's parent is the one lowering expects: the command
// about to claim it as a child, or no parent at all for a command a
// mounted registry contributed. See Subcommands for why -- a command a
// library exports may be handed to more than one parent, and only the
// source *Command remembers which one Subcommands actually accepted.
//
// Whether a subcommand has already been lowered, which means the
// subcommand links lead back into the tree above. nodeMap is the record
// of what has been visited, so the descent stops there rather than
// building nodes forever. Note that a cycle here need not be one in the
// parent links Compile checked: Subcommands leaves an owned command's
// parent alone, so a command can be mounted below its own ancestor
// without any parent link changing.
func (c *Command) lower(parent *ir.Command, inherited Middleware, nodeMap map[*Command]*ir.Command, errs *[]error) *ir.Command {
	fullName := c.name
	if parent != nil {
		fullName = parent.FullName + " " + c.name
	}
	node := &ir.Command{
		Name:               c.name,
		Summary:            c.summary,
		Description:        c.description,
		Hidden:             c.hidden,
		ForwardArgs:        c.forwardArgs,
		Interrupt:          c.interrupt,
		ForwardedValueName: argv.ForwardedValueNameFor(c.forwardedValueName),
		ForwardedUsage:     c.forwardedUsage,
		FullName:           fullName,
		Handler:            c.handlerFunc,
		UsageFunc:          c.usageFunc,
	}
	node.Root = node
	node.Ancestry = []*ir.Command{node}
	if parent != nil {
		node.Root = parent.Root
		// Cloned rather than appended in place, or sibling subcommands
		// would share a backing array and overwrite each other.
		node.Ancestry = append(slices.Clone(parent.Ancestry), node)
	}

	// What c named for itself is above; what it left unnamed is inherited
	// here. Each field resolves on its own, so naming one leaves the
	// others inherited, and a parent's are already resolved, so
	// inheriting ends the search.
	if parent != nil {
		if node.UsageFunc == nil {
			node.UsageFunc = parent.UsageFunc
		}
	}
	// Middleware composes where the fields above fall back: what this
	// command applies runs inside every wrapper its ancestors applied,
	// rather than replacing them.
	wrappers := c.effectiveMiddleware()
	for _, mw := range wrappers {
		if mw == nil {
			*errs = append(*errs, ir.NewConfigErrorf(nil, node, nil,
				"middleware must not be nil"))
		}
	}
	middleware := chainMiddleware(inherited, wrappers)

	// The handler is assembled here rather than at dispatch, so the
	// compiled command carries the whole of what it does and nothing
	// downstream has to know that middleware exists. The fallback is not
	// wrapped: there is no handler for a wrapper to wrap, and a command
	// that only groups subcommands must not run its ancestors' wrappers.
	// An interrupt is not wrapped either, own or inherited, the way an
	// interrupt flag's Handler never is: middleware is written against a
	// command line that parsed, and an interrupt answers one that may
	// not have.
	if node.Handler == nil {
		node.Handler = missingSubcommand(node)
	} else if middleware != nil && c.interrupt == nil {
		node.Handler = middleware(node.Handler)
	}
	if c.interrupt != nil && c.handlerFunc != nil {
		*errs = append(*errs, ir.NewConfigErrorf(nil, node, nil,
			"a command is an interrupt or has a handler, not both"))
	}

	// The root has no parent to inherit from, so a stream nobody named is
	// the process's. UsageFunc has no default to fall back to here: nil
	// means the help renderer chooses one when it prints.
	nodeMap[c] = node

	// Own groups first, then every mounted registry's, matching the order
	// effectiveGroups reports and marking which is which -- validation
	// needs to tell a declared flag from a mounted one.
	for _, group := range c.flagGroups {
		node.FlagGroups = append(node.FlagGroups, group.lower(false, errs))
	}
	for _, registry := range c.registries {
		for _, group := range registry.groups {
			node.FlagGroups = append(node.FlagGroups, group.lower(true, errs))
		}
	}
	// Own subcommands first, then every mounted registry's, ordered as
	// the flag groups above are. The two differ only in what the child
	// is expected to name as its parent: Subcommands parents a command
	// it accepts, while a registry is not a node and parents nothing, so
	// a registered command has to arrive unparented -- one already
	// mounted in a tree of its own would be lowered under two parents.
	lowerSub := func(sub *Command, wantParent *Command) {
		if prev, ok := nodeMap[sub]; ok {
			*errs = append(*errs, ir.NewConfigErrorf(nil, node, nil,
				"%q is already mounted at %q", sub.name, prev.FullName))
			return
		}
		// Subcommands leaves an already-parented command's parent alone
		// rather than stealing it, so the mismatch is still visible here
		// to report. Either way the parent that is not wanted is the one
		// worth naming, and it is never nil: a declared subcommand was
		// given c by Subcommands, and a registered one is only wrong
		// here for having a parent at all.
		if sub.parent != wantParent {
			*errs = append(*errs, ir.NewConfigErrorf(nil, node, nil,
				"%q is already a subcommand of %q", sub.name, sub.parent.name))
		}
		node.Subcommands = append(node.Subcommands, sub.lower(node, middleware, nodeMap, errs))
	}
	for _, sub := range c.subcommands {
		lowerSub(sub, c)
	}
	for _, registry := range c.registries {
		for _, sub := range registry.subcommands {
			lowerSub(sub, nil)
		}
	}
	return node
}

// chainMiddleware composes outer, the middleware a command inherits from
// its parent, with own, the middleware that command applies -- see
// Command.effectiveMiddleware -- into the single Middleware the compiled
// command carries.
//
// Applying the result wraps a handler so that outer runs first, then
// own's entries in the order given, then the handler itself, each
// resuming in reverse as the call returns. The outermost wrapper is
// therefore the one applied highest in the tree, and earliest in own.
//
// A command that applies nothing inherits outer unchanged, so a path with
// no middleware at all composes to nil and costs no call.
//
// A nil entry in own is skipped rather than applied. Lowering reports one
// as a configuration error and then has to finish, so that the rest of
// the tree's errors are collected in the same run; the tree it composes
// meanwhile is never dispatched.
func chainMiddleware(outer Middleware, own []Middleware) Middleware {
	if len(own) == 0 {
		return outer
	}
	return func(next HandlerFunc) HandlerFunc {
		// Built inside out, which reads backwards from the order above:
		// the wrapper applied last ends up outermost, and so runs first.
		for _, mw := range slices.Backward(own) {
			if mw == nil {
				continue
			}
			next = mw(next)
		}
		if outer != nil {
			next = outer(next)
		}
		return next
	}
}

// missingSubcommand returns the handler lowering gives a command that
// declared none of its own. Such a command exists only to group its
// subcommands, so naming it alone is a usage error rather than a request
// for help, and reporting that is the whole of what it does.
//
// It names cmd rather than the invocation's command so that the error
// names the command that lacked the handler, whoever ran it.
func missingSubcommand(cmd *ir.Command) HandlerFunc {
	return func(ctx context.Context, inv *Invocation) error {
		return ir.NewArgumentErrorf(nil, cmd, nil, "", "missing subcommand")
	}
}

// Description specifies the prose printed at the end of this command's help
// message, after its flags and subcommands. It carries the detail that does
// not fit the one-line summary given to NewCommand.
func (c *Command) Description(s string) *Command {
	c.description = s
	return c
}

// HandleFunc registers the handler for the command. If no handler is
// specified and the command is invoked, Run reports an argument error
// followed by the command's usage and exits with the usage error code.
//
// The handler runs inside whatever middleware the command and its
// ancestors declared, Compile having wrapped it while lowering the tree;
// see Command.Middleware.
func (c *Command) HandleFunc(handler HandlerFunc) *Command {
	c.handlerFunc = handler
	return c
}

// Middleware wraps the handler of this command, and of every command
// beneath it, in each of the given wrappers -- an authorization check, a
// timing trace, a resource opened and closed -- written once instead of at
// the top of every handler.
//
//	var App = climux.NewCommand("myapp", "Do things").
//	    Middleware(authorize, trace).
//	    Subcommands(GetCommand, DeleteCommand)
//
// The outermost wrapper is the one declared highest in the tree, and
// within one command they run in the order given here. Repeated calls
// append.
//
// A wrapper runs only around a handler, and only once the command line has
// parsed, so it may read the flags the user set. Returning an error
// without calling the handler refuses the invocation. Neither an interrupt
// such as --help nor a command that only groups subcommands runs a handler,
// so neither runs a wrapper. See Middleware.
func (c *Command) Middleware(mw ...Middleware) *Command {
	c.middleware = append(c.middleware, mw...)
	return c
}

// Hidden hides the command from all help messages but still allows the command
// to be invoked on the command line.
func (c *Command) Hidden() *Command {
	c.hidden = true
	return c
}

// Flags appends command line flags to the implicit "options" flag group
// every command carries from construction.
func (c *Command) Flags(flags ...*Flag) *Command {
	if c.defaultGroup == nil {
		c.defaultGroup = &FlagGroup{name: "options", title: "Options"}
		c.flagGroups = append(c.flagGroups, c.defaultGroup)
	}
	c.defaultGroup.flags = append(c.defaultGroup.flags, flags...)
	return c
}

// HelpFlag adds the flag that prints this command's help message. Given
// no names it answers to "--help" and "-h".
//
//	var App = climux.NewCommand("myapp", "My application").HelpFlag()
//
// Call it on the root: every command below answers to the flag too, and
// each prints its own help. It is shorthand for adding HelpFlag with
// Flags, which is the way to hide it, or to name it something else
// entirely.
func (c *Command) HelpFlag(names ...string) *Command {
	return c.Flags(HelpFlag(names...))
}

// VersionFlag adds the flag that prints version, alongside the name of
// the program it is mounted in. Given no names it answers to "--version".
//
//	var App = climux.NewCommand("orbital", "").VersionFlag(version)
//
// It is shorthand for adding VersionFlag with Flags, which is the way
// to put it in a group of its own, or to hide it.
func (c *Command) VersionFlag(version string, names ...string) *Command {
	return c.Flags(VersionFlag(version, names...))
}

// VersionCommand adds a subcommand named "version" that prints version,
// alongside the name of the program it is mounted in.
//
// Like HelpFlag, it ends the program before any handlers run. See
// VersionFlag for the same thing spelled as a flag.
func (c *Command) VersionCommand(version string) *Command {
	return c.Subcommands(VersionCommand(version))
}

// SchemaCommand adds a subcommand named "schema" that writes a JSON
// description of the program to standard output. Like VersionCommand,
// it ends the program before any handlers run.
func (c *Command) SchemaCommand() *Command {
	return c.Subcommands(SchemaCommand())
}

// FlagGroups adds groups of command line flags, built with NewFlagGroup or
// FromFlagSet, to this command, showing each under its own heading in help
// messages.
func (c *Command) FlagGroups(groups ...*FlagGroup) *Command {
	c.flagGroups = append(c.flagGroups, groups...)
	return c
}

// Mount adds everything registered in each of the given registries to this
// command: their flag groups after the command's own, their subcommands
// after the ones it declared, and their middleware around both. Mount
// DefaultRegistry to pick up everything the program's libraries
// registered:
//
//	var App = climux.NewCommand("myapp", "").Mount(climux.DefaultRegistry)
//
// Nothing limits a program to one registry. An organization keeping
// several -- one per platform team, say -- mounts on each command the
// ones that command should carry, and a registry mounted on a subcommand
// reaches that subtree alone.
//
// A contribution registered after this call is still picked up. Mounted
// flags behave exactly like the command's own, each group under its own
// heading in help messages, and mounted subcommands like the command's
// own children.
func (c *Command) Mount(registries ...*Registry) *Command {
	c.registries = append(c.registries, registries...)
	return c
}

// Subcommands adds subcommands to this command and sets their parent to
// this command, unless a command given here already has one -- typically
// a command a library exports that another tree has already mounted -- in
// which case its existing parent is left alone and validation reports the
// mismatch; see lower.
//
// For a subcommand a library contributes to whichever programs mount it,
// rather than one this command names, see Registry.Subcommands.
func (c *Command) Subcommands(cmds ...*Command) *Command {
	c.subcommands = append(c.subcommands, cmds...)
	for _, cmd := range cmds {
		if cmd.parent == nil {
			cmd.parent = c
		}
	}
	return c
}

// UsageFunc specifies a custom renderer for this command's help messages,
// in place of the default.
func (c *Command) UsageFunc(fn ir.UsageFunc) *Command {
	c.usageFunc = fn
	return c
}

// EnableCompletion opts this command into shell completion. Call it on the
// command Run is given, typically the root, since the environment variable
// it enables is named from that command.
//
// The variable is the command's name, uppercased, with every rune that is
// not a letter or digit replaced by "_", and "_COMPLETE" appended: "myapp"
// answers to MYAPP_COMPLETE. A user enables completion in their shell with
//
//	source <(MYAPP_COMPLETE=bash_source myapp 2>/dev/null)
//
// Run is otherwise unchanged, including when the variable is unset.
func (c *Command) EnableCompletion() *Command {
	c.completionEnabled = true
	return c
}

// ForwardArgs specifies that a "--" on the command line ends option
// processing, and that everything after it reaches the handler unparsed as
// Invocation.Forwarded rather than binding to positional flags.
//
// This is for a command that hands arguments on to something else, such as
// a subprocess. Without it, "--" has no special meaning.
func (c *Command) ForwardArgs() *Command {
	c.forwardArgs = true
	return c
}

// Forwarded names the arguments the command forwards to its handler
// unparsed, for help and for the machine-readable description: what an
// interrupt command takes after its name, or what a command that set
// ForwardArgs takes after the "--" terminator.
//
//	InterruptCommand("schema", summary, fn).
//	    Forwarded("command", "Command to describe")
//
// valueName is shown where the forwarded arguments are, the way a
// positional argument's name is shown, and usage explains them beneath
// the usage line. Naming forwarded arguments on a command that forwards
// nothing is a configuration error.
func (c *Command) Forwarded(valueName, usage string) *Command {
	c.forwardedValueName = valueName
	c.forwardedUsage = usage
	return c
}
