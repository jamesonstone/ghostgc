package daemon

import (
	"context"
	"encoding/json"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/policy"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Candidates implements api.Backend with separate authority projections.
func (d *Daemon) Candidates(ctx context.Context) (api.CandidatesResponse, error) {
	records, err := d.currentPolicyDecisions(ctx)
	if err != nil {
		return api.CandidatesResponse{}, err
	}
	resp := api.CandidatesResponse{
		Enforceable: make([]api.CandidateEntry, 0),
		Recommended: make([]api.CandidateEntry, 0),
		Audited:     make([]api.CandidateEntry, 0, len(records)),
		Note:        phaseNote,
	}
	for _, rec := range records {
		entry := candidateEntry(rec)
		if entry.PID == 0 {
			continue
		}
		if d.isRecommendation(rec) {
			entry.Command = "ghostgc cleanup --dry-run --process " + rec.ProcUID + " --policy " + rec.PolicyID
			resp.Recommended = append(resp.Recommended, entry)
		} else if d.isEnforceable(rec) {
			entry.Command = "automatic exact-key SIGTERM after full revalidation"
			resp.Enforceable = append(resp.Enforceable, entry)
		} else {
			resp.Audited = append(resp.Audited, entry)
		}
	}
	return resp, nil
}

func candidateEntry(rec storage.PolicyDecisionRecord) api.CandidateEntry {
	key, err := process.ParseKey(rec.ProcUID)
	if err != nil {
		return api.CandidateEntry{}
	}
	entry := api.CandidateEntry{
		PID: key.PID, ProcUID: rec.ProcUID, SessionID: rec.SessionID,
		PolicyID: rec.PolicyID, State: rec.ClassificationState,
		DecisionTsNs: rec.TsNs, ClassificationTsNs: rec.ClassificationTsNs, Result: rec.Result,
		Reason: rec.Reason, CooldownUntilNs: rec.CooldownUntilNs,
	}
	_ = json.Unmarshal([]byte(rec.EvidenceJSON), &entry.Evidence)
	return entry
}

func (d *Daemon) policyDefinition(id string) (config.Policy, bool) {
	for _, definition := range d.cfg.Policies {
		if definition.ID == id {
			return definition, true
		}
	}
	return config.Policy{}, false
}

func (d *Daemon) manualCleanupEnabled() bool {
	if d.cfg.GlobalMode != config.ModeRecommend && d.cfg.GlobalMode != config.ModeEnforce {
		return false
	}
	for _, definition := range d.cfg.Policies {
		if definition.Enabled && definition.Mode == config.ModeRecommend {
			return true
		}
	}
	return false
}

func (d *Daemon) automaticCleanupEnabled() bool {
	if d.cfg.GlobalMode != config.ModeEnforce {
		return false
	}
	for _, definition := range d.cfg.Policies {
		if definition.Enabled && definition.Mode == config.ModeEnforce && definition.Automatic {
			return true
		}
	}
	return false
}

func (d *Daemon) isRecommendation(rec storage.PolicyDecisionRecord) bool {
	definition, ok := d.policyDefinition(rec.PolicyID)
	eligible := rec.Result == string(policy.ResultCandidate) || rec.Result == string(policy.ResultCooldown)
	global := d.cfg.GlobalMode == config.ModeRecommend || d.cfg.GlobalMode == config.ModeEnforce
	return ok && eligible && global && definition.Enabled && definition.Mode == config.ModeRecommend
}

func (d *Daemon) isEnforceable(rec storage.PolicyDecisionRecord) bool {
	definition, ok := d.policyDefinition(rec.PolicyID)
	return ok && rec.Result == string(policy.ResultCandidate) && d.cfg.GlobalMode == config.ModeEnforce &&
		definition.Enabled && definition.Mode == config.ModeEnforce && definition.Automatic
}

func (d *Daemon) currentPolicyDecisions(ctx context.Context) ([]storage.PolicyDecisionRecord, error) {
	records, err := d.store.CurrentPolicyDecisions(ctx)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	snapshot := d.snapshot
	d.mu.RUnlock()
	if snapshot == nil {
		return nil, nil
	}
	current := make([]storage.PolicyDecisionRecord, 0, len(records))
	for _, record := range records {
		key, err := process.ParseKey(record.ProcUID)
		if err != nil {
			continue
		}
		if _, ok := snapshot.ByKey(key); ok {
			current = append(current, record)
		}
	}
	return current, nil
}

// Policies implements api.Backend.
func (d *Daemon) Policies(ctx context.Context) (api.PoliciesResponse, error) {
	resp := api.PoliciesResponse{GlobalMode: string(d.cfg.GlobalMode), Note: phaseNote}
	for _, def := range d.cfg.Policies {
		resp.Policies = append(resp.Policies, api.PolicySummary{
			ID: def.ID, Description: def.Description, Enabled: def.Enabled, Mode: string(def.Mode), Automatic: def.Automatic,
			States: def.States, Agents: def.Agents, Executables: def.Executables,
			RequireDetached: def.RequireDetached, RequireSessionEnded: def.RequireSessionEnded,
			MinStableNs: int64(def.MinStable.D()), CooldownNs: int64(def.Cooldown.D()),
		})
	}
	return resp, nil
}

func policyEntriesForProcess(all []api.CandidateEntry, procUID string) []api.CandidateEntry {
	var out []api.CandidateEntry
	for _, entry := range all {
		if entry.ProcUID == procUID {
			out = append(out, entry)
		}
	}
	return out
}
