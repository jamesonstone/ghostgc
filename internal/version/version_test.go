package version

import (
	"strconv"
	"strings"
	"testing"
)

// Phase is displayed by `ghostgc status`, `ghostgc version`, the CLI usage
// banner and the daemon's startup audit entry. It is a hand-maintained
// constant, so without a check it goes stale the moment a phase lands — which
// is exactly what happened at the end of phase 2, where the daemon was still
// announcing itself as phase 1.
func TestPhaseStringMatchesPhaseNumber(t *testing.T) {
	if !strings.HasPrefix(Phase, strconv.Itoa(PhaseNumber)) {
		t.Fatalf("Phase = %q but PhaseNumber = %d; bump both together", Phase, PhaseNumber)
	}
}

// Every phase before 6 must say so where a user can see it.
func TestPhaseAdvertisesThatNothingCanBeTerminated(t *testing.T) {
	if PhaseNumber >= 6 {
		t.Skip("phase 6 introduces manually approved termination; this assertion no longer holds")
	}
	if !strings.Contains(Phase, "no process termination code is present") {
		t.Fatalf("Phase = %q; a build that cannot terminate anything must say so", Phase)
	}
}

func TestPhaseSevenNamesItsAutomaticBound(t *testing.T) {
	if PhaseNumber == 7 && (!strings.Contains(Phase, "one candidate per evaluation") || !strings.Contains(Phase, "SIGTERM only")) {
		t.Fatalf("Phase 7 must advertise its automatic authority bound: %q", Phase)
	}
}

func TestStringIncludesVersion(t *testing.T) {
	if !strings.Contains(String(), Version) {
		t.Fatalf("String() = %q, want it to contain the version %q", String(), Version)
	}
}
