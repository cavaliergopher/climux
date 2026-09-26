package conformance_test

var envRouting = Feature{
	Slug:    "env-routing",
	Summary: "The environment decides which command a word names.",
	Cases: []Case{
		{
			// With buildx installed, build is rewritten to buildx build.
			Argv:   "docker build -t foo .",
			State:  NotImplemented,
			Source: "docker/cli@v29.8.2 cmd/docker/builder.go",
			Spec: Outcome{
				Cmd: "docker buildx build",
				Flags: Flags{
					"tag":  arg([]string{"foo"}),
					"PATH": arg("."),
				},
			},
		},
		{
			Argv:   "DOCKER_BUILDKIT=0 docker build -t foo .",
			State:  NotImplemented,
			Source: "docker/cli@v29.8.2 cmd/docker/builder.go",
			Spec: Outcome{
				Cmd: "docker build",
				Flags: Flags{
					"tag":  arg([]string{"foo"}),
					"PATH": arg("."),
				},
			},
		},
	},
}
