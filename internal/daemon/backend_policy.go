package daemon

import (
	"context"
	"encoding/json"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Candidates implements api.Backend. Phase 5 has no enforceable lane.
func (d *Daemon) Candidates(ctx context.Context) (api.CandidatesResponse, error) {
	records, err := d.currentPolicyDecisions(ctx)
	if err != nil {
		return api.CandidatesResponse{}, err
	}
	resp := api.CandidatesResponse{
		Enforceable: make([]api.CandidateEntry, 0),
		Audited:     make([]api.CandidateEntry, 0, len(records)),
		Note:        phaseNote,
	}
	for _, rec := range records {
		key, err := process.ParseKey(rec.ProcUID)
		if err != nil {
			continue
		}
		entry := api.CandidateEntry{
			PID: key.PID, ProcUID: rec.ProcUID, SessionID: rec.SessionID,
			PolicyID: rec.PolicyID, State: rec.ClassificationState, Result: rec.Result,
			Reason: rec.Reason, CooldownUntilNs: rec.CooldownUntilNs,
		}
		_ = json.Unmarshal([]byte(rec.EvidenceJSON), &entry.Evidence)
		resp.Audited = append(resp.Audited, entry)
	}
	return resp, nil
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
			ID: def.ID, Description: def.Description, Enabled: def.Enabled, Mode: string(def.Mode),
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
