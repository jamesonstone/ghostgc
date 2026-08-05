package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStartupProfilesSupplyExactCodexDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"shell_snapshots", "worktrees"} {
		if err := os.MkdirAll(filepath.Join(home, ".codex", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	audit, err := startupDefaults(StartupAudit)
	if err != nil {
		t.Fatal(err)
	}
	assertStartupProfile(t, audit, ModeAudit)
	wantCache, err := filepath.EvalSymlinks(filepath.Join(home, ".codex", "shell_snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Cache.Roots) != 1 || audit.Cache.Roots[0] != wantCache {
		t.Fatalf("audit cache roots = %v, want [%s]", audit.Cache.Roots, wantCache)
	}
	wantWorktrees, err := filepath.EvalSymlinks(filepath.Join(home, ".codex", "worktrees"))
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Worktrees.Roots) != 1 || audit.Worktrees.Roots[0] != wantWorktrees {
		t.Fatalf("audit worktree roots = %v, want [%s]", audit.Worktrees.Roots, wantWorktrees)
	}

	reconcile, err := startupDefaults(StartupReconcile)
	if err != nil {
		t.Fatal(err)
	}
	assertStartupProfile(t, reconcile, ModeRecommend)
}

func TestStartupDefaultsIgnoreUnavailableOrLinkedRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := startupDefaults(StartupAudit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cache.Enabled || len(cfg.Cache.Roots) != 0 || len(cfg.Worktrees.Roots) != 0 {
		t.Fatalf("missing Codex directories granted authority: cache=%+v worktrees=%+v", cfg.Cache, cfg.Worktrees)
	}

	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, ".codex", "shell_snapshots")); err != nil {
		t.Fatal(err)
	}
	cfg, err = startupDefaults(StartupAudit)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cache.Enabled {
		t.Fatal("a linked default cache root granted authority")
	}
}

func TestStartupOverlayCanNarrowBuiltInDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"shell_snapshots", "worktrees"} {
		if err := os.MkdirAll(filepath.Join(home, ".codex", name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := writeConfig(t, "version: 1\nglobalMode: disabled\npolicies: []\ncache:\n  enabled: false\n  policies: []\nworktrees:\n  roots: []\n")
	cfg, err := LoadForStartup(path, StartupReconcile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GlobalMode != ModeDisabled || cfg.Cache.Enabled || len(cfg.Policies) != 0 || len(cfg.Worktrees.Roots) != 0 {
		t.Fatalf("configuration did not narrow startup defaults: %+v", cfg)
	}
}

func TestGeneratedOverlayPreservesStartupProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".codex", "shell_snapshots"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadForStartup(writeConfig(t, Example()), StartupReconcile)
	if err != nil {
		t.Fatal(err)
	}
	assertStartupProfile(t, cfg, ModeRecommend)
}

func TestStartupModesCapConfiguredEnforcement(t *testing.T) {
	cfg := Default()
	cfg.GlobalMode = ModeEnforce
	policy := validPolicy()
	policy.Mode, policy.Automatic = ModeEnforce, true
	cfg.Policies = []Policy{policy}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, string(raw))

	audit, err := LoadForStartup(path, StartupAudit)
	if err != nil {
		t.Fatal(err)
	}
	if audit.GlobalMode != ModeAudit || audit.Policies[0].Mode != ModeAudit || audit.Policies[0].Automatic {
		t.Fatalf("audit ceiling = %+v", audit.Policies[0])
	}
	reconcile, err := LoadForStartup(path, StartupReconcile)
	if err != nil {
		t.Fatal(err)
	}
	if reconcile.GlobalMode != ModeRecommend || reconcile.Policies[0].Mode != ModeRecommend || reconcile.Policies[0].Automatic {
		t.Fatalf("reconcile ceiling = %+v", reconcile.Policies[0])
	}
}

func assertStartupProfile(t *testing.T, cfg Config, want Mode) {
	t.Helper()
	if cfg.GlobalMode != want || len(cfg.Policies) != 1 || cfg.Policies[0].Mode != want || !cfg.Policies[0].Enabled {
		t.Fatalf("process startup profile = mode %q policies %+v; want %q", cfg.GlobalMode, cfg.Policies, want)
	}
	if !cfg.Cache.Enabled || cfg.Cache.GlobalMode != want || len(cfg.Cache.Policies) != 1 || cfg.Cache.Policies[0].Mode != want {
		t.Fatalf("cache startup profile = %+v; want %q", cfg.Cache, want)
	}
	if cfg.GlobalMode == ModeEnforce || cfg.Policies[0].Automatic {
		t.Fatal("startup profile enabled automatic authority")
	}
}
