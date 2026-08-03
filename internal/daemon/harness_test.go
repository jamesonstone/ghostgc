package daemon_test

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/daemon"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const testUID = 501

var t0 = time.Date(2026, 8, 3, 14, 0, 0, 0, time.UTC)

func mk(pid, ppid int, exec string, offset time.Duration) process.Process {
	return process.Process{
		PID: pid, PPID: ppid, PGID: pid, SID: pid, UID: testUID,
		StartTime: t0.Add(offset), ExecPath: exec, Comm: filepath.Base(exec),
		Args: []string{exec}, CWD: "/repo", Detailed: true,
		Status: process.StatusSleeping, RSSBytes: 4 << 20, Threads: 3,
	}
}

func codexRoot(pid, ppid int, offset time.Duration) process.Process {
	p := mk(pid, ppid, "/opt/homebrew/bin/codex", offset)
	p.Env = map[string]string{
		"CODEX_MANAGED_PACKAGE_ROOT": "/opt/homebrew/lib/node_modules/@openai/codex",
		"CODEX_HOME":                 "/Users/dev/.codex",
	}
	return p
}

func snapshot(at time.Duration, procs ...process.Process) *process.Snapshot {
	return process.NewSnapshot(t0.Add(at), procs, len(procs)+900)
}

type harness struct {
	d     *daemon.Daemon
	store *storage.Store
	fake  *platformtest.Fake
	paths config.Paths
}

func newHarness(t *testing.T, snaps ...*process.Snapshot) *harness {
	t.Helper()
	dir := t.TempDir()
	// The socket lives in its own short directory: t.TempDir() embeds the test
	// name, and unix socket paths are capped near 104 bytes.
	sockDir, err := os.MkdirTemp("", "gg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })

	paths := config.Paths{
		Config:   filepath.Join(dir, "config.yaml"),
		StateDir: dir,
		LogDir:   filepath.Join(dir, "logs"),
		Socket:   filepath.Join(sockDir, "s.sock"),
		Database: filepath.Join(dir, "ghostgc.db"),
	}
	store, storeErr := storage.Open(paths.Database)
	if storeErr != nil {
		t.Fatalf("storage.Open: %v", storeErr)
	}
	t.Cleanup(func() { _ = store.Close() })

	fake := platformtest.New(testUID, snaps...)
	d, err := daemon.New(daemon.Options{
		Config:   config.Default(),
		Paths:    paths,
		Store:    store,
		Platform: fake,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	return &harness{d: d, store: store, fake: fake, paths: paths}
}

func withParent(p process.Process, ppid int) process.Process {
	p.PPID = ppid
	return p
}

type errUnavailable struct{}
