//go:build darwin

package darwin

import "testing"

func TestParseLaunchctlListAcceptsQuotedPaddedKeys(t *testing.T) {
	out := []byte(`{
	"PID" = 71164;
	"LastExitStatus" = 7;
}`)
	pid, lastExit := parseLaunchctlList(out)
	if pid != 71164 || lastExit != 7 {
		t.Fatalf("launchctl state = pid %d exit %d, want pid 71164 exit 7", pid, lastExit)
	}
}
