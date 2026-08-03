// Package policy evaluates bounded audit and recommendation policies. It has
// no signalling interface; action authority belongs to the daemon gate.
package policy

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/protection"
)

// Result is one auditable policy outcome.
type Result string

const (
	ResultCandidate Result = "candidate"
	ResultRefused   Result = "refused"
	ResultCooldown  Result = "cooldown"
)

// Evidence explains a policy match or refusal.
type Evidence struct {
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// Target is the complete current fact set used by policy evaluation.
type Target struct {
	ProcUID          string
	SessionID        string
	ClassificationTs time.Time
	State            string
	StableSince      time.Time
	AgentID          string
	Executable       string
	Detached         bool
	SessionEnded     bool
	Protection       protection.Result
}

// Decision is emitted only when the bounded match conditions hold.
type Decision struct {
	PolicyID      string
	ProcUID       string
	SessionID     string
	At            time.Time
	State         string
	Result        Result
	Reason        string
	CooldownUntil time.Time
	Evidence      []Evidence
}

// Evaluate returns a decision only after all explicit match conditions hold.
func Evaluate(def config.Policy, target Target, at, priorCooldownUntil time.Time) (Decision, bool) {
	if !def.Enabled || (def.Mode != config.ModeAudit && def.Mode != config.ModeRecommend) || !matches(def.States, target.State) ||
		!matches(def.Agents, target.AgentID) || !matches(def.Executables, target.Executable) ||
		(def.RequireDetached && !target.Detached) || (def.RequireSessionEnded && !target.SessionEnded) ||
		target.ClassificationTs.IsZero() || target.StableSince.IsZero() ||
		at.Sub(target.StableSince) < def.MinStable.D() {
		return Decision{}, false
	}
	decision := Decision{
		PolicyID: def.ID, ProcUID: target.ProcUID, SessionID: target.SessionID,
		At: at, State: target.State,
		Evidence: []Evidence{
			{Rule: "policy-state-v1", Detail: fmt.Sprintf("classification is %s and has been stable since %s", target.State, target.StableSince.Format(time.RFC3339))},
			{Rule: "policy-scope-v1", Detail: fmt.Sprintf("agent %s executable %s matched exact policy scope", target.AgentID, target.Executable)},
		},
	}
	if target.Protection.Protected {
		decision.Result = ResultRefused
		decision.Reason = fmt.Sprintf("%d non-overridable protection(s) apply", len(target.Protection.Rules))
		for _, rule := range target.Protection.Rules {
			decision.Evidence = append(decision.Evidence, Evidence{Rule: rule.ID, Detail: rule.Reason})
		}
		return decision, true
	}
	if priorCooldownUntil.After(at) {
		decision.Result, decision.CooldownUntil = ResultCooldown, priorCooldownUntil
		decision.Reason = "an earlier candidate for this exact policy and process is cooling down"
		decision.Evidence = append(decision.Evidence, Evidence{Rule: "policy-cooldown-v1", Detail: "cooldown ends " + priorCooldownUntil.Format(time.RFC3339)})
		return decision, true
	}
	decision.Result = ResultCandidate
	decision.Reason = "policy matched and no hard protection applies"
	decision.CooldownUntil = at.Add(def.Cooldown.D())
	if def.Mode == config.ModeRecommend {
		decision.Evidence = append(decision.Evidence, Evidence{Rule: "manual-approval-v1", Detail: "recommendation requires a separate short-lived approval and full revalidation"})
	} else {
		decision.Evidence = append(decision.Evidence, Evidence{Rule: "audit-only-v1", Detail: "audit policy grants no recommendation or signalling authority"})
	}
	return decision, true
}

func matches(values []string, got string) bool {
	for _, value := range values {
		if value == got {
			return true
		}
	}
	return false
}
