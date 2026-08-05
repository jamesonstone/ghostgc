package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ByteSize is a byte bound that accepts integers or KiB, MiB and GiB strings.
type ByteSize int64

// UnmarshalYAML implements yaml.Unmarshaler.
func (b *ByteSize) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag == "!!int" {
		var value int64
		if err := node.Decode(&value); err != nil {
			return err
		}
		*b = ByteSize(value)
		return nil
	}
	var raw string
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("config: cache byte bound must be an integer or size string")
	}
	multipliers := map[string]int64{"KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30}
	for suffix, multiplier := range multipliers {
		if strings.HasSuffix(raw, suffix) {
			value, err := strconv.ParseInt(strings.TrimSuffix(raw, suffix), 10, 64)
			if err != nil || value <= 0 || value > math.MaxInt64/multiplier {
				return fmt.Errorf("config: invalid cache byte bound %q", raw)
			}
			*b = ByteSize(value * multiplier)
			return nil
		}
	}
	return fmt.Errorf("config: invalid cache byte bound %q; use KiB, MiB or GiB", raw)
}

// MarshalYAML implements yaml.Marshaler.
func (b ByteSize) MarshalYAML() (any, error) { return int64(b), nil }

// CachePolicy is one exact cache-artifact selector.
type CachePolicy struct {
	ID           string `yaml:"id" json:"id"`
	Description  string `yaml:"description" json:"description"`
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Mode         Mode   `yaml:"mode" json:"mode"`
	Provider     string `yaml:"provider" json:"provider"`
	Agent        string `yaml:"agent" json:"agent"`
	ArtifactKind string `yaml:"artifactKind" json:"artifact_kind"`
	SessionState string `yaml:"sessionState" json:"session_state"`
}

// Cache controls the independent cache observation and action lane.
type Cache struct {
	Enabled             bool          `yaml:"enabled" json:"enabled"`
	GlobalMode          Mode          `yaml:"globalMode" json:"global_mode"`
	ScanInterval        Duration      `yaml:"scanInterval" json:"scan_interval"`
	MinStable           Duration      `yaml:"minStable" json:"min_stable"`
	QuarantineGrace     Duration      `yaml:"quarantineGrace" json:"quarantine_grace"`
	MaxEntriesPerScan   int           `yaml:"maxEntriesPerScan" json:"max_entries_per_scan"`
	MaxEntriesPerAction int           `yaml:"maxEntriesPerAction" json:"max_entries_per_action"`
	MaxBytesPerAction   ByteSize      `yaml:"maxBytesPerAction" json:"max_bytes_per_action"`
	Policies            []CachePolicy `yaml:"policies" json:"policies"`
}

const (
	maxCacheScanEntries   = 100_000
	maxCacheActionEntries = 1_000
	maxCacheActionBytes   = ByteSize(1 << 40)
)

// DefaultCache returns a disabled cache lane with conservative bounds.
func DefaultCache() Cache {
	return Cache{
		Enabled:             false,
		GlobalMode:          ModeAudit,
		ScanInterval:        Duration(30 * time.Minute),
		MinStable:           Duration(24 * time.Hour),
		QuarantineGrace:     Duration(7 * 24 * time.Hour),
		MaxEntriesPerScan:   10_000,
		MaxEntriesPerAction: 1_000,
		MaxBytesPerAction:   ByteSize(10 << 30),
	}
}

// Digest binds approvals to the complete cache authority configuration.
func (c Cache) Digest() string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Validate enforces cache-specific authority and exact policy selectors.
func (c Cache) Validate(agents map[string]Agent) error {
	if c.GlobalMode != ModeDisabled && c.GlobalMode != ModeAudit && c.GlobalMode != ModeRecommend {
		return fmt.Errorf("cache.globalMode %q is not one of disabled, audit, recommend", c.GlobalMode)
	}
	if c.ScanInterval.D() < time.Minute || c.MinStable.D() < time.Minute || c.QuarantineGrace.D() < time.Minute {
		return fmt.Errorf("cache durations must each be at least 1m")
	}
	if c.MaxEntriesPerScan < 1 || c.MaxEntriesPerAction < 1 || c.MaxBytesPerAction < 1 {
		return fmt.Errorf("cache traversal and action bounds must be positive")
	}
	if c.MaxEntriesPerScan > maxCacheScanEntries || c.MaxEntriesPerAction > maxCacheActionEntries || c.MaxBytesPerAction > maxCacheActionBytes {
		return fmt.Errorf("cache bounds exceed hard maxima of %d scan entries, %d action entries and 1TiB",
			maxCacheScanEntries, maxCacheActionEntries)
	}
	ids := make(map[string]bool)
	enabled := 0
	for i, policy := range c.Policies {
		prefix := fmt.Sprintf("cache.policies[%d]", i)
		if !policyIDPattern.MatchString(policy.ID) || ids[policy.ID] {
			return fmt.Errorf("%s.id %q must be unique and match %s", prefix, policy.ID, policyIDPattern)
		}
		ids[policy.ID] = true
		if strings.TrimSpace(policy.Description) == "" {
			return fmt.Errorf("%s.description is required", prefix)
		}
		if policy.Mode != ModeDisabled && policy.Mode != ModeAudit && policy.Mode != ModeRecommend {
			return fmt.Errorf("%s.mode %q is not one of disabled, audit, recommend", prefix, policy.Mode)
		}
		if policy.Enabled && policy.Mode == ModeDisabled {
			return fmt.Errorf("%s is enabled but mode is disabled", prefix)
		}
		if policy.Enabled {
			enabled++
		}
		if policy.Provider != "codex-shell-snapshot-v1" || policy.Agent != "codex" || policy.ArtifactKind != "shell-snapshot" || policy.SessionState != "completed" {
			return fmt.Errorf("%s must exactly select provider codex-shell-snapshot-v1, agent codex, artifactKind shell-snapshot and sessionState completed", prefix)
		}
		if agent, ok := agents[policy.Agent]; !ok || !agent.Enabled {
			return fmt.Errorf("%s agent %q is not enabled", prefix, policy.Agent)
		}
	}
	if enabled > 1 {
		return fmt.Errorf("only one enabled exact cache policy is allowed")
	}
	return nil
}
