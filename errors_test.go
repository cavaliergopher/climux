package climux

import (
	"context"
	"strings"
	"testing"
)

// TestNewArgumentErrorfReportsLikeTheParser asserts that an argument error
// a handler returns is reported the way the parser's own are: the
// "Argument error" prefix, the usage of the command it names, and the
// usage exit code. Here the handler enforces an argument required only
// when no subcommand was named.
func TestNewArgumentErrorfReportsLikeTheParser(t *testing.T) {
	var plugin string
	cmd := NewCommand("kubectl", "").
		Flags(String("PLUGIN", "").Bind(&plugin).Positional()).
		Subcommands(NewCommand("get", "").HandleFunc(
			func(ctx context.Context, inv *Invocation) error { return nil })).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			if plugin == "" {
				return NewArgumentErrorf(nil, inv.Cmd, nil, "", "missing subcommand or PLUGIN")
			}
			return nil
		})

	code, _, stderr := runCaptured(cmd)
	if got, want := code, ExitCodeUsage; got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
	if want := "Argument error: missing subcommand or PLUGIN\nUsage: kubectl "; !strings.HasPrefix(stderr, want) {
		t.Errorf("stderr = %q, want it to begin %q", stderr, want)
	}

	if code, _, stderr := runCaptured(cmd, "get"); code != ExitCodeSuccess {
		t.Errorf("exit code = %d, want %d (stderr: %s)", code, ExitCodeSuccess, stderr)
	}
}
