package conformance_test

var externalSubcommand = Feature{
	Slug:    "external-subcommand",
	Summary: "A word the tree does not declare dispatches to another executable, which reads the rest.",
	Cases: []Case{
		{
			Argv:      "git -C /srv/repo lfs push origin main --dry-run",
			State:     NotImplemented,
			Source:    "git/git git.c",
			DependsOn: []string{"opaque-args"},
			Spec: Outcome{
				Cmd: "git lfs",
				Flags: Flags{
					"C":   arg("/srv/repo"),
					"ARG": arg([]string{"push", "origin", "main", "--dry-run"}),
				},
			},
		},
		{
			// Past the command word, git's own -C is git-lfs's to read.
			Argv:      "git lfs -C /srv/repo push --dry-run",
			State:     NotImplemented,
			Source:    "git/git git.c",
			DependsOn: []string{"opaque-args"},
			Spec: Outcome{
				Cmd:   "git lfs",
				Flags: Flags{"ARG": arg([]string{"-C", "/srv/repo", "push", "--dry-run"})},
			},
		},
		{
			Argv:      "docker --debug buildx build --push .",
			State:     NotImplemented,
			Source:    "docker/cli@v29.8.2 cli-plugins/manager/cobra.go",
			DependsOn: []string{"persistent-flags", "opaque-args"},
			Spec: Outcome{
				Cmd: "docker buildx",
				Flags: Flags{
					"debug": arg(true),
					"ARG":   arg([]string{"build", "--push", "."}),
				},
			},
		},
	},
}
