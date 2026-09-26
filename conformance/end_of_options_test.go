package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/git"
)

var endOfOptions = Feature{
	Slug:    "end-of-options",
	Summary: "A marker other than -- ends option processing, or -- splits one kind of operand from another.",
	Cases: []Case{
		{
			// -- is taken to mean paths, so git needed a second spelling.
			Argv:   "git log --oneline --end-of-options --weird-branch",
			Source: "git/git revision.c",
			Spec: Outcome{
				Cmd: "git log",
				Flags: Flags{
					"oneline":        arg(true),
					"end-of-options": given(),
					"REV":            arg([]string{"--weird-branch"}),
				},
			},
			Build: git.Log,
		},
		{
			// Nothing says where REV stops and PATH starts but the --.
			Argv:   "git log --oneline main -- Makefile",
			State:  NotImplemented,
			Source: "git/git revision.c",
			Spec: Outcome{
				Cmd: "git log",
				Flags: Flags{
					"oneline": arg(true),
					"REV":     arg([]string{"main"}),
					"PATH":    arg([]string{"Makefile"}),
				},
			},
		},
	},
}
