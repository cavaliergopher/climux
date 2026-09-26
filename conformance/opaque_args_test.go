package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/cargo"
	"go.hotsrc.dev/climux/conformance/internal/docker"
	"go.hotsrc.dev/climux/conformance/internal/git"
	"go.hotsrc.dev/climux/conformance/internal/kubectl"
)

var opaqueArgs = Feature{
	Slug:    "opaque-args",
	Summary: "A command hands the rest of its line to another program unread, with no -- required.",
	Cases: []Case{
		{
			Argv:   "docker run alpine echo hi",
			Source: "docker/cli@v29.8.2 cli/command/container/run.go",
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"IMAGE": arg("alpine"),
					"ARG":   arg([]string{"echo", "hi"}),
				},
			},
			Build: docker.Run,
		},
		{
			Argv:   "docker run -it --rm alpine ls -la",
			Source: "docker/cli@v29.8.2 cli/command/container/run.go",
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"interactive": arg(true),
					"tty":         arg(true),
					"rm":          arg(true),
					"IMAGE":       arg("alpine"),
					"ARG":         arg([]string{"ls", "-la"}),
				},
			},
			Build: docker.Run,
		},
		{
			// -v spells run's own --volume, but past IMAGE it is the
			// container's.
			Argv:   "docker run alpine -v",
			Source: "docker/cli@v29.8.2 cli/command/container/run.go",
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"IMAGE": arg("alpine"),
					"ARG":   arg([]string{"-v"}),
				},
			},
			Build: docker.Run,
		},
		{
			// IMAGE already ended option processing, so docker forwards
			// the -- as the container's first argument.
			Argv:   "docker run alpine -- -v",
			Source: "docker/cli@v29.8.2 cli/command/container/run.go",
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"IMAGE": arg("alpine"),
					"ARG":   arg([]string{"--", "-v"}),
				},
			},
			Build: docker.Run,
		},
		{
			Argv:   "git bisect run ./t.sh --verbose",
			Source: "git/git builtin/bisect.c",
			Spec: Outcome{
				Cmd:   "git bisect run",
				Flags: Flags{"CMD": arg([]string{"./t.sh", "--verbose"})},
			},
			Build: git.Bisect,
		},
		{
			Argv:   "cargo run --release val1 --foo",
			Source: "rust-lang/cargo@0.99.0 src/bin/cargo/commands/run.rs",
			Spec: Outcome{
				Cmd: "cargo run",
				Flags: Flags{
					"release": arg(true),
					"ARGS":    arg([]string{"val1", "--foo"}),
				},
			},
			Build: cargo.Run,
		},
		{
			Argv:   "cargo run --release -- --foo",
			Source: "rust-lang/cargo@0.99.0 src/bin/cargo/commands/run.rs",
			Spec: Outcome{
				Cmd: "cargo run",
				Flags: Flags{
					"release": arg(true),
					"ARGS":    arg([]string{"--foo"}),
				},
			},
			Build: cargo.Run,
		},
		{
			Argv:   "kubectl exec mypod -c app -- ls -la",
			Source: "kubernetes/kubectl@v1.34 pkg/cmd/exec/exec.go",
			Spec: Outcome{
				Cmd: "kubectl exec",
				Flags: Flags{
					"POD":       arg("mypod"),
					"container": arg("app"),
					"COMMAND":   arg([]string{"ls", "-la"}),
				},
			},
			Build: kubectl.Exec,
		},
		{
			// The spelling without -- was removed. kubectl rejects the
			// spelling rather than any one word.
			Argv:   "kubectl exec mypod ls",
			State:  KnownBad,
			Source: "kubernetes/kubectl@v1.34 pkg/cmd/exec/exec.go",
			Spec:   Outcome{Err: &Failure{}},
			// climux cannot require a -- before a positional.
			Defect: &Outcome{
				Cmd: "kubectl exec",
				Flags: Flags{
					"POD":     arg("mypod"),
					"COMMAND": arg([]string{"ls"}),
				},
			},
			Build: kubectl.Exec,
		},
	},
}
