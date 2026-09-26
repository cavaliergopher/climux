package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/docker"
)

var persistentFlags = Feature{
	Slug:    "persistent-flags",
	Summary: "A root flag may be written before or after the subcommand that reads it.",
	Cases: []Case{
		{
			Argv:      "docker --debug run -it alpine ls -la",
			Source:    "docker/cli@v29.8.2 cli/flags/options.go",
			DependsOn: []string{"opaque-args"},
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"debug":       arg(true),
					"interactive": arg(true),
					"tty":         arg(true),
					"IMAGE":       arg("alpine"),
					"ARG":         arg([]string{"ls", "-la"}),
				},
			},
			Build: docker.Run,
		},
		{
			Argv:      "docker run --debug -it alpine ls",
			Source:    "docker/cli@v29.8.2 cmd/docker/docker.go",
			DependsOn: []string{"opaque-args"},
			Spec: Outcome{
				Cmd: "docker run",
				Flags: Flags{
					"debug":       arg(true),
					"interactive": arg(true),
					"tty":         arg(true),
					"IMAGE":       arg("alpine"),
					"ARG":         arg([]string{"ls"}),
				},
			},
			Build: docker.Run,
		},
	},
}
