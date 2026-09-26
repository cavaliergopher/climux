// This example demonstrates how a custom struct type may be used to encapsulate
// the behavior of a single command.
package climux

import (
	"context"
	"fmt"
)

// exampleCommand encapsulates the configuration and handler for a CLI
// command as a custom type.
// It collects the value of each flag as a struct field.
// Command types should be very fast to initialize, ideally initialized with their zero-value.
type exampleCommand struct {
	Species    string
	GopherType string
}

// Command returns the CLI configuration of the example command as a
// *Command.
func (c *exampleCommand) Command() *Command {
	return NewCommand("example", "An example CLI program").
		HelpFlag().
		Flags(
			String(&c.Species, "species", "the species we are studying").Default("Gopher"),
			String(&c.GopherType, "gopher_type", "the variety of gopher").Default("Pocket"),
		).
		HandleFunc(c.Run)
}

// Run handles calls to this command from the command line.
func (c *exampleCommand) Run(ctx context.Context, inv *Invocation) error {
	fmt.Fprintf(inv.Stdout, "%s is a variety of species %s\n", c.GopherType, c.Species)
	return nil
}

// ExampleCommand is a *Command built from a global instance of the
// exampleCommand type so that its parsed flag values can be accessed from
// other commands. This is an optional alternative to defining flag variables
// individually in the global scope.
var ExampleCommand = (&exampleCommand{}).Command()

func Example_customTypes() {
	ctx := context.Background()

	fmt.Println("+ example --help")
	Run(ctx, ExampleCommand, WithArgs("--help"))

	// Most programs will call the following from main:
	//
	//     func main() {
	//         ctx, stop := climux.NotifyContext(context.Background())
	//         defer stop()
	//         os.Exit(climux.Run(ctx, ExampleCommand))
	//     }
	//
	fmt.Println()
	fmt.Println("+ example --gopher_type 'Goldman's pocket gopher'")
	Run(ctx, ExampleCommand, WithArgs("--gopher_type", "Goldman's pocket gopher"))
	// Output:
	// + example --help
	// Usage: example [OPTIONS]
	//
	// An example CLI program
	//
	// Options:
	//   -h, --help         Show this help message and exit
	//       --species      the species we are studying
	//       --gopher_type  the variety of gopher
	//
	// + example --gopher_type 'Goldman's pocket gopher'
	// Goldman's pocket gopher is a variety of species Gopher
}
