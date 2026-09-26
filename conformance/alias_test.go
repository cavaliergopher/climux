package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/cargo"
	"go.hotsrc.dev/climux/conformance/internal/gh"
	"go.hotsrc.dev/climux/conformance/internal/git"
)

var alias = Feature{
	Slug:    "alias",
	Summary: "A second name for a command, declared by the program or configured by the user.",
	Cases: []Case{
		{
			// With alias.st = "status -sb" in the user's config.
			Argv:   "git st -uno",
			State:  KnownBad,
			Source: "git/git git.c",
			Spec: Outcome{
				Cmd: "git status",
				Flags: Flags{
					"short":           arg(true),
					"branch":          arg(true),
					"untracked-files": arg("no"),
				},
			},
			// st is declared as an alias of status, so the line reaches
			// it without -sb. To solve: an alias that carries arguments.
			Defect: &Outcome{
				Cmd: "git status",
				Flags: Flags{
					"untracked-files": arg("no"),
				},
			},
			Build: git.Status,
		},
		{
			// With co aliased to "pr checkout". The expansion is parsed
			// again, and pr checkout has no --web.
			Argv:   "gh co 123 --web",
			Source: "cli/cli@v2.101.0 pkg/cmd/root/alias.go",
			Spec:   Outcome{Err: &Failure{Arg: "--web"}},
			Build:  gh.PR,
		},
		{
			Argv:      "cargo r --release -- --foo",
			Source:    "rust-lang/cargo@0.99.0 src/bin/cargo/cli.rs",
			DependsOn: []string{"opaque-args"},
			Spec: Outcome{
				Cmd: "cargo run",
				Flags: Flags{
					"release": arg(true),
					"ARGS":    arg([]string{"--foo"}),
				},
			},
			Build: cargo.Run,
		},
	},
}
