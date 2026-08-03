package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/protection"
)

var policyIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Policy is one deliberately bounded cleanup rule.
type Policy struct {
	ID                  string   `yaml:"id"`
	Description         string   `yaml:"description"`
	Enabled             bool     `yaml:"enabled"`
	Mode                Mode     `yaml:"mode"`
	Automatic           bool     `yaml:"automatic"`
	States              []string `yaml:"states"`
	Agents              []string `yaml:"agents"`
	Executables         []string `yaml:"executables"`
	RequireDetached     bool     `yaml:"requireDetached"`
	RequireSessionEnded bool     `yaml:"requireSessionEnded"`
	MinStable           Duration `yaml:"minStable"`
	Cooldown            Duration `yaml:"cooldown"`
}

func (c Config) validatePolicies() error {
	ids := make(map[string]bool)
	enforcePolicies := 0
	for i, policy := range c.Policies {
		prefix := fmt.Sprintf("policies[%d]", i)
		if !policyIDPattern.MatchString(policy.ID) {
			return fmt.Errorf("%s.id %q must match %s", prefix, policy.ID, policyIDPattern)
		}
		if ids[policy.ID] {
			return fmt.Errorf("duplicate policy id %q", policy.ID)
		}
		ids[policy.ID] = true
		if !policy.Mode.Valid() {
			return fmt.Errorf("%s.mode %q is unknown", prefix, policy.Mode)
		}
		if policy.Enabled && policy.Mode == ModeDisabled {
			return fmt.Errorf("%s is enabled but mode is disabled", prefix)
		}
		if policy.Automatic && policy.Mode != ModeEnforce {
			return fmt.Errorf("%s.automatic requires mode enforce", prefix)
		}
		if policy.Mode == ModeEnforce && !policy.Automatic {
			return fmt.Errorf("%s mode enforce requires automatic: true", prefix)
		}
		if policy.Enabled && policy.Mode == ModeEnforce {
			enforcePolicies++
		}
		if strings.TrimSpace(policy.Description) == "" {
			return fmt.Errorf("%s.description is required", prefix)
		}
		if len(policy.States) == 0 || len(policy.Agents) == 0 || len(policy.Executables) == 0 {
			return fmt.Errorf("%s must scope states, agents and executables explicitly", prefix)
		}
		if duplicate := duplicateValue(policy.States); duplicate != "" {
			return fmt.Errorf("%s has duplicate state %q", prefix, duplicate)
		}
		if duplicate := duplicateValue(policy.Agents); duplicate != "" {
			return fmt.Errorf("%s has duplicate agent %q", prefix, duplicate)
		}
		if duplicate := duplicateValue(policy.Executables); duplicate != "" {
			return fmt.Errorf("%s has duplicate executable %q", prefix, duplicate)
		}
		for _, state := range policy.States {
			switch state {
			case "orphaned", "hung", "crashed":
			default:
				return fmt.Errorf("%s state %q is not policy-eligible", prefix, state)
			}
			if state == "orphaned" && (!policy.RequireDetached || !policy.RequireSessionEnded) {
				return fmt.Errorf("%s state %q requires requireDetached and requireSessionEnded", prefix, state)
			}
		}
		for _, agent := range policy.Agents {
			if configured, ok := c.Agents[agent]; !ok || !configured.Enabled {
				return fmt.Errorf("%s agent %q is not enabled", prefix, agent)
			}
		}
		for _, executable := range policy.Executables {
			if executable == "" || filepath.Base(executable) != executable {
				return fmt.Errorf("%s executable %q must be an exact basename", prefix, executable)
			}
			if rule, protected := protection.ExecutableProtection(executable); protected {
				return fmt.Errorf("%s executable %q is a protected broad class (%s)", prefix, executable, rule.ID)
			}
		}
		if policy.MinStable.D() < 0 {
			return fmt.Errorf("%s.minStable cannot be negative", prefix)
		}
		if needsStableWindow(policy.States) && policy.MinStable.D() < 5*time.Minute {
			return fmt.Errorf("%s.minStable must be at least 5m for non-crash states", prefix)
		}
		if policy.Cooldown.D() < time.Minute {
			return fmt.Errorf("%s.cooldown must be at least 1m", prefix)
		}
		if policy.Mode == ModeEnforce {
			if len(policy.States) != 1 || policy.States[0] != "orphaned" || len(policy.Agents) != 1 || len(policy.Executables) != 1 {
				return fmt.Errorf("%s enforce scope must be one orphaned state, one agent and one executable", prefix)
			}
			if !policy.RequireDetached || !policy.RequireSessionEnded {
				return fmt.Errorf("%s enforce requires detachment and an ended session", prefix)
			}
			if policy.Cooldown.D() < time.Hour {
				return fmt.Errorf("%s enforce cooldown must be at least 1h", prefix)
			}
		}
	}
	if enforcePolicies > 1 {
		return errors.New("only one enabled enforce policy is allowed")
	}
	return nil
}

func needsStableWindow(states []string) bool {
	for _, state := range states {
		if state == "orphaned" || state == "hung" {
			return true
		}
	}
	return false
}

func duplicateValue(values []string) string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return value
		}
		seen[value] = true
	}
	return ""
}
