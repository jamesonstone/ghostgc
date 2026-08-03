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
	"github.com/jamesonstone/ghostgc/internal/process"
)

func TestPlatformRejectsNonTERMAndChangedIdentity(t *testing.T) {
	p, err := platform.New(platform.Options{})
	if err != nil {
		t.Skipf("no platform implementation for this host: %v", err)
	}
	invalid := process.Key{PID: os.Getpid(), StartTimeNs: 1}
	invalidExecutable := process.ExecutableIdentity{ExecPath: "/invalid", Comm: "invalid"}
	if err := p.SignalProcess(context.Background(), invalid, invalidExecutable, platform.Signal(-1)); !errors.Is(err, platform.ErrSignalNotAllowed) {
		t.Fatalf("non-TERM signal returned %v, want ErrSignalNotAllowed", err)
	}
	if err := p.SignalProcess(context.Background(), invalid, invalidExecutable, platform.SIGTERM); err == nil {
		t.Fatal("changed exact identity was accepted")
	}
}

func TestSignalPrimitiveIsSingleLiteralSIGTERM(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	allowed := filepath.Join(root, "internal", "platform", "darwin", "signal.go")
	var sites int
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
		if filepath.Ext(path) != ".go" || path == filepath.Join(root, "internal", "platform", "signal_gate_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		scanned := text
		if path == allowed {
			scanned = strings.Replace(scanned, "syscall.Kill(key.PID, syscall.SIGTERM)", "", 1)
		}
		scanned = strings.ReplaceAll(scanned, "platform.Signal(", "")
		for _, forbidden := range []string{"SIGKILL", "unix.Kill", "SYS_KILL", ".Kill(", ".Signal(", `"kill"`, `"pkill"`, `"killall"`} {
			if strings.Contains(scanned, forbidden) {
				t.Errorf("%s references forbidden terminator %q", path, forbidden)
			}
		}
		if count := strings.Count(text, "syscall.Kill"); count > 0 {
			sites += count
			if path != allowed || count != 1 || strings.Count(text, "syscall.Kill(key.PID, syscall.SIGTERM)") != 1 {
				t.Errorf("%s has a non-literal or unauthorized signal site", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sites != 1 {
		t.Fatalf("found %d signalling sites, want exactly one", sites)
	}
}

func TestNoSourceFileShellsOutToATerminator(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor") {
			return fs.SkipDir
		}
		if d.IsDir() || filepath.Ext(path) != ".go" {
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
					t.Errorf("%s shells out to a terminator: %s", path, strings.TrimSpace(line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
