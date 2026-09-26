package climux

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.hotsrc.dev/climux/ir"
)

func assertBool(t *testing.T, expect, actual bool) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected bool: %v, got: %v", expect, actual)
	return false
}

func assertDuration(t *testing.T, expect, actual time.Duration) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected duration: %v, got: %v", expect, actual)
	return false
}

func assertFloat64(t *testing.T, expect, actual float64) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected float64: %v, got: %v", expect, actual)
	return false
}

func assertInt64(t *testing.T, expect, actual int64) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected int64: %v, got: %v", expect, actual)
	return false
}

func assertUint64(t *testing.T, expect, actual uint64) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected uint64: 0x%0X, got: 0x%0X", expect, actual)
	return false
}

func assertString(t *testing.T, expect, actual string) bool {
	if expect == actual {
		return true
	}
	t.Errorf("expected string: \"%s\", got: \"%s\"", expect, actual)
	return false
}

func assertStrings(t *testing.T, expect, actual []string) bool {
	if len(expect) != len(actual) {
		t.Errorf("expected string slice: %q, got: %q", expect, actual)
		return false
	}
	for i := 0; i < len(expect); i++ {
		if expect[i] != actual[i] {
			t.Errorf("expected string slice: %q, got: %q", expect, actual)
			return false
		}
	}
	return true
}

func parseFlag(flag Flag, args ...string) error {
	_, err := Parse(NewCommand("test", "").Flags(flag), args...)
	return err
}

func assertFlagParses(t *testing.T, flag Flag, args ...string) bool {
	err := parseFlag(flag, args...)
	if err != nil {
		t.Error(err)
		return false
	}
	return true
}

func assertArgumentError(t *testing.T, err error) bool {
	t.Helper()
	var argErr *ir.ArgumentError
	if errors.As(err, &argErr) {
		return true
	}
	t.Errorf("expected *ArgumentError, got: %T: %v", err, err)
	return false
}

// errWriter is an io.Writer that always fails, standing in for output that
// has gone away: a closed pipe, or a full disk.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

// runCaptured runs cmd with the given arguments, redirecting its output, and
// returns the exit code alongside everything written to stdout and stderr.
func runCaptured(cmd *Command, args ...string) (code int, stdout, stderr string) {
	var out, errOut strings.Builder
	code = Run(context.Background(), cmd,
		WithArgs(args...), WithStdout(&out), WithStderr(&errOut))
	return code, out.String(), errOut.String()
}

// captureStderr returns everything written to os.Stderr while fn runs. It is
// only needed for the failures a command cannot report on its own output,
// which are reported on os.Stderr directly.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = saved }()
	captured := make(chan string, 1)
	go func() {
		var buf strings.Builder
		io.Copy(&buf, r)
		captured <- buf.String()
	}()
	fn()
	w.Close()
	defer r.Close()
	return <-captured
}

// assertCanceled asserts that ctx is canceled, allowing time for the
// goroutine watching for signals to observe whatever canceled it.
func assertCanceled(t *testing.T, ctx context.Context, cause string) bool {
	t.Helper()
	select {
	case <-ctx.Done():
		return true
	case <-time.After(10 * time.Second):
		t.Errorf("context was not canceled by %s", cause)
		return false
	}
}

func TestNotifyContextStop(t *testing.T) {
	ctx, stop := NotifyContext(context.Background())
	select {
	case <-ctx.Done():
		t.Fatal("context was canceled before stop was called")
	default:
	}
	stop()
	if assertCanceled(t, ctx, "stop") {
		if got, want := ctx.Err(), context.Canceled; !errors.Is(got, want) {
			t.Errorf("ctx.Err() = %v, want %v", got, want)
		}
	}
}

func TestNotifyContextParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, stop := NotifyContext(parent)
	defer stop()
	cancel()
	assertCanceled(t, ctx, "canceling the parent")
}

// TestNotifyContextSignal asserts that an interrupt cancels the context. It
// signals this very process, which Windows does not support, and leaves
// default signal handling restored -- so it must be the only test to send
// itself a signal.
func TestNotifyContextSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process cannot signal itself on Windows")
	}
	ctx, stop := NotifyContext(context.Background())
	defer stop()
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	assertCanceled(t, ctx, "an interrupt")
}
