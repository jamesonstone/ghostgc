package daemon

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func TestRelativeStateDirectoryProducesAbsoluteGitSnapshotPath(t *testing.T) {
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	relative, err := filepath.Rel(current, root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := worktreeSnapshotDir(config.Paths{StateDir: relative})
	if err != nil || !filepath.IsAbs(snapshot) {
		t.Fatalf("snapshot path = %q, %v", snapshot, err)
	}
	store, err := storage.Open(filepath.Join(root, "ghostgc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	d, err := New(Options{
		Config: config.Default(), Paths: config.Paths{StateDir: relative, Database: store.Path()},
		Store: store, Platform: platformtest.New(501),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil || d.worktreeGitErr != nil {
		t.Fatalf("daemon with relative state directory = %v, Git = %v", err, d.worktreeGitErr)
	}
}
