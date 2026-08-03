package policy

import (
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/protection"
)

var policyNow = time.Date(2026, 8, 3, 17, 0, 0, 0, time.UTC)

func auditPolicy() config.Policy {
	return config.Policy{ID: "browser", Enabled: true, Mode: config.ModeAudit,
		States: []string{"orphaned"}, Agents: []string{"codex"}, Executables: []string{"chrome-headless-shell"},
		RequireDetached: true, RequireSessionEnded: true,
		MinStable: config.Duration(5 * time.Minute), Cooldown: config.Duration(time.Hour)}
}

func eligibleTarget() Target {
	return Target{ProcUID: "42:1", SessionID: "s1", ClassificationTs: policyNow,
		State: "orphaned", StableSince: policyNow.Add(-5 * time.Minute), AgentID: "codex",
		Executable: "chrome-headless-shell", Detached: true, SessionEnded: true}
}

func TestEvaluateCandidateRefusalAndCooldown(t *testing.T) {
	tests := []struct {
		name     string
		target   Target
		cooldown time.Time
		want     Result
	}{
		{name: "candidate", target: eligibleTarget(), want: ResultCandidate},
		{name: "refused", target: func() Target {
			v := eligibleTarget()
			v.Protection = protection.Result{Protected: true, Rules: []protection.Rule{{ID: "protected-test", Reason: "test protection"}}}
			return v
		}(), want: ResultRefused},
		{name: "cooldown", target: eligibleTarget(), cooldown: policyNow.Add(time.Hour), want: ResultCooldown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matched := Evaluate(auditPolicy(), tt.target, policyNow, tt.cooldown)
			if !matched || got.Result != tt.want || got.Reason == "" || len(got.Evidence) == 0 {
				t.Fatalf("Evaluate() = %+v, %t; want %s with evidence", got, matched, tt.want)
			}
		})
	}
}

func TestEvaluateRecommendCandidate(t *testing.T) {
	definition := auditPolicy()
	definition.Mode = config.ModeRecommend
	got, matched := Evaluate(definition, eligibleTarget(), policyNow, time.Time{})
	if !matched || got.Result != ResultCandidate || got.Reason == "" {
		t.Fatalf("Evaluate() = %+v, %t", got, matched)
	}
	found := false
	for _, evidence := range got.Evidence {
		found = found || evidence.Rule == "manual-approval-v1"
	}
	if !found {
		t.Fatalf("recommendation is missing manual approval evidence: %+v", got.Evidence)
	}
}

func TestEvaluateMismatchAndExactIdentityCooldown(t *testing.T) {
	target := eligibleTarget()
	target.State = "idle"
	if got, matched := Evaluate(auditPolicy(), target, policyNow, time.Time{}); matched {
		t.Fatalf("nonmatching state produced %+v", got)
	}
	reused := eligibleTarget()
	reused.ProcUID = "42:2"
	got, matched := Evaluate(auditPolicy(), reused, policyNow, time.Time{})
	if !matched || got.Result != ResultCandidate {
		t.Fatalf("new exact identity inherited a cooldown: %+v, %t", got, matched)
	}
}

func TestEvaluateRequiresEveryScopedFact(t *testing.T) {
	tests := []struct {
		name   string
		change func(*config.Policy, *Target)
	}{
		{name: "disabled", change: func(p *config.Policy, _ *Target) { p.Enabled = false }},
		{name: "agent", change: func(_ *config.Policy, v *Target) { v.AgentID = "other" }},
		{name: "executable", change: func(_ *config.Policy, v *Target) { v.Executable = "other" }},
		{name: "detachment", change: func(_ *config.Policy, v *Target) { v.Detached = false }},
		{name: "session", change: func(_ *config.Policy, v *Target) { v.SessionEnded = false }},
		{name: "classification time", change: func(_ *config.Policy, v *Target) { v.ClassificationTs = time.Time{} }},
		{name: "stable time", change: func(_ *config.Policy, v *Target) { v.StableSince = time.Time{} }},
		{name: "stable duration", change: func(_ *config.Policy, v *Target) { v.StableSince = policyNow.Add(-time.Minute) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition, target := auditPolicy(), eligibleTarget()
			tt.change(&definition, &target)
			if got, matched := Evaluate(definition, target, policyNow, time.Time{}); matched {
				t.Fatalf("incomplete scope produced %+v", got)
			}
		})
	}
}
