//go:build darwin

package darwin

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSignalProcessRejectsChangedExecutableAtFinalGate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	collector, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var observedErr error
	for range 20 {
		observed, err := collector.InspectProcess(context.Background(), cmd.Process.Pid)
		if err != nil {
			observedErr = err
			time.Sleep(10 * time.Millisecond)
			continue
		}
		executable, ok := observed.Executable()
		if !ok {
			t.Fatal("fixture executable identity is unavailable")
		}
		executable.ExecPath += ".changed"
		err = collector.SignalProcess(context.Background(), observed.Key(), executable, syscall.SIGTERM)
		if err == nil || !strings.Contains(err.Error(), "executable changed") {
			t.Fatalf("changed executable returned %v", err)
		}
		return
	}
	t.Fatalf("fixture process was not inspectable: %v", observedErr)
}
