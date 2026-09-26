// This example demonstrates the packaging convention for a library that
// contributes command line flags to whichever program imports it: a
// settings struct whose FlagGroup method binds flags to the receiver, and
// one package-level instance registered into climux.DefaultRegistry.
package climux

import (
	"context"
	"fmt"
)

// logSettings is the settings struct a library such as a logging package
// would own. Its FlagGroup method binds flags to the receiver rather than
// to package variables, which is the point of the convention: a test binds
// a fresh, isolated instance, while the package-level instance below is
// merely the one the program mounts.
type logSettings struct {
	Level  string
	Format string
}

// FlagGroup returns a new group of flags bound to s.
func (s *logSettings) FlagGroup() *FlagGroup {
	return NewFlagGroup(
		"logging", "Logging options",
		String(&s.Level, "log-level", "Set log verbosity").Default("info"),
		String(&s.Format, "log-format", "Log output format").Default("text"),
	)
}

// logFlags is the instance the program mounts. Its group is registered in
// a var declaration of its own, so the library needs no init function; the
// registry is what the call returns and nothing here wants it, so it goes
// to the blank identifier. A blank import of the library is enough to
// contribute the flags, since nothing the program writes names them.
var logFlags = &logSettings{}

var _ = DefaultRegistry.FlagGroups(logFlags.FlagGroup())

func Example_sharedFlagGroups() {
	ctx := context.Background()

	// The program mounts every registered group with one line. From
	// outside this package that line reads Mount(climux.DefaultRegistry).
	app := NewCommand("myapp", "Do things, with logging").
		HelpFlag().
		Mount(DefaultRegistry).
		HandleFunc(func(ctx context.Context, inv *Invocation) error {
			fmt.Fprintf(inv.Stdout, "level: %s\n", logFlags.Level)
			return nil
		})

	fmt.Println("+ myapp --help")
	Run(ctx, app, WithArgs("--help"))

	fmt.Println()
	fmt.Println("+ myapp --log-level=debug")
	Run(ctx, app, WithArgs("--log-level=debug"))

	// A test mounts a fresh instance instead, fully isolated from the
	// registered one.
	var isolated logSettings
	cmd := NewCommand("test", "").FlagGroups(isolated.FlagGroup())
	if _, err := Parse(cmd, "--log-level=warn"); err != nil {
		panic(err)
	}
	fmt.Println()
	fmt.Printf("isolated: %s, registered: %s\n", isolated.Level, logFlags.Level)
	// Output:
	// + myapp --help
	// Usage: myapp [OPTIONS]
	//
	// Do things, with logging
	//
	// Options:
	//   -h, --help  Show this help message and exit
	//
	// Logging options:
	//    --log-level   Set log verbosity
	//    --log-format  Log output format
	//
	// + myapp --log-level=debug
	// level: debug
	//
	// isolated: warn, registered: debug
}
