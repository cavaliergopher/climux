package climux

import (
	"encoding/json"
	"testing"
)

// TestCommandDescribe compiles a small tree and asserts that Describe
// reports the full name computed from ancestry, the kind of value a flag
// takes, and the option order the machine-readable-schema ADR makes
// contractual: the canonical name, the short name, then whatever the
// dialect generated -- here, "--no-force" for the boolean's opposite.
func TestCommandDescribe(t *testing.T) {
	var force bool
	add := NewCommand("add", "Add a remote").
		Flags(Bool(&force, "force", "Overwrite an existing remote").Aliases("f"))
	NewCommand("orbital", "").Subcommands(add)

	node, err := add.Compile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := node.Describe()

	if got, want := got.FullName, "orbital add"; got != want {
		t.Errorf("FullName = %q, want %q", got, want)
	}
	if got, want := len(got.FlagGroups), 1; got != want {
		t.Fatalf("len(FlagGroups) = %d, want %d", got, want)
	}
	flag := got.FlagGroups[0].Flags[0]
	if got, want := flag.Kind, "bool"; got != want {
		t.Errorf("Kind = %q, want %q", got, want)
	}

	b, err := json.Marshal(flag.Options)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `[{"option":"--force"},{"option":"-f"},{"option":"--no-force","effect":"negate"}]`; got != want {
		t.Errorf("Options = %s, want %s", got, want)
	}
}
