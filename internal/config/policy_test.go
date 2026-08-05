package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validPolicy() Policy {
	return Policy{ID: "headless-browser", Description: "audit completed browser",
		Enabled: true, Mode: ModeAudit, States: []string{"orphaned"}, Agents: []string{"codex"},
		Executables: []string{"chrome-headless-shell"}, RequireDetached: true,
		RequireSessionEnded: true, MinStable: Duration(5 * time.Minute), Cooldown: Duration(time.Hour)}
}

func TestPolicyValidationFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config, *Policy)
		want   string
	}{
		{name: "automatic audit", change: func(_ *Config, p *Policy) { p.Automatic = true }, want: "requires mode enforce"},
		{name: "enforce without automatic", change: func(_ *Config, p *Policy) { p.Mode = ModeEnforce }, want: "requires automatic"},
		{name: "enabled disabled mode", change: func(_ *Config, p *Policy) { p.Mode = ModeDisabled }, want: "enabled but mode is disabled"},
		{name: "weak state", change: func(_ *Config, p *Policy) { p.States = []string{"idle"} }, want: "not policy-eligible"},
		{name: "working state", change: func(_ *Config, p *Policy) { p.States = []string{"suspicious"} }, want: "not policy-eligible"},
		{name: "broad runtime", change: func(_ *Config, p *Policy) { p.Executables = []string{"node"} }, want: "protected broad class"},
		{name: "path executable", change: func(_ *Config, p *Policy) { p.Executables = []string{"/tmp/helper"} }, want: "exact basename"},
		{name: "unknown agent", change: func(_ *Config, p *Policy) { p.Agents = []string{"other"} }, want: "not enabled"},
		{name: "short window", change: func(_ *Config, p *Policy) { p.MinStable = Duration(time.Minute) }, want: "at least 5m"},
		{name: "short cooldown", change: func(_ *Config, p *Policy) { p.Cooldown = 0 }, want: "at least 1m"},
		{name: "duplicate state", change: func(_ *Config, p *Policy) { p.States = append(p.States, p.States[0]) }, want: "duplicate state"},
		{name: "duplicate agent", change: func(_ *Config, p *Policy) { p.Agents = append(p.Agents, p.Agents[0]) }, want: "duplicate agent"},
		{name: "duplicate executable", change: func(_ *Config, p *Policy) { p.Executables = append(p.Executables, p.Executables[0]) }, want: "duplicate executable"},
		{name: "duplicate", change: func(c *Config, p *Policy) { c.Policies = append(c.Policies, *p) }, want: "duplicate policy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			policy := validPolicy()
			cfg.Policies = []Policy{policy}
			tt.change(&cfg, &cfg.Policies[0])
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEnforcePolicyValidationIsNarrow(t *testing.T) {
	valid := validPolicy()
	valid.Mode, valid.Automatic = ModeEnforce, true
	tests := []struct {
		name string
		edit func(*Policy)
		want string
	}{
		{"multiple states", func(p *Policy) { p.States = []string{"orphaned", "crashed"} }, "one orphaned state"},
		{"crashed", func(p *Policy) { p.States = []string{"crashed"} }, "one orphaned state"},
		{"multiple agents", func(p *Policy) { p.Agents = []string{"codex", "codex"} }, "duplicate agent"},
		{"multiple executables", func(p *Policy) { p.Executables = []string{"helper", "worker"} }, "one executable"},
		{"attached", func(p *Policy) { p.RequireDetached = false }, "state \"orphaned\" requires"},
		{"active session", func(p *Policy) { p.RequireSessionEnded = false }, "state \"orphaned\" requires"},
		{"short cooldown", func(p *Policy) { p.Cooldown = Duration(time.Minute) }, "at least 1h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, policy := Default(), valid
			cfg.GlobalMode = ModeEnforce
			tt.edit(&policy)
			cfg.Policies = []Policy{policy}
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want %q", err, tt.want)
			}
		})
	}
	cfg := Default()
	cfg.GlobalMode, cfg.Policies = ModeEnforce, []Policy{valid}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("narrow enforce policy rejected: %v", err)
	}
	second := valid
	second.ID = "second-policy"
	cfg.Policies = append(cfg.Policies, second)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Fatalf("multiple enabled enforce policies = %v", err)
	}
}

func TestPolicyConfigIsStrictAndExampleLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	raw := Example() + "\nunknownPolicyField: true\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "field unknownPolicyField") {
		t.Fatalf("unknown YAML field was accepted: %v", err)
	}
	if err := os.WriteFile(path, []byte(Example()), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || len(cfg.Policies) != 0 {
		t.Fatalf("example policy = %+v, %v", cfg.Policies, err)
	}
}
