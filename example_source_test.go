package climux

import (
	"context"
	"fmt"
)

// ExampleFlagState shows a flag whose meaning depends on whether another
// flag was given at all: --template picks the output format, but only
// where --output did not choose one. The value cannot answer that -- a
// defaulted --output and an --output typed with its default value hold
// the same string -- so the flag's state is asked instead.
func ExampleFlagState() {
	// A tree reads one command line, so each line below builds its own;
	// see docs/adr/a-tree-reads-one-command-line.md.
	run := func(args ...string) {
		output := String("output", "Output format").Default("table").State()
		template := String("template", "Go template").State()

		cmd := NewCommand("get", "Display resources").
			Flags(output, template).
			HandleFunc(func(ctx context.Context, inv *Invocation) error {
				format := output.Value()
				if template.Value() != "" && !output.IsSet() {
					format = "go-template"
				}
				fmt.Fprintf(inv.Stdout, "output=%-12s --output was %s\n",
					format, output.Source())
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
