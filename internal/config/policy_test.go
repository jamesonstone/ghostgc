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
		{name: "recommend mode", change: func(_ *Config, p *Policy) { p.Mode = ModeRecommend }, want: "unavailable"},
		{name: "weak state", change: func(_ *Config, p *Policy) { p.States = []string{"idle"} }, want: "not policy-eligible"},
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
	if err != nil || len(cfg.Policies) != 1 || cfg.Policies[0].Enabled {
		t.Fatalf("example policy = %+v, %v", cfg.Policies, err)
	}
}
