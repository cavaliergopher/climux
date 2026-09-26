// Package conformance holds only tests. They run command lines that
// well-known CLIs accept against climux models of those CLIs, to show
// which of them a program built on climux can reproduce.
//
// A case is Conformant, KnownBad or NotImplemented. A conformant case must
// match the real tool, a known-bad case must keep failing as it did, and a
// not-implemented case is skipped until climux can express it. REPORT.md
// lists every case by state and is regenerated with:
//
//	go test ./conformance -run Report -update
package conformance
