package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/policy"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type policyBatch struct {
	due        bool
	at         time.Time
	records    []storage.PolicyDecisionRecord
	audit      []storage.AuditRecord
	candidates int
}

func (d *Daemon) evaluatePolicies(ctx context.Context, snap *process.Snapshot, tree *process.Tree,
	res *sessions.Result, classes classificationBatch) (policyBatch, error) {
	if !classes.due || (!d.lastPolicyAt.IsZero() && snap.Taken.Sub(d.lastPolicyAt) < d.cfg.Sampling.PolicyEvaluation.D()) {
		return policyBatch{}, nil
	}
	batch := policyBatch{due: true, at: snap.Taken}
	for _, class := range classes.current {
		key, err := process.ParseKey(class.ProcUID)
		if err != nil {
			continue
		}
		proc, ok := snap.ByKey(key)
		if !ok {
			continue
		}
		attr, ok := res.Attributions[class.ProcUID]
		if !ok {
			continue
		}
		root, isRoot := res.Roots[proc.PID]
		isRoot = isRoot && root.Key == key
		protectionResult := d.protectionFor(proc, tree, attr.Confidence, isRoot, !class.SessionEnded)
		for _, definition := range d.cfg.Policies {
			untilNs, err := d.store.LastCandidateCooldown(ctx, definition.ID, class.ProcUID)
			if err != nil {
				return policyBatch{}, err
			}
			decision, matched := policy.Evaluate(definition, policy.Target{
				ProcUID: class.ProcUID, SessionID: class.SessionID,
				ClassificationTs: time.Unix(0, class.TsNs), State: class.State,
				StableSince: time.Unix(0, class.StableSinceNs), AgentID: attr.AgentID,
				Executable: proc.Name(), Detached: class.Detached, SessionEnded: class.SessionEnded,
				Protection: protectionResult,
			}, snap.Taken, time.Unix(0, untilNs))
			if !matched {
				continue
			}
			var classificationEvidence []policy.Evidence
			_ = json.Unmarshal([]byte(class.EvidenceJSON), &classificationEvidence)
			decision.Evidence = append(classificationEvidence, decision.Evidence...)
			evidence, _ := json.Marshal(decision.Evidence)
			var cooldownUntilNs int64
			if !decision.CooldownUntil.IsZero() {
				cooldownUntilNs = decision.CooldownUntil.UnixNano()
			}
			record := storage.PolicyDecisionRecord{
				PolicyID: decision.PolicyID, ProcUID: decision.ProcUID, SessionID: decision.SessionID,
				TsNs: snap.Taken.UnixNano(), ClassificationTsNs: class.TsNs,
				ClassificationState: decision.State, Result: string(decision.Result),
				Reason: decision.Reason, CooldownUntilNs: cooldownUntilNs, EvidenceJSON: string(evidence),
			}
			batch.records = append(batch.records, record)
			if decision.Result == policy.ResultCandidate {
				batch.candidates++
			}
			batch.audit = append(batch.audit, storage.AuditRecord{
				TsNs: snap.Taken.UnixNano(), Kind: "policy." + string(decision.Result), Subject: class.ProcUID,
				Summary: fmt.Sprintf("policy %s: %s", decision.PolicyID, decision.Reason), EvidenceJSON: string(evidence),
			})
		}
	}
	return batch, nil
}

func (d *Daemon) commitPolicies(batch policyBatch) {
	if batch.due {
		d.lastPolicyAt = batch.at
	}
}
