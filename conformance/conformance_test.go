package conformance_test

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.hotsrc.dev/climux"
	"go.hotsrc.dev/climux/ir"
)

// features is every feature the suite covers, in the order the report
// lists them.
var features = []Feature{
	opaqueArgs,
	endOfOptions,
	persistentFlags,
	optionalValue,
	alias,
	externalSubcommand,
	envRouting,
}

// tools orders the report's columns.
var tools = []string{"docker", "git", "gh", "cargo", "kubectl", "aws"}

// A Feature is one mechanism the audit found, with every command line
// that shows a tool relying on it.
type Feature struct {
	Slug    string
	Summary string
	Cases   []Case
}

// A Case is one real command line and what the tool makes of it.
type Case struct {
	// Argv is the command line as typed at a shell, program name
	// included. Leading NAME=value words set the environment.
	Argv string

	// State is how far climux gets with Argv, and is Conformant unless
	// marked otherwise.
	State State

	// Source cites where the tool implements the behavior.
	Source string

	// DependsOn names other features the case relies on. The report shows
	// them; the runner ignores them.
	DependsOn []string

	// Spec is what the real tool makes of Argv.
	Spec Outcome

	// Defect is what climux makes of Argv today, for a KnownBad case. It
	// is pinned so that any change in behavior is seen.
	Defect *Outcome

	// Build returns a model of just enough of the tool to read Argv, for
	// every case but a NotImplemented one. It is called once per run,
	// since a tree reads one command line.
	Build func() *climux.Command
}

// A State is how far climux gets with a case's command line.
type State int

const (
	// Conformant cases must produce Spec.
	Conformant State = iota

	// KnownBad cases have a model climux can declare today, and must
	// still produce Defect rather than Spec.
	KnownBad

	// NotImplemented cases need something climux cannot declare yet, so
	// they have no model and are skipped.
	NotImplemented
)

func (s State) String() string {
	switch s {
	case Conformant:
		return "conformant"
	case KnownBad:
		return "known-bad"
	case NotImplemented:
		return "not-implemented"
	}
	return fmt.Sprintf("State(%d)", int(s))
}

// An Outcome is what reading a command line meant, in terms that do not
// depend on how climux exposes it.
type Outcome struct {
	Cmd       string   // full name of the command reached
	Flags     Flags    // flags the command line or environment set
	Interrupt string   // name of the interrupt given, if any
	Err       *Failure // non-nil if the line was rejected
}

// Flags maps a flag's declared name to what it was set to.
type Flags map[string]Bound

// A Bound is the value a flag holds and where it came from. Values have
// the type the flag was declared with: bool, int, string, []string. A
// flag that binds no value holds true once given.
type Bound struct {
	Value  any
	Source climux.Source
}

// A Failure is a command line the tool rejects. Arg is the argument it
// blames, if any.
type Failure struct {
	Arg string
}

// arg is a value set on the command line.
func arg(v any) Bound { return Bound{v, climux.SourceArgs} }

// env is a value set by an environment variable.
func env(v any) Bound { return Bound{v, climux.SourceEnv} }

// given is a flag that binds no value, named on the command line.
func given() Bound { return Bound{true, climux.SourceArgs} }

// check reports a case whose declaration does not fit its State.
func (c *Case) check() error {
	build, defect := c.Build != nil, c.Defect != nil
	switch {
	case c.State == Conformant && (!build || defect):
		return errors.New("a Conformant case needs Build and no Defect")
	case c.State == KnownBad && (!build || !defect):
		return errors.New("a KnownBad case needs Build and Defect")
	case c.State == NotImplemented && (build || defect):
		return errors.New("a NotImplemented case has no Build or Defect")
	}
	return nil
}

// tool is the program the case runs, which is its first word after any
// environment assignments.
func (c *Case) tool() string {
	_, args := splitLine(c.Argv)
	return args[0]
}

// name is Argv without the program name, which the subtest is named for.
func (c *Case) name() string {
	return strings.Replace(c.Argv, c.tool()+" ", "", 1)
}

func TestConformance(t *testing.T) {
	for _, f := range features {
		t.Run(f.Slug, func(t *testing.T) {
			for _, c := range f.Cases {
				t.Run(c.tool()+"/"+c.name(), func(t *testing.T) { c.run(t, f) })
			}
		})
	}
}

func (c *Case) run(t *testing.T, f Feature) {
	if err := c.check(); err != nil {
		t.Fatalf("%s\n%v", c.Argv, err)
	}
	if c.State == NotImplemented {
		t.Skipf("not implemented: needs %s", f.Slug)
	}
	environ, args := splitLine(c.Argv)
	for k, v := range environ {
		t.Setenv(k, v)
	}
	got := outcome(t, c.Build(), args[1:])

	if c.State == Conformant {
		if !reflect.DeepEqual(got, c.Spec) {
			t.Errorf("%s\n%s", c.Argv, diff(c.Spec, got))
		}
		return
	}
	if reflect.DeepEqual(got, c.Spec) {
		t.Errorf("%s\nconformant now: delete State and Defect", c.Argv)
		return
	}
	if !reflect.DeepEqual(got, *c.Defect) {
		t.Errorf("%s\nknown-bad outcome changed:\n%s", c.Argv, diff(*c.Defect, got))
	}
}

// outcome parses args against cmd and reports what that meant. A tree
// that does not compile is a broken fixture rather than an outcome.
func outcome(t *testing.T, cmd *climux.Command, args []string) Outcome {
	t.Helper()
	inv, err := climux.Parse(cmd, args...)
	if cfgErr := (*ir.ConfigError)(nil); errors.As(err, &cfgErr) {
		t.Fatalf("fixture: %v", err)
	}
	if argErr := (*ir.ArgumentError)(nil); errors.As(err, &argErr) {
		return Outcome{Err: &Failure{Arg: argErr.Arg}}
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	o := Outcome{Cmd: inv.Cmd.FullName}
	if inv.Interrupt != nil {
		o.Interrupt = inv.Interrupt.Name
	}
	seen := make(map[*ir.FlagState]bool)
	for _, cmd := range inv.Cmd.Ancestry {
		for _, group := range cmd.FlagGroups {
			for _, f := range group.Flags {
				// A flag mounted at two depths shares one state.
				if f.State.Source == climux.SourceDefault || seen[f.State] {
					continue
				}
				seen[f.State] = true
				if o.Flags == nil {
					o.Flags = Flags{}
				}
				if _, dup := o.Flags[f.Name]; dup {
					t.Fatalf("fixture: two flags named %q were set", f.Name)
				}
				var v any
				if f.State.Get != nil {
					v = f.State.Get()
				}
				o.Flags[f.Name] = Bound{v, f.State.Source}
			}
		}
	}
	return o
}

// diff lists each field of two outcomes that differs.
func diff(want, got Outcome) string {
	var b strings.Builder
	line := func(field string, w, g any) {
		fmt.Fprintf(&b, "  %-24s want %s\n  %-24s  got %s\n", field, w, "", g)
	}
	if want.Cmd != got.Cmd {
		line("Cmd", fmt.Sprintf("%q", want.Cmd), fmt.Sprintf("%q", got.Cmd))
	}
	if want.Interrupt != got.Interrupt {
		line("Interrupt", fmt.Sprintf("%q", want.Interrupt), fmt.Sprintf("%q", got.Interrupt))
	}
	if !reflect.DeepEqual(want.Err, got.Err) {
		line("Err", failureString(want.Err), failureString(got.Err))
	}
	names := make([]string, 0, len(want.Flags)+len(got.Flags))
	for name := range want.Flags {
		names = append(names, name)
	}
	for name := range got.Flags {
		if _, ok := want.Flags[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		w, wok := want.Flags[name]
		g, gok := got.Flags[name]
		if wok == gok && reflect.DeepEqual(w, g) {
			continue
		}
		line(fmt.Sprintf("Flags[%q]", name), boundString(w, wok), boundString(g, gok))
	}
	return b.String()
}

func boundString(b Bound, ok bool) string {
	if !ok {
		return "unset"
	}
	return fmt.Sprintf("%#v from %s", b.Value, b.Source)
}

func failureString(f *Failure) string {
	if f == nil {
		return "accepted"
	}
	return fmt.Sprintf("rejected at %q", f.Arg)
}

var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// splitLine splits a command line into words as a POSIX shell would for
// the quoting the cases use, and separates the leading environment
// assignments from the command.
func splitLine(line string) (environ map[string]string, args []string) {
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	for len(words) > 0 && assignment.MatchString(words[0]) {
		k, v, _ := strings.Cut(words[0], "=")
		if environ == nil {
			environ = map[string]string{}
		}
		environ[k] = v
		words = words[1:]
	}
	return environ, words
}
