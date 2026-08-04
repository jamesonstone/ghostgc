package version

import (
	"strings"
	"testing"
)

func TestStringIncludesVersion(t *testing.T) {
	if !strings.Contains(String(), Version) {
		t.Fatalf("String() = %q, want it to contain the version %q", String(), Version)
	}
}
