package climux

import (
	"context"
	"fmt"
)

// ExampleSource shows a flag whose meaning depends on whether another
// flag was given at all: --template picks the output format, but only
// where --output did not choose one. The bound variable cannot answer
// that -- a defaulted --output and an --output typed with its default
// value hold the same string -- so the invocation is asked instead.
func ExampleSource() {
	// A tree reads one command line, so each line below builds its own;
	// see docs/adr/a-tree-reads-one-command-line.md.
	run := func(args ...string) {
		var output, template string
		outputFlag := String(&output, "output", "table", "Output format")

		cmd := NewCommand("get", "Display resources").
			Flags(
				outputFlag,
				String(&template, "template", "", "Go template"),
			).
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				if template != "" && !outputFlag.IsSetIn(inv) {
					output = "go-template"
				}
				fmt.Fprintf(inv.Stdout, "output=%-12s --output was %s\n",
					output, inv.Source("output"))
				return nil
			})

		Run(context.Background(), cmd, WithArgs(args...))
	}

	run()
	run("--template", "{{.Name}}")
	run("--output", "json", "--template", "{{.Name}}")
	run("--output", "table")

	// Output:
	// output=table        --output was default
	// output=go-template  --output was default
	// output=json         --output was args
	// output=table        --output was args
}
