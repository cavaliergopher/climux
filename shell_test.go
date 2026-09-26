package climux

import (
	"context"
	"strings"
	"testing"
)

// TestCompletionEnvVarDerivation pins how EnableCompletion derives the
// environment variable Run consults from the root command's name.
func TestCompletionEnvVarDerivation(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{"app", "APP_COMPLETE"},
		{"my-app", "MY_APP_COMPLETE"},
		{"my.app_2", "MY_APP_2_COMPLETE"},
	} {
		if got := completionEnvVar(tt.name); got != tt.want {
			t.Errorf("completionEnvVar(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRunCompletionBashSource(t *testing.T) {
	cmd := NewCommand("app", "").EnableCompletion()
	t.Setenv("APP_COMPLETE", "bash_source")

	code, stdout, stderr := runCaptured(cmd)

	if got, want := code, ExitCodeSuccess; got != want {
		t.Errorf("code = %d, want %d", got, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if !strings.Contains(stdout, "_app_complete()") {
		t.Errorf("stdout does not look like a bash completion script:\n%s", stdout)
	}
	if !strings.Contains(stdout, "APP_COMPLETE=bash_complete") {
		t.Errorf("stdout does not re-invoke with bash_complete:\n%s", stdout)
	}
}

func TestRunCompletionZshSource(t *testing.T) {
	cmd := NewCommand("app", "").EnableCompletion()
	t.Setenv("APP_COMPLETE", "zsh_source")

	code, stdout, _ := runCaptured(cmd)

	if got, want := code, ExitCodeSuccess; got != want {
		t.Errorf("code = %d, want %d", got, want)
	}
	if !strings.HasPrefix(stdout, "#compdef app") {
		t.Errorf("stdout does not start with #compdef app:\n%s", stdout)
	}
}

// TestRunCompletionReply asserts the wire format end to end: COMP_WORDS
// and COMP_CWORD in, "plain," and "nofiles," lines out.
func TestRunCompletionReply(t *testing.T) {
	cmd := NewCommand("app", "").EnableCompletion().Flags(
		Bool("verbose", "").Aliases("v"),
	)
	t.Setenv("APP_COMPLETE", "bash_complete")
	t.Setenv("COMP_WORDS", "app\n--v")
	t.Setenv("COMP_CWORD", "1")

	code, stdout, stderr := runCaptured(cmd)

	if got, want := code, ExitCodeSuccess; got != want {
		t.Errorf("code = %d, want %d", got, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	if got, want := stdout, "plain,--verbose\nnofiles,\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// TestRunCompletionUnknownValueFallsThrough asserts that a value the
// protocol does not recognize leaves Run's ordinary behavior untouched --
// EnableCompletion never hijacks a command line it does not understand.
func TestRunCompletionUnknownValueFallsThrough(t *testing.T) {
	handlerCalled := false
	cmd := NewCommand("app", "").EnableCompletion().
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			handlerCalled = true
			return nil
		})
	t.Setenv("APP_COMPLETE", "something_else")

	code, _, _ := runCaptured(cmd)

	if got, want := code, ExitCodeSuccess; got != want {
		t.Errorf("code = %d, want %d", got, want)
	}
	if !handlerCalled {
		t.Error("handler was not called; Run did not fall through")
	}
}

// TestRunCompletionMalformedTreeFallsThrough asserts that a tree that does
// not compile answers no completion request and falls through to Run's
// ordinary reporting, rather than answering from a tree whose shape is
// exactly what failed to validate.
func TestRunCompletionMalformedTreeFallsThrough(t *testing.T) {
	cmd := NewCommand("app", "").EnableCompletion().Flags(
		String("foo", ""),
		String("foo", ""),
	)
	t.Setenv("APP_COMPLETE", "bash_source")

	code, stdout, stderr := runCaptured(cmd)

	if got, want := code, ExitCodeUsage; got != want {
		t.Errorf("code = %d, want %d", got, want)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	want := "Program error: app: flag declared more than once: --foo\n"
	if got := stderr; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}
