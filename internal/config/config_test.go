package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsAreAuditOnly(t *testing.T) {
	cfg := Default()
	if cfg.GlobalMode != ModeAudit {
		t.Fatalf("default globalMode = %q, want audit", cfg.GlobalMode)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the built-in defaults must validate: %v", err)
	}
	if cfg.Privacy.StoreSourceContents {
		t.Fatal("storeSourceContents must default to false")
	}
	if cfg.Privacy.NetworkTelemetry {
		t.Fatal("networkTelemetry must default to false")
	}
	if !cfg.Privacy.RedactEnvironmentValues {
		t.Fatal("redactEnvironmentValues must default to true")
	}
}

// The generated example is what users start from, so it must not merely be
// valid, it must be audit mode.
func TestExampleConfigLoadsAndIsAudit(t *testing.T) {
	path := writeConfig(t, Example())
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the generated example must load: %v", err)
	}
	if cfg.GlobalMode != ModeAudit {
		t.Fatalf("example globalMode = %q, want audit", cfg.GlobalMode)
	}
	if cfg.Defaulted {
		t.Fatal("a config read from disk must not be marked as defaulted")
	}
}

func TestRecommendAndEnforceModesAreAccepted(t *testing.T) {
	path := writeConfig(t, "version: 1\nglobalMode: recommend\n")
	if _, err := Load(path); err != nil {
		t.Fatalf("recommend mode must be available in phase 6: %v", err)
	}
	path = writeConfig(t, "version: 1\nglobalMode: enforce\n")
	if _, err := Load(path); err != nil {
		t.Fatalf("enforce mode must be available in phase 7: %v", err)
	}
}

func TestUnknownModeIsRejected(t *testing.T) {
	path := writeConfig(t, "version: 1\nglobalMode: yolo\n")
	if _, err := Load(path); err == nil {
		t.Fatal("an unknown globalMode must be rejected")
	}
}

func TestSourceContentsCannotBeEnabled(t *testing.T) {
	path := writeConfig(t, "version: 1\nprivacy:\n  storeSourceContents: true\n")
	_, err := Load(path)
	if err == nil {
		t.Fatal("privacy.storeSourceContents: true must be refused")
	}
	if !strings.Contains(err.Error(), "source-code contents") {
		t.Fatalf("the refusal should explain itself, got: %v", err)
	}
}

func TestTelemetryCannotBeEnabled(t *testing.T) {
	path := writeConfig(t, "version: 1\nprivacy:\n  networkTelemetry: true\n")
	if _, err := Load(path); err == nil {
		t.Fatal("privacy.networkTelemetry: true must be refused")
	}
}

func TestMissingFileFallsBackToSafeDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("a missing config must not be an error: %v", err)
	}
	if !cfg.Defaulted {
		t.Fatal("a missing config must be reported as defaulted")
	}
	if cfg.GlobalMode != ModeAudit {
		t.Fatalf("fallback globalMode = %q, want audit", cfg.GlobalMode)
	}
}

func TestPartialFileKeepsDefaults(t *testing.T) {
	path := writeConfig(t, "version: 1\nsampling:\n  processScan: 30s\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sampling.ProcessScan.D() != 30*time.Second {
		t.Fatalf("processScan = %s, want 30s", cfg.Sampling.ProcessScan.D())
	}
	if cfg.Sampling.PolicyEvaluation.D() != 5*time.Minute {
		t.Fatalf("policyEvaluation = %s, want the default 5m", cfg.Sampling.PolicyEvaluation.D())
	}
	if cfg.Retention.MaxDatabaseBytes != 250<<20 {
		t.Fatalf("maxDatabaseBytes = %d, want the default", cfg.Retention.MaxDatabaseBytes)
	}
}

func TestUnknownFieldIsRejected(t *testing.T) {
	path := writeConfig(t, "version: 1\nglobalMod: audit\n")
	if _, err := Load(path); err == nil {
		t.Fatal("a misspelled key must be an error, not a silently ignored setting")
	}
}

func TestAbsurdSamplingIntervalsAreRejected(t *testing.T) {
	path := writeConfig(t, "version: 1\nsampling:\n  processScan: 10ms\n")
	if _, err := Load(path); err == nil {
		t.Fatal("a sub-second scan interval must be rejected; it would defeat the CPU budget")
	}
}

func TestDurationParsing(t *testing.T) {
	path := writeConfig(t, "version: 1\nretention:\n  rawObservations: 90m\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Retention.RawObservations.D() != 90*time.Minute {
		t.Fatalf("rawObservations = %s, want 90m", cfg.Retention.RawObservations.D())
	}
}

func TestSocketPathLengthIsValidated(t *testing.T) {
	p := Paths{Socket: "/" + strings.Repeat("a", 200) + "/ghostgc.sock"}
	if err := p.Validate(); err == nil {
		t.Fatal("an over-long unix socket path must be reported up front")
	}
}

func TestResolvePathsAppliesOverrides(t *testing.T) {
	cfg := Default()
	cfg.Paths.StateDir = "/tmp/ghostgc-test"
	got := cfg.ResolvePaths(Paths{StateDir: "/original", Database: "/original/db", Socket: "/original/sock"})
	if got.StateDir != "/tmp/ghostgc-test" {
		t.Fatalf("StateDir = %q", got.StateDir)
	}
	if !strings.HasPrefix(got.Database, "/tmp/ghostgc-test/") {
		t.Fatalf("Database = %q, want it to follow the state directory", got.Database)
	}
}

func TestEnabledAgents(t *testing.T) {
	cfg := Default()
	cfg.Agents["cursor"] = Agent{Enabled: false}
	got := cfg.EnabledAgents()
	if len(got) != 1 || got[0] != "codex" {
		t.Fatalf("EnabledAgents() = %v, want [codex]", got)
	}
}
