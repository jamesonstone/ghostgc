package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/config"
)

func TestConfigShowFallsBackToDefaultAuditStartupProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"shell_snapshots", "worktrees"} {
		if err := os.MkdirAll(filepath.Join(home, ".codex", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	paths := config.Paths{Config: filepath.Join(t.TempDir(), "config.yaml")}
	e := &env{paths: paths, socket: filepath.Join(t.TempDir(), "missing.sock")}
	output, _ := captureStdout(t, func() int {
		if err := configShow(context.Background(), e); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	for _, want := range []string{
		"Source: default start preview", "Startup profile: audit", "Global mode: audit",
		"Cache lifecycle: enabled=true mode=audit", "policies=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("config output is missing %q:\n%s", want, output)
		}
	}
}
