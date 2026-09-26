// Package identity holds the audit identity that orbital threads through
// its command tree: the root --actor flag sets it once, and any package's
// middleware can read it back without importing main.
package identity

import "go.hotsrc.dev/climux"

// Actor is the --actor flag main mounts on the root command. It names
// who is running orbital, and is read at call time by the Audit
// middleware in examples/orbital/internal/middleware. Declared here
// rather than in main so that a reader needs no import of main; the flag
// is the same object wherever it is mounted.
var Actor = climux.String("actor",
	"Identity performing this action, recorded for the audit trail").
	Required().
	Env("ORBITAL_ACTOR").
	Persistent().
	State()
