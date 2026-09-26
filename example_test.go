// This example demonstrates a simple "Hello, World!" CLI program.
package climux

import (
	"context"
	"fmt"
	"strings"
)

// Each flag holds its own value, and State returns the half of it the
// handler reads.
var (
	flagNoNewLines = Bool("n", "Do not print the trailing newline character").State()

	// String flag to select a desired language. Can be specified with
	// -l, --language or the HW_LANG environment variable.
	flagLanguage = String("language", "Language (en, es, it or nl)").
			Default("en").
			Aliases("l").
			Env("HW_LANG").
			State()

	// StringSlice flag to optionally print multiple positional
	// arguments. Positional arguments are not denoted with "-" or "--".
	flagMessage = Strings("MESSAGE", "Optional message to print").
			Positional().
			State()
)

var translations = map[string]string{
	"en": "Hello, World!",
	"es": "Hola, Mundo!",
	"it": "Ciao, Mondo!",
	"nl": "Hallo, Wereld!",
}

var App = NewCommand("helloworld", "Print \"Hello, World!\"").
	// By convention the flag that prints help is declared first, so it
	// heads the list of options; nothing requires it.
	HelpFlag().
	Description(
		"The helloworld utility writes \"Hello, World!\" to the standard\n"+
			" output multiple languages.",
	).
	Flags(flagNoNewLines, flagLanguage, flagMessage).
	HandleFunc(helloWorld)

// helloWorld is the HandlerFunc for the main App command.
func helloWorld(ctx context.Context, inv *Invocation) error {
	s, ok := translations[flagLanguage.Value()]
	if !ok {
		return fmt.Errorf("unsupported language: %s", flagLanguage.Value())
	}
	if message := flagMessage.Value(); len(message) > 0 {
		s = strings.Join(message, " ")
	}
	fmt.Fprint(inv.Stdout, s)
	if !flagNoNewLines.Value() {
		fmt.Fprint(inv.Stdout, "\n")
	}
	return nil
}

func Example() {
	ctx := context.Background()

	fmt.Println("+ helloworld --help")
	Run(ctx, App, WithArgs("--help"))

	// Most programs will call the following from main:
	//
	//     func main() {
	//         ctx, stop := climux.NotifyContext(context.Background())
	//         defer stop()
	//         os.Exit(climux.Run(ctx, App))
	//     }
	//
	fmt.Println()
	fmt.Println("+ helloworld --language=es")
	Run(ctx, App, WithArgs("--language=es"))
	// Output:
	// + helloworld --help
	// Usage: helloworld [OPTIONS] [MESSAGE...]
	//
	// Print "Hello, World!"
	//
	// Positional arguments:
	//   MESSAGE  Optional message to print
	//
	// Options:
	//   -h, --help      Show this help message and exit
	//   -n              Do not print the trailing newline character
	//   -l, --language  Language (en, es, it or nl)
	//
	// Environment variables:
	//   HW_LANG  Language (en, es, it or nl)
	//
	// The helloworld utility writes "Hello, World!" to the standard
	//  output multiple languages.
	//
	// + helloworld --language=es
	// Hola, Mundo!
}
