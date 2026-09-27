package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/cargo"
	"go.hotsrc.dev/climux/conformance/internal/gh"
	"go.hotsrc.dev/climux/conformance/internal/git"
	"go.hotsrc.dev/climux/conformance/internal/kubectl"
)

// Each tool names both flags in its message rather than blaming one, so
// no case expects an Arg.
var constraints = Feature{
	Slug:    "constraints",
	Summary: "Two flags each legal alone are refused together. The handler checks them; see docs/adr/flag-constraints-belong-to-the-handler.md.",
	Cases: []Case{
		{
			Argv:   "gh pr create --editor --web",
			Source: "cli/cli@v2.101.0 pkg/cmd/pr/create/create.go",
			Spec:   Outcome{Err: &Failure{}},
			Build:  gh.PRCreate,
		},
		{
			// Either flag alone is accepted.
			Argv:   "gh pr create --web",
			Source: "cli/cli@v2.101.0 pkg/cmd/pr/create/create.go",
			Spec: Outcome{
				Cmd:   "gh pr create",
				Flags: Flags{"web": arg(true)},
			},
			Build: gh.PRCreate,
		},
		{
			Argv:   "git commit -m fix -F msg.txt",
			Source: "git/git builtin/commit.c",
			Spec:   Outcome{Err: &Failure{}},
			Build:  git.Commit,
		},
		{
			Argv:   "kubectl logs mypod --since=1h --since-time=2026-09-01T00:00:00Z",
			Source: "kubernetes/kubectl@v1.34 pkg/cmd/logs/logs.go",
			Spec:   Outcome{Err: &Failure{}},
			Build:  kubectl.Logs,
		},
		{
			// Declared with clap's conflicts_with.
			Argv:   "cargo build --release --profile dev",
			Source: "rust-lang/cargo@0.99.0 src/cargo/util/command_prelude.rs",
			Spec:   Outcome{Err: &Failure{}},
			Build:  cargo.Build,
		},
	},
}
