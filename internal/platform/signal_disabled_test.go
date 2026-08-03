package platform_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/platform"
)

// The daemon in this delivery phase must have no way to signal a process. The
// two tests below check that from both directions: the running implementation
// refuses, and no source file outside the platform package even references a
// signalling primitive.

func TestPlatformRefusesToSignal(t *testing.T) {
	p, err := platform.New(platform.Options{})
	if err != nil {
		t.Skipf("no platform implementation for this host: %v", err)
	}
	// Signal 0 delivers nothing even on a platform that permitted it, so this
	// call is safe to make against the test process itself.
	err = p.SignalProcess(context.Background(), os.Getpid(), 0)
	if !errors.Is(err, platform.ErrSignalingDisabled) {
		t.Fatalf("SignalProcess returned %v, want ErrSignalingDisabled", err)
	}
	for _, sig := range []platform.Signal{platform.SIGTERM, platform.SIGKILL} {
		if err := p.SignalProcess(context.Background(), os.Getpid(), sig); !errors.Is(err, platform.ErrSignalingDisabled) {
			t.Fatalf("SignalProcess(%v) returned %v, want ErrSignalingDisabled", sig, err)
		}
	}
}

// forbidden are the ways a Go program can terminate another process. None of
// them may appear outside internal/platform, and inside it they may appear
// only in the refusal itself.
var forbidden = []string{
	"syscall.Kill",
	"unix.Kill",
	"SYS_KILL",
	".Process.Kill(",
	".Process.Signal(",
	"process.Kill()",
	`"pkill"`,
	`"killall"`,
}

func TestNoSourceFileCanSignalAProcess(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(root, "internal", "platform", "signal_disabled_test.go")

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || path == self {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, needle := range forbidden {
			if strings.Contains(text, needle) {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s references %q; this build must contain no code path that can signal a process", rel, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// exec.Command("kill", ...) would be an equally effective way to violate the
// invariant, so shelling out to a terminator is checked separately.
func TestNoSourceFileShellsOutToATerminator(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(root, "internal", "platform", "signal_disabled_test.go")

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
			return fs.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || path == self {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(body), "\n") {
			if !strings.Contains(line, "exec.Command") {
				continue
			}
			for _, bad := range []string{`"kill"`, `"pkill"`, `"killall"`, `"launchctl", "kill"`} {
				if strings.Contains(line, bad) {
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s shells out to a terminator: %s", rel, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
