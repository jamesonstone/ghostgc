package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/classification"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/policy"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type cleanupTarget struct {
	key      process.Key
	evidence []policy.Evidence
}

func (d *Daemon) revalidateCleanup(ctx context.Context, approval *cleanupApproval) (cleanupTarget, error) {
	definition, ok := d.policyDefinition(approval.policy.ID)
	if !ok || definition.Mode != approval.policy.Mode || !d.cleanupAuthorityEnabled(approval, definition) {
		return cleanupTarget{}, errors.New("cleanup authority, policy or global mode changed")
	}
	records, err := d.store.CurrentPolicyDecisions(ctx)
	if err != nil {
		return cleanupTarget{}, err
	}
	var current storage.PolicyDecisionRecord
	for _, record := range records {
		if record.ID == approval.decision.ID && record.EvaluationID == approval.decision.EvaluationID {
			current = record
			break
		}
	}
	if current.ID == 0 || !d.cleanupDecisionCurrent(approval, current) {
		return cleanupTarget{}, errors.New("the authorized cleanup decision is no longer current")
	}
	binding, err := approvalBinding(current, definition, approval.executable)
	if err != nil {
		return cleanupTarget{}, err
	}
	if binding != approval.bindingDigest {
		return cleanupTarget{}, errors.New("the policy decision or evidence changed after authority was established")
	}

	key, err := process.ParseKey(current.ProcUID)
	if err != nil {
		return cleanupTarget{}, err
	}
	snap, err := d.plat.SnapshotProcesses(ctx)
	if err != nil {
		return cleanupTarget{}, fmt.Errorf("fresh process snapshot unavailable: %w", err)
	}
	proc, ok := snap.ByKey(key)
	if !ok {
		return cleanupTarget{}, errors.New("exact process identity exited or changed")
	}
	executable, err := exactExecutable(proc, true)
	if err != nil {
		return cleanupTarget{}, err
	}
	if executable != approval.executable {
		return cleanupTarget{}, errors.New("exact executable identity changed after authority was established")
	}
	tree := process.BuildTree(snap)
	res, err := d.recon.Reconcile(ctx, snap, tree, d.cfg.Privacy.StoreCommandLines)
	if err != nil {
		return cleanupTarget{}, fmt.Errorf("fresh ownership reconciliation failed: %w", err)
	}
	attr, ok := res.Attributions[current.ProcUID]
	if !ok || !attr.Attributed() || attr.SessionID != current.SessionID {
		return cleanupTarget{}, errors.New("ownership or session attribution changed")
	}
	ended, err := d.sessionEnded(ctx, current.SessionID, res)
	if err != nil {
		return cleanupTarget{}, err
	}
	sample, err := d.plat.SampleActivity(ctx, key, attr.RepositoryPath)
	if err != nil {
		return cleanupTarget{}, fmt.Errorf("fresh activity evidence unavailable: %w", err)
	}
	if sample.Key != key || sample.Taken.IsZero() || sample.Taken.Before(snap.Taken) {
		return cleanupTarget{}, errors.New("fresh activity sample failed exact-key or freshness validation")
	}
	delta := process.DeriveActivity(d.activityBaseline[current.ProcUID], sample)
	detached := attr.OriginalParentObserved && proc.PPID != attr.OriginalPPID
	detail := ""
	if detached {
		detail = fmt.Sprintf("observed original parent pid %d was replaced by current parent pid %d", attr.OriginalPPID, proc.PPID)
	}
	conclusion := classification.Classify(classification.Input{
		Key: key, Status: proc.Status, Detached: detached, SessionEnded: ended,
		Previous:        d.classificationPrevious[current.ProcUID],
		EvidenceCadence: d.cfg.Sampling.ActivitySample.D(), DetachmentDetail: detail,
		Activity: classification.Activity{
			Taken: sample.Taken, BaselineOK: delta.BaselineOK,
			CPUPercent: delta.CPUPercent, CPUKnown: delta.CPUKnown,
			DiskReadBytes:    boundedInt64(delta.DiskReadBytes),
			DiskWrittenBytes: boundedInt64(delta.DiskWrittenBytes), IOKnown: delta.IOKnown,
			WritableRepositoryFiles: delta.WritableRepositoryFiles, FilesKnown: delta.FilesKnown,
			ConnectedSockets: delta.ConnectedSockets, NetworkChanged: delta.NetworkChanged, SocketsKnown: delta.SocketsKnown,
		},
	})
	evidence := []policy.Evidence{
		{Rule: "authority-binding-v1", Detail: fmt.Sprintf("the exact committed evaluation, decision, policy and %s authority still match", approval.authority)},
		{Rule: "exact-executable-v1", Detail: "the fresh executable path and kernel name still match bound authority"},
		{Rule: "exact-process-key-v1", Detail: "fresh snapshot contains " + key.UID()},
		{Rule: "fresh-ownership-v1", Detail: fmt.Sprintf("fresh reconciliation retained session %s with confidence %.2f", attr.SessionID, attr.Confidence)},
	}
	for _, item := range conclusion.Evidence {
		evidence = append(evidence, policy.Evidence{Rule: item.Rule, Detail: item.Detail})
	}
	if string(conclusion.State) != current.ClassificationState {
		return cleanupTarget{key: key, evidence: evidence}, fmt.Errorf("classification changed from %s to %s", current.ClassificationState, conclusion.State)
	}
	root, isRoot := res.Roots[proc.PID]
	protections := d.protectionFor(proc, tree, attr.Confidence, isRoot && root.Key == key, !ended)
	if protections.Protected {
		for _, rule := range protections.Rules {
			evidence = append(evidence, policy.Evidence{Rule: rule.ID, Detail: rule.Reason})
		}
		return cleanupTarget{key: key, evidence: evidence}, fmt.Errorf("%d non-overridable protection(s) apply", len(protections.Rules))
	}
	decision, matched := policy.Evaluate(definition, policy.Target{
		ProcUID: key.UID(), SessionID: attr.SessionID, ClassificationTs: sample.Taken,
		State: string(conclusion.State), StableSince: conclusion.StableSince,
		AgentID: attr.AgentID, Executable: proc.Name(), Detached: detached,
		SessionEnded: ended, Protection: protections,
	}, sample.Taken, time.Time{})
	if !matched || decision.Result != policy.ResultCandidate {
		return cleanupTarget{key: key, evidence: evidence}, errors.New("fresh facts no longer satisfy the exact cleanup policy")
	}
	evidence = append(evidence, decision.Evidence...)
	evidence = append(evidence, policy.Evidence{Rule: "pre-action-revalidation-v1", Detail: "all authority, identity, lifecycle, activity, classification, policy and hard-protection gates passed"})
	return cleanupTarget{key: key, evidence: evidence}, nil
}

func (d *Daemon) cleanupAuthorityEnabled(approval *cleanupApproval, definition config.Policy) bool {
	switch approval.authority {
	case authorityManual:
		return d.manualCleanupEnabled() && definition.Enabled && definition.Mode == config.ModeRecommend
	case authorityAutomatic:
		return d.automaticCleanupEnabled() && definition.Enabled && definition.Mode == config.ModeEnforce && definition.Automatic
	default:
		return false
	}
}

func (d *Daemon) cleanupDecisionCurrent(approval *cleanupApproval, current storage.PolicyDecisionRecord) bool {
	if approval.authority == authorityAutomatic {
		return d.isEnforceable(current)
	}
	return approval.authority == authorityManual && d.isRecommendation(current)
}

func (d *Daemon) sessionEnded(ctx context.Context, sessionID string, res *sessions.Result) (bool, error) {
	record, err := d.store.GetSession(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("reading session lifecycle: %w", err)
	}
	if record.EndedNs != nil {
		return true, nil
	}
	for _, ended := range res.Ended {
		if ended.SessionID == sessionID {
			return true, nil
		}
	}
	return false, nil
}

func actionEvidenceJSON(evidence []policy.Evidence) string {
	raw, _ := json.Marshal(evidence)
	return string(raw)
}
