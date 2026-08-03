// Package config loads and validates the ghostgc configuration.
//
// The configuration is the only place a user can widen what the daemon is
// allowed to do, so validation is where several of the phase-1 safety
// invariants are enforced. Anything that would permit an action is rejected
// with an error that names the delivery phase in which it becomes available.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Mode is the action level of the daemon as a whole, or of a single policy.
type Mode string

const (
	ModeDisabled  Mode = "disabled"
	ModeAudit     Mode = "audit"
	ModeRecommend Mode = "recommend"
	ModeEnforce   Mode = "enforce"
)

// Valid reports whether the mode is a known value.
func (m Mode) Valid() bool {
	switch m {
	case ModeDisabled, ModeAudit, ModeRecommend, ModeEnforce:
		return true
	}
	return false
}

// Duration is a time.Duration that round-trips through YAML as "15s", "5m".
type Duration time.Duration

// UnmarshalYAML implements yaml.Unmarshaler.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("config: %q is not a duration string such as \"15s\"", node.Value)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("config: invalid duration %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML implements yaml.Marshaler.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// Sampling controls the daemon's polling cadence.
type Sampling struct {
	ProcessScan      Duration `yaml:"processScan"`
	ActivitySample   Duration `yaml:"activitySample"`
	Classification   Duration `yaml:"classification"`
	PolicyEvaluation Duration `yaml:"policyEvaluation"`
	Retention        Duration `yaml:"retention"`
}

// Retention bounds how long observations are kept.
type Retention struct {
	RawObservations        Duration `yaml:"rawObservations"`
	AggregatedObservations Duration `yaml:"aggregatedObservations"`
	Actions                Duration `yaml:"actions"`
	// MaxDatabaseBytes is a hard ceiling. When the database exceeds it, the
	// retention pass compacts more aggressively and records the fact.
	MaxDatabaseBytes int64 `yaml:"maxDatabaseBytes"`
}

// Notifications controls which findings are surfaced to the user.
type Notifications struct {
	Suspicious  bool `yaml:"suspicious"`
	Hung        bool `yaml:"hung"`
	Candidate   bool `yaml:"candidate"`
	ActionTaken bool `yaml:"actionTaken"`
	Healthy     bool `yaml:"healthy"`
}

// Privacy controls what leaves process memory.
type Privacy struct {
	StoreCommandLines       bool `yaml:"storeCommandLines"`
	RedactEnvironmentValues bool `yaml:"redactEnvironmentValues"`
	StoreSourceContents     bool `yaml:"storeSourceContents"`
	NetworkTelemetry        bool `yaml:"networkTelemetry"`
}

// Agent enables or disables a single agent adapter.
type Agent struct {
	Enabled bool `yaml:"enabled"`
}

// PathOverrides lets a user or a test relocate the daemon's files.
type PathOverrides struct {
	StateDir string `yaml:"stateDir"`
	LogDir   string `yaml:"logDir"`
	Socket   string `yaml:"socket"`
	Database string `yaml:"database"`
}

// Config is the complete daemon configuration.
type Config struct {
	Version       int              `yaml:"version"`
	GlobalMode    Mode             `yaml:"globalMode"`
	Sampling      Sampling         `yaml:"sampling"`
	Retention     Retention        `yaml:"retention"`
	Notifications Notifications    `yaml:"notifications"`
	Privacy       Privacy          `yaml:"privacy"`
	Agents        map[string]Agent `yaml:"agents"`
	Policies      []Policy         `yaml:"policies"`
	Paths         PathOverrides    `yaml:"paths"`

	// SourcePath records where the configuration was loaded from. It is not a
	// YAML field.
	SourcePath string `yaml:"-"`
	// Defaulted reports that no configuration file existed and built-in
	// defaults are in use.
	Defaulted bool `yaml:"-"`
}

// Default returns the built-in configuration. Audit remains the safe default;
// recommendation must be selected explicitly in a configuration file.
func Default() Config {
	return Config{
		Version:    1,
		GlobalMode: ModeAudit,
		Sampling: Sampling{
			ProcessScan:      Duration(15 * time.Second),
			ActivitySample:   Duration(60 * time.Second),
			Classification:   Duration(60 * time.Second),
			PolicyEvaluation: Duration(5 * time.Minute),
			Retention:        Duration(6 * time.Hour),
		},
		Retention: Retention{
			RawObservations:        Duration(24 * time.Hour),
			AggregatedObservations: Duration(30 * 24 * time.Hour),
			Actions:                Duration(90 * 24 * time.Hour),
			MaxDatabaseBytes:       250 << 20,
		},
		Notifications: Notifications{
			Suspicious:  true,
			Hung:        true,
			Candidate:   true,
			ActionTaken: true,
			Healthy:     false,
		},
		Privacy: Privacy{
			StoreCommandLines:       true,
			RedactEnvironmentValues: true,
			StoreSourceContents:     false,
			NetworkTelemetry:        false,
		},
		Agents: map[string]Agent{
			"codex": {Enabled: true},
		},
	}
}

// ErrNotExist reports that no configuration file was found.
var ErrNotExist = errors.New("config: no configuration file")

// Load reads a configuration file, applying built-in defaults for anything the
// file does not set. A missing file is not an error: the defaults are safe.
func Load(path string) (Config, error) {
	cfg := Default()
	cfg.SourcePath = path

	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		cfg.Defaulted = true
		return cfg, cfg.Validate()
	case err != nil:
		return Config{}, fmt.Errorf("config: reading %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// Validate enforces both ordinary correctness and the phase-1 safety
// invariants.
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported version %d, expected 1", c.Version)
	}
	if !c.GlobalMode.Valid() {
		return fmt.Errorf("globalMode %q is not one of disabled, audit, recommend, enforce", c.GlobalMode)
	}
	if c.GlobalMode == ModeEnforce {
		return fmt.Errorf(
			"globalMode %q is not available in this build: enforcement arrives in delivery phase 7; phase 6 permits only disabled, audit, or manually approved recommendation",
			c.GlobalMode)
	}
	if c.Privacy.StoreSourceContents {
		return errors.New("privacy.storeSourceContents must be false: ghostgc never reads or stores source-code contents")
	}
	if c.Privacy.NetworkTelemetry {
		return errors.New("privacy.networkTelemetry must be false: this build has no telemetry transport")
	}

	minimums := []struct {
		name string
		got  time.Duration
		min  time.Duration
	}{
		{"sampling.processScan", c.Sampling.ProcessScan.D(), time.Second},
		{"sampling.activitySample", c.Sampling.ActivitySample.D(), time.Second},
		{"sampling.classification", c.Sampling.Classification.D(), time.Second},
		{"sampling.policyEvaluation", c.Sampling.PolicyEvaluation.D(), time.Second},
		{"sampling.retention", c.Sampling.Retention.D(), time.Minute},
		{"retention.rawObservations", c.Retention.RawObservations.D(), time.Minute},
		{"retention.aggregatedObservations", c.Retention.AggregatedObservations.D(), time.Minute},
		{"retention.actions", c.Retention.Actions.D(), time.Minute},
	}
	for _, m := range minimums {
		if m.got < m.min {
			return fmt.Errorf("%s is %s, which is below the minimum of %s", m.name, m.got, m.min)
		}
	}
	if c.Retention.MaxDatabaseBytes < 1<<20 {
		return fmt.Errorf("retention.maxDatabaseBytes is %d, which is below the 1 MiB minimum", c.Retention.MaxDatabaseBytes)
	}
	return c.validatePolicies()
}

// EnabledAgents returns the identifiers of enabled agent adapters.
func (c Config) EnabledAgents() []string {
	var out []string
	for id, a := range c.Agents {
		if a.Enabled {
			out = append(out, id)
		}
	}
	return out
}

// ResolvePaths applies configured overrides to the platform defaults.
func (c Config) ResolvePaths(base Paths) Paths {
	if c.Paths.StateDir != "" {
		base.StateDir = c.Paths.StateDir
		base.Database = base.StateDir + "/" + AppName + ".db"
		base.Socket = base.StateDir + "/" + AppName + ".sock"
	}
	if c.Paths.LogDir != "" {
		base.LogDir = c.Paths.LogDir
	}
	if c.Paths.Socket != "" {
		base.Socket = c.Paths.Socket
	}
	if c.Paths.Database != "" {
		base.Database = c.Paths.Database
	}
	return base
}

// Example renders a commented default configuration file.
func Example() string {
	return exampleConfig
}
