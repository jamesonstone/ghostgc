package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

// StartupMode selects the maximum authority available to a daemon instance.
type StartupMode string

const (
	StartupAudit     StartupMode = "audit"
	StartupReconcile StartupMode = "reconcile"
)

// ParseStartupMode accepts the two canonical modes and their user-facing
// synonyms. The returned value is always canonical.
func ParseStartupMode(value string) (StartupMode, error) {
	switch value {
	case "", "audit", "shadow":
		return StartupAudit, nil
	case "reconcile", "live":
		return StartupReconcile, nil
	default:
		return "", fmt.Errorf("startup mode %q is not one of audit or reconcile", value)
	}
}

// LoadForStartup overlays an optional strict YAML file on the selected
// built-in profile, then applies the mode as an authority ceiling.
func LoadForStartup(path string, mode StartupMode) (Config, error) {
	cfg, err := startupDefaults(mode)
	if err != nil {
		return Config{}, err
	}
	cfg, err = loadWithDefaults(path, cfg)
	if err != nil {
		return Config{}, err
	}
	applyStartupCeiling(&cfg, mode)
	cfg.StartupMode = mode
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// startupDefaults returns the no-file profile for one startup mode.
func startupDefaults(mode StartupMode) (Config, error) {
	if mode != StartupAudit && mode != StartupReconcile {
		return Config{}, fmt.Errorf("startup mode %q is not one of audit or reconcile", mode)
	}
	cfg := Default()
	actionMode := ModeAudit
	if mode == StartupReconcile {
		actionMode = ModeRecommend
		cfg.GlobalMode = ModeRecommend
	}
	cfg.Policies = []Policy{defaultProcessPolicy(actionMode)}

	if root, ok := standardCodexDirectory("shell_snapshots"); ok {
		cfg.Cache.Enabled = true
		cfg.Cache.GlobalMode = actionMode
		cfg.Cache.Roots = []string{root}
		cfg.Cache.Policies = []CachePolicy{defaultCachePolicy(actionMode)}
	}
	if root, ok := standardCodexDirectory("worktrees"); ok {
		cfg.Worktrees.Roots = []string{root}
	}
	return cfg, cfg.Validate()
}

func defaultProcessPolicy(mode Mode) Policy {
	return Policy{
		ID: "completed-codex-headless-browser", Description: "completed Codex headless browser",
		Enabled: true, Mode: mode, States: []string{"orphaned"}, Agents: []string{"codex"},
		Executables: []string{"chrome-headless-shell"}, RequireDetached: true,
		RequireSessionEnded: true, MinStable: Duration(5 * time.Minute), Cooldown: Duration(time.Hour),
	}
}

func defaultCachePolicy(mode Mode) CachePolicy {
	return CachePolicy{
		ID: "codex-shell-snapshots", Description: "completed Codex shell snapshots",
		Enabled: true, Mode: mode, Provider: cacheartifact.ProviderCodexShellSnapshot,
		Agent: cacheartifact.AgentCodex, ArtifactKind: cacheartifact.KindShellSnapshot,
		SessionState: "completed",
	}
}

func standardCodexDirectory(name string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	codexHome := filepath.Join(home, ".codex")
	info, err := os.Lstat(codexHome)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	candidate := filepath.Join(codexHome, name)
	info, err = os.Lstat(candidate)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	return canonical, true
}

func applyStartupCeiling(cfg *Config, mode StartupMode) {
	ceiling := ModeAudit
	if mode == StartupReconcile {
		ceiling = ModeRecommend
	}
	cfg.GlobalMode = cappedMode(cfg.GlobalMode, ceiling)
	cfg.Cache.GlobalMode = cappedMode(cfg.Cache.GlobalMode, ceiling)
	for i := range cfg.Policies {
		capped := cappedMode(cfg.Policies[i].Mode, ceiling)
		if capped != cfg.Policies[i].Mode {
			cfg.Policies[i].Mode = capped
			cfg.Policies[i].Automatic = false
		}
	}
	for i := range cfg.Cache.Policies {
		cfg.Cache.Policies[i].Mode = cappedMode(cfg.Cache.Policies[i].Mode, ceiling)
	}
}

func cappedMode(current, ceiling Mode) Mode {
	switch ceiling {
	case ModeAudit:
		if current == ModeRecommend || current == ModeEnforce {
			return ModeAudit
		}
	case ModeRecommend:
		if current == ModeEnforce {
			return ModeRecommend
		}
	}
	return current
}
