package config

import (
	"strings"
	"testing"
	"time"
)

func TestCacheDefaultsAreDisabledAndBounded(t *testing.T) {
	cfg := Default()
	if cfg.Cache.Enabled || cfg.Cache.GlobalMode != ModeAudit {
		t.Fatalf("cache defaults = enabled %t mode %q, want disabled audit", cfg.Cache.Enabled, cfg.Cache.GlobalMode)
	}
	if cfg.Cache.MaxEntriesPerScan < 1 || cfg.Cache.MaxEntriesPerAction < 1 || cfg.Cache.MaxBytesPerAction < 1 {
		t.Fatal("cache defaults must retain explicit traversal and action bounds")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default cache configuration must validate: %v", err)
	}
}

func TestCacheRejectsEnforceAndAutomaticAuthority(t *testing.T) {
	for name, body := range map[string]string{
		"global enforce": "version: 1\ncache:\n  globalMode: enforce\n",
		"policy enforce": validCacheYAML("enforce") + "",
		"automatic key":  "version: 1\ncache:\n  automatic: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body)); err == nil {
				t.Fatalf("%s must be rejected", name)
			}
		})
	}
}

func TestCacheAcceptsOnlyExactProvenSelector(t *testing.T) {
	cfg, err := Load(writeConfig(t, validCacheYAML("recommend")))
	if err != nil {
		t.Fatalf("exact cache policy must load: %v", err)
	}
	if !cfg.Cache.Enabled || len(cfg.Cache.Policies) != 1 {
		t.Fatalf("loaded cache config = %#v", cfg.Cache)
	}

	broken := strings.Replace(validCacheYAML("recommend"), "shell-snapshot", "attachment", 1)
	if _, err := Load(writeConfig(t, broken)); err == nil || !strings.Contains(err.Error(), "must exactly select") {
		t.Fatalf("broadened selector must fail closed, got %v", err)
	}
}

func TestCacheRequiresExactApprovedRootsWhenEnabled(t *testing.T) {
	cfg := Default()
	cfg.Cache.Enabled = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cache.roots") {
		t.Fatalf("enabled cache without roots must fail closed, got %v", err)
	}
	cfg.Cache.Roots = []string{"relative/shell_snapshots"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative cache root must be rejected")
	}
	cfg.Cache.Roots = []string{"/tmp/codex/cache"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("non-provider cache root must be rejected")
	}
}

func TestCacheRequiresTwoBoundedObservationIntervals(t *testing.T) {
	for name, mutate := range map[string]func(*Cache){
		"scan":           func(c *Cache) { c.ScanInterval = Duration(time.Second) },
		"stable":         func(c *Cache) { c.MinStable = Duration(time.Second) },
		"grace":          func(c *Cache) { c.QuarantineGrace = Duration(time.Second) },
		"scan bound":     func(c *Cache) { c.MaxEntriesPerScan = 0 },
		"byte bound":     func(c *Cache) { c.MaxBytesPerAction = 0 },
		"scan maximum":   func(c *Cache) { c.MaxEntriesPerScan = maxCacheScanEntries + 1 },
		"action maximum": func(c *Cache) { c.MaxEntriesPerAction = maxCacheActionEntries + 1 },
		"byte maximum":   func(c *Cache) { c.MaxBytesPerAction = maxCacheActionBytes + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			mutate(&cfg.Cache)
			if err := cfg.Validate(); err == nil {
				t.Fatal("unsafe cache bound must be rejected")
			}
		})
	}
}

func TestCacheConfigurationDigestBindsAuthority(t *testing.T) {
	cache := DefaultCache()
	before := cache.Digest()
	cache.MaxEntriesPerAction++
	if cache.Digest() == before {
		t.Fatal("cache authority digest must change with an action bound")
	}
}

func TestCacheByteSizeParsingAndOverflow(t *testing.T) {
	cfg, err := Load(writeConfig(t, "version: 1\ncache:\n  maxBytesPerAction: 12MiB\n"))
	if err != nil || cfg.Cache.MaxBytesPerAction != 12<<20 {
		t.Fatalf("12MiB parse = %d, %v", cfg.Cache.MaxBytesPerAction, err)
	}
	if _, err := Load(writeConfig(t, "version: 1\ncache:\n  maxBytesPerAction: 9223372036854775807GiB\n")); err == nil {
		t.Fatal("overflowing cache byte bound must be rejected")
	}
}

func validCacheYAML(mode string) string {
	return "version: 1\n" +
		"cache:\n" +
		"  enabled: true\n" +
		"  globalMode: recommend\n" +
		"  roots: [/tmp/codex/shell_snapshots]\n" +
		"  policies:\n" +
		"    - id: codex-snapshots\n" +
		"      description: completed Codex shell snapshots\n" +
		"      enabled: true\n" +
		"      mode: " + mode + "\n" +
		"      provider: codex-shell-snapshot-v1\n" +
		"      agent: codex\n" +
		"      artifactKind: shell-snapshot\n" +
		"      sessionState: completed\n"
}
