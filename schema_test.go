package climux

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"go.hotsrc.dev/climux/desc"
)

// TestSchemaCommand runs "app schema" against a small tree and asserts
// that the output is a valid Document at the current schema version, that
// a nested command reports the full name Compile gave it, and that a
// hidden flag still appears, marked hidden -- the document describes what
// the parser accepts, not what help shows.
func TestSchemaCommand(t *testing.T) {
	app := NewCommand("app", "").
		Flags(String(new(string), "secret", "", "").Hidden()).
		Subcommands(
			NewCommand("sub", ""),
			SchemaCommand(),
		)

	var out bytes.Buffer
	if err := Dispatch(context.Background(), app, WithArgs("schema"), WithStdout(&out)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Terminating stdout with a newline is a text-stream convention, not a
	// promise about the format: a document's whitespace is not contractual
	// and consumers compare parsed values.
	if got, want := out.String()[len(out.String())-1:], "\n"; got != want {
		t.Errorf("output does not end with a newline: %q", out.String())
	}

	var doc desc.Document
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if got, want := doc.SchemaVersion, 1; got != want {
		t.Errorf("SchemaVersion = %d, want %d", got, want)
	}

	if got, want := len(doc.Command.Subcommands), 2; got != want {
		t.Fatalf("len(Subcommands) = %d, want %d", got, want)
	}
	if got, want := doc.Command.Subcommands[0].FullName, "app sub"; got != want {
		t.Errorf("Subcommands[0].FullName = %q, want %q", got, want)
	}

	if got, want := len(doc.Command.FlagGroups), 1; got != want {
		t.Fatalf("len(FlagGroups) = %d, want %d", got, want)
	}
	if got, want := len(doc.Command.FlagGroups[0].Flags), 1; got != want {
		t.Fatalf("len(FlagGroups[0].Flags) = %d, want %d", got, want)
	}
	flag := doc.Command.FlagGroups[0].Flags[0]
	if got, want := flag.Name, "secret"; got != want {
		t.Errorf("Flags[0].Name = %q, want %q", got, want)
	}
	if got, want := flag.Hidden, true; got != want {
		t.Errorf("Flags[0].Hidden = %v, want %v", got, want)
	}
}
