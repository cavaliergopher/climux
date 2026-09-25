package desc

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// fullyPopulatedDocument returns a Document with every field of every
// desc type set to a non-zero value somewhere in the tree, so that
// flattening it exercises every key the format defines.
func fullyPopulatedDocument() *Document {
	return &Document{
		SchemaVersion: 1,
		Command: &Command{
			Name:        "add",
			FullName:    "orbital remote add",
			Summary:     "Add a remote",
			Description: "Add a new remote repository.",
			Hidden:      true,
			FlagGroups: []*FlagGroup{
				{
					Name:  "options",
					Title: "Options",
					Flags: []*Flag{
						{
							Name:  "force",
							Kind:  "bool",
							Usage: "Overwrite an existing remote",
							Options: []Option{
								{Option: "--force"},
								{Option: "-f"},
								{Option: "--no-force", Effect: "negate"},
							},
							MaxCount: 1,
						},
						{
							Name:         "path",
							ValueName:    "PATH",
							Kind:         "string",
							Usage:        "Path to the remote",
							Default:      "origin",
							ShowDefault:  true,
							Positional:   true,
							EndOfOptions: true,
							Hidden:       true,
							MinCount:     1,
							MaxCount:     1,
							EnvVar:       "ORBITAL_REMOTE_PATH",
							Choices:      []string{"origin", "upstream"},
							TakesValue:   true,
						},
					},
				},
			},
			Subcommands: []*Command{
				{
					Name:     "list",
					FullName: "orbital remote add list",
				},
			},
		},
	}
}

// wireKeyPaths pins the flattened key paths of fullyPopulatedDocument's
// JSON. Every path here is a promise this schema version has made to a
// consumer. See TestWireKeyPaths for what to do when this list needs to
// change.
var wireKeyPaths = []string{
	"command",
	"command.description",
	"command.flagGroups",
	"command.flagGroups.flags",
	"command.flagGroups.flags.choices",
	"command.flagGroups.flags.default",
	"command.flagGroups.flags.envVar",
	"command.flagGroups.flags.hidden",
	"command.flagGroups.flags.kind",
	"command.flagGroups.flags.maxCount",
	"command.flagGroups.flags.minCount",
	"command.flagGroups.flags.name",
	"command.flagGroups.flags.options",
	"command.flagGroups.flags.options.effect",
	"command.flagGroups.flags.options.option",
	"command.flagGroups.flags.endOfOptions",
	"command.flagGroups.flags.positional",
	"command.flagGroups.flags.showDefault",
	"command.flagGroups.flags.takesValue",
	"command.flagGroups.flags.usage",
	"command.flagGroups.flags.valueName",
	"command.flagGroups.name",
	"command.flagGroups.title",
	"command.fullName",
	"command.hidden",
	"command.name",
	"command.subcommands",
	"command.subcommands.fullName",
	"command.subcommands.name",
	"command.summary",
	"schemaVersion",
}

// flattenKeyPaths marshals v and walks the result, collecting every key
// path reachable from the root as "a.b.c". An array contributes no
// segment of its own: every element is walked at its parent's path, so
// index positions collapse into one path per field.
func flattenKeyPaths(t *testing.T, v any) []string {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	paths := make(map[string]bool)
	walkKeyPaths("", decoded, paths)

	got := make([]string, 0, len(paths))
	for p := range paths {
		got = append(got, p)
	}
	sort.Strings(got)
	return got
}

func walkKeyPaths(prefix string, v any, paths map[string]bool) {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			path := k
			if prefix != "" {
				path = prefix + "." + k
			}
			paths[path] = true
			walkKeyPaths(path, child, paths)
		}
	case []any:
		for _, elem := range v {
			walkKeyPaths(prefix, elem, paths)
		}
	}
}

// TestWireKeyPaths guards the description format against an unintended
// change: it fails if marshaling fullyPopulatedDocument stops producing
// exactly the key paths pinned in wireKeyPaths.
//
// A path that disappeared was REMOVED or RENAMED. Either is a wire
// break and must not ship within schemaVersion 1: revert the field
// change, or bump SchemaVersion, which is the maintainer's call.
//
// A path that appeared is ADDED. That is legal within schemaVersion 1 --
// update wireKeyPaths, and docs/desc.schema.json if the new field
// belongs in it, in the same change. A changed schema has to be
// republished as well; CONTRIBUTING.md says how.
func TestWireKeyPaths(t *testing.T) {
	got := flattenKeyPaths(t, fullyPopulatedDocument())
	want := wireKeyPaths

	gotSet := make(map[string]bool, len(got))
	for _, p := range got {
		gotSet[p] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, p := range want {
		wantSet[p] = true
	}

	var removed, added []string
	for _, p := range want {
		if !gotSet[p] {
			removed = append(removed, p)
		}
	}
	for _, p := range got {
		if !wantSet[p] {
			added = append(added, p)
		}
	}

	if len(removed) == 0 && len(added) == 0 {
		return
	}

	var msg strings.Builder
	if len(removed) > 0 {
		msg.WriteString("REMOVED or RENAMED (wire break, must not ship within schemaVersion 1):\n")
		for _, p := range removed {
			msg.WriteString("  - " + p + "\n")
		}
	}
	if len(added) > 0 {
		msg.WriteString("ADDED (additive, legal within schemaVersion 1 -- update wireKeyPaths and docs/desc.schema.json in this change, then republish; see CONTRIBUTING.md):\n")
		for _, p := range added {
			msg.WriteString("  + " + p + "\n")
		}
	}
	t.Errorf("wire key paths changed:\n%s", msg.String())
}

// TestSchemaVersion pins the document's schema version. Bumping it is
// the maintainer's call, made deliberately when a breaking change is
// accepted, not a side effect of an unrelated test update.
func TestSchemaVersion(t *testing.T) {
	if got, want := fullyPopulatedDocument().SchemaVersion, 1; got != want {
		t.Errorf("SchemaVersion = %d, want %d", got, want)
	}
}
