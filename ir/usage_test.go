package ir

import (
	"errors"
	"strings"
	"testing"
)

// blipWriter fails exactly once, on its nth Write call, and succeeds on
// every call before and after -- standing in for a transient failure, as
// opposed to a persistently broken writer whose eventual Flush error would
// mask a dropped write in between.
type blipWriter struct {
	n   int
	buf strings.Builder
}

func (w *blipWriter) Write(p []byte) (int, error) {
	if w.n == 0 {
		w.n = -1
		return 0, errors.New("write failed")
	}
	if w.n > 0 {
		w.n--
	}
	return w.buf.Write(p)
}

// TestPrintUsage pins the bytes of the usage line: the command's full name
// assembled from its ancestry, then each clause -- [OPTIONS], COMMAND, and
// one form per positional arity.
// compiled finishes a hand-built command the way Compile would, setting
// the fields derived from the tree's shape: the ancestry from the root
// down, and the root it begins with. A Command literal is not a compiled
// node until these are set, and everything that reads a compiled tree is
// entitled to assume they are.
func compiled(c *Command, ancestors ...*Command) *Command {
	c.Ancestry = append(append([]*Command{}, ancestors...), c)
	c.Root = c.Ancestry[0]
	return c
}

func TestPrintUsage(t *testing.T) {
	// leaf inherits [OPTIONS] from its parent's persistent flag, and
	// localLeaf nothing from its parent's local one; FullName is set the
	// way Compile would compute it, parent's FullName plus its own name,
	// since printUsage reads the field rather than assembling it from
	// ancestry.
	leaf := &Command{Name: "leaf", FullName: "root leaf"}
	root := compiled(&Command{
		Name:        "root",
		FullName:    "root",
		FlagGroups:  []*FlagGroup{{Flags: []*Flag{{NamedOptions: []string{"--foo"}, Persistent: true}}}},
		Subcommands: []*Command{leaf},
	})
	compiled(leaf, root)
	localLeaf := &Command{Name: "leaf", FullName: "root leaf"}
	localRoot := compiled(&Command{
		Name:        "root",
		FullName:    "root",
		FlagGroups:  []*FlagGroup{{Flags: []*Flag{{NamedOptions: []string{"--foo"}}}}},
		Subcommands: []*Command{localLeaf},
	})
	compiled(localLeaf, localRoot)

	tests := []struct {
		name string
		cmd  *Command
		want string
	}{
		{
			name: "Bare",
			cmd:  compiled(&Command{Name: "test", FullName: "test"}),
			want: "Usage: test\n",
		},
		{
			name: "Options",
			cmd: compiled(&Command{
				Name:       "test",
				FullName:   "test",
				FlagGroups: []*FlagGroup{{Flags: []*Flag{{NamedOptions: []string{"--foo"}}}}},
			}),
			want: "Usage: test [OPTIONS]\n",
		},
		{
			name: "OptionsInherited",
			cmd:  leaf,
			want: "Usage: root leaf [OPTIONS]\n",
		},
		{
			name: "LocalOptionsNotInherited",
			cmd:  localLeaf,
			want: "Usage: root leaf\n",
		},
		{
			name: "Subcommands",
			cmd: compiled(&Command{
				Name:        "test",
				FullName:    "test",
				Subcommands: []*Command{{Name: "sub"}},
			}),
			want: "Usage: test COMMAND\n",
		},
		{
			name: "OptionalPositional",
			cmd: compiled(&Command{
				Name:     "test",
				FullName: "test",
				FlagGroups: []*FlagGroup{{Flags: []*Flag{
					{ValueName: "ARG", Positional: true, MinCount: 0, MaxCount: 1},
				}}},
			}),
			want: "Usage: test [ARG]\n",
		},
		{
			name: "OptionalRepeatedPositional",
			cmd: compiled(&Command{
				Name:     "test",
				FullName: "test",
				FlagGroups: []*FlagGroup{{Flags: []*Flag{
					{ValueName: "ARG", Positional: true, MinCount: 0, MaxCount: 0},
				}}},
			}),
			want: "Usage: test [ARG...]\n",
		},
		{
			name: "RequiredPositional",
			cmd: compiled(&Command{
				Name:     "test",
				FullName: "test",
				FlagGroups: []*FlagGroup{{Flags: []*Flag{
					{ValueName: "ARG", Positional: true, MinCount: 1, MaxCount: 1},
				}}},
			}),
			want: "Usage: test ARG\n",
		},
		{
			name: "RequiredRepeatedPositional",
			cmd: compiled(&Command{
				Name:     "test",
				FullName: "test",
				FlagGroups: []*FlagGroup{{Flags: []*Flag{
					{ValueName: "ARG", Positional: true, MinCount: 1, MaxCount: 0},
				}}},
			}),
			want: "Usage: test ARG...\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			if err := printUsage(&sb, tt.cmd); err != nil {
				t.Fatal(err)
			}
			if got, want := sb.String(), tt.want; got != want {
				t.Errorf("usage line = %q, want %q", got, want)
			}
		})
	}
}

// TestDetailPositionalsReportsBlip asserts that a write failing partway
// through a detail helper is reported even though the tabwriter's rows have
// no usage text, and so no tab character to defer flushing on: each such
// row is flushed straight through on its own Write call, immediately, not
// batched until the trailing Flush -- the very write a persistently broken
// writer's Flush error could otherwise stand in for.
func TestDetailPositionalsReportsBlip(t *testing.T) {
	cmd := &Command{
		Name: "test",
		FlagGroups: []*FlagGroup{{Flags: []*Flag{
			{ValueName: "A", Positional: true},
			{ValueName: "B", Positional: true},
		}}},
	}
	// The header line succeeds; the first flag's row is where the writer
	// blips.
	w := &blipWriter{n: 1}
	if err := detailPositionals(w, cmd); err == nil {
		t.Fatal("expected error, got nil")
	}
}
