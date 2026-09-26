package conformance_test

import (
	"go.hotsrc.dev/climux/conformance/internal/kubectl"
)

var optionalValue = Feature{
	Slug:    "optional-value",
	Summary: "A flag is legal bare or with a value, and the bare form carries a declared default.",
	Cases: []Case{
		{
			// Bare --dry-run means "unchanged", and never takes the next
			// word as its value, so client lands in run's operands.
			Argv:   "kubectl run nginx --image=nginx --dry-run client",
			State:  KnownBad,
			Source: "kubernetes/kubectl@v1.34 pkg/cmd/util/helpers.go",
			Spec: Outcome{
				Cmd: "kubectl run",
				Flags: Flags{
					"NAME":    arg("nginx"),
					"image":   arg("nginx"),
					"dry-run": arg("unchanged"),
					"ARGS":    arg([]string{"client"}),
				},
			},
			// The nearest declaration today is a flag that requires its
			// value, so it takes client.
			Defect: &Outcome{
				Cmd: "kubectl run",
				Flags: Flags{
					"NAME":    arg("nginx"),
					"image":   arg("nginx"),
					"dry-run": arg("client"),
				},
			},
			Build: kubectl.Run,
		},
	},
}
