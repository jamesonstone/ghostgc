// Package version carries build identification.
package version

import "runtime/debug"

// Version is set at build time with -ldflags "-X .../version.Version=v0.1.0".
var Version = "dev"

// PhaseNumber is the delivery phase this build implements.
const PhaseNumber = 6

// Phase names the delivery phase this build implements. It is displayed
// wherever the daemon reports its action-authority boundary.
//
// It is a constant, so it goes stale silently unless something checks it;
// version_test.go does.
const Phase = "6 — manually approved cleanup (full revalidation, SIGTERM only)"

// Revision returns the VCS revision recorded by the Go toolchain, if any.
func Revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			if len(s.Value) > 12 {
				return s.Value[:12]
			}
			return s.Value
		}
	}
	return ""
}

// String returns a display string for the CLI and daemon banner.
func String() string {
	if rev := Revision(); rev != "" {
		return Version + " (" + rev + ")"
	}
	return Version
}
