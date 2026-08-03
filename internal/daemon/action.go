package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/policy"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const (
	actionAttempting = "attempting"
	actionRejected   = "rejected"
	actionSignalled  = "signalled"
	actionFailed     = "failed"
)

// CleanupApply consumes manual authority and performs full fresh revalidation.
func (d *Daemon) CleanupApply(ctx context.Context, req api.CleanupApplyRequest) (api.CleanupApplyResponse, error) {
	if req.Approval == "" {
		return api.CleanupApplyResponse{}, errors.New("cleanup apply requires an approval")
	}
	now := time.Now()
	approval, refusal := d.consumeApproval(req.Approval, now)
	if approval == nil {
		return api.CleanupApplyResponse{}, errors.New(refusal)
	}
	actionID, err := newActionID()
	if err != nil {
		return api.CleanupApplyResponse{}, err
	}
	if refusal != "" {
		evidence := []policy.Evidence{{Rule: "approval-consumption-v1", Detail: refusal}}
		return d.rejectAction(ctx, actionID, approval, refusal, evidence, now)
	}
	// Hold the scan lane from the fresh snapshot through the side effect. This
	// prevents a policy scan from advancing daemon state between revalidation
	// and the exact-key platform gate.
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	return d.executeCleanupLocked(ctx, actionID, approval, now)
}

func (d *Daemon) executeCleanupLocked(ctx context.Context, actionID string,
	approval *cleanupApproval, now time.Time) (api.CleanupApplyResponse, error) {
	target, err := d.revalidateCleanup(ctx, approval)
	if err != nil {
		evidence := append(target.evidence, policy.Evidence{Rule: "pre-action-refusal-v1", Detail: err.Error()})
		return d.rejectAction(ctx, actionID, approval, err.Error(), evidence, time.Now())
	}

	evidenceJSON := actionEvidenceJSON(target.evidence)
	attempt := storage.ActionRecord{
		ActionID: actionID, PolicyID: approval.policy.ID, ProcUID: approval.decision.ProcUID,
		SessionID: approval.decision.SessionID, Authority: approval.authority, RequestedNs: now.UnixNano(),
		UpdatedNs: time.Now().UnixNano(), Result: actionAttempting, Signal: "SIGTERM",
		Reason:       approval.authority + " authority passed full pre-action revalidation; exact-key SIGTERM is about to be attempted",
		EvidenceJSON: evidenceJSON,
	}
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.InsertAction(attempt); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: attempt.UpdatedNs, Kind: "action.attempting", Subject: attempt.ProcUID,
			Summary:      fmt.Sprintf("%s action %s committed before exact-key SIGTERM for policy %s", approval.authority, actionID, attempt.PolicyID),
			EvidenceJSON: evidenceJSON,
		})
	}); err != nil {
		return api.CleanupApplyResponse{}, err
	}

	signalErr := d.plat.SignalProcess(ctx, target.key, approval.executable, platform.SIGTERM)
	completedAt := time.Now()
	result, reason, kind := actionSignalled, "exact-key SIGTERM was accepted by the operating system", "action.signalled"
	if signalErr != nil {
		result, reason, kind = actionFailed, "exact-key SIGTERM failed: "+signalErr.Error(), "action.failed"
	}
	completedEvidence := append(append([]policy.Evidence(nil), target.evidence...), policy.Evidence{
		Rule: "signal-result-v1", Detail: reason,
	})
	completedJSON := actionEvidenceJSON(completedEvidence)
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.UpdateAction(actionID, result, reason, completedJSON, completedAt.UnixNano()); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: completedAt.UnixNano(), Kind: kind, Subject: attempt.ProcUID,
			Summary: fmt.Sprintf("%s action %s: %s", approval.authority, actionID, reason), EvidenceJSON: completedJSON,
		})
	}); err != nil {
		return api.CleanupApplyResponse{}, err
	}
	return actionResponse(actionID, approval, result, reason, completedJSON, completedAt), nil
}

func newActionID() (string, error) {
	id, _, err := newSecret(12)
	if err != nil {
		return "", err
	}
	return "act_" + id, nil
}

func (d *Daemon) rejectAction(ctx context.Context, actionID string, approval *cleanupApproval,
	reason string, evidence []policy.Evidence, at time.Time) (api.CleanupApplyResponse, error) {
	evidenceJSON := actionEvidenceJSON(evidence)
	record := storage.ActionRecord{
		ActionID: actionID, PolicyID: approval.policy.ID, ProcUID: approval.decision.ProcUID,
		SessionID: approval.decision.SessionID, Authority: approval.authority,
		RequestedNs: at.UnixNano(), UpdatedNs: at.UnixNano(),
		Result: actionRejected, Signal: "SIGTERM", Reason: reason, EvidenceJSON: evidenceJSON,
	}
	if err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if err := tx.InsertAction(record); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs: at.UnixNano(), Kind: "action.rejected", Subject: record.ProcUID,
			Summary:      fmt.Sprintf("%s action %s rejected before signalling: %s", approval.authority, actionID, reason),
			EvidenceJSON: evidenceJSON,
		})
	}); err != nil {
		return api.CleanupApplyResponse{}, err
	}
	return actionResponse(actionID, approval, actionRejected, reason, evidenceJSON, at), nil
}

func actionResponse(id string, approval *cleanupApproval, result, reason, evidence string, at time.Time) api.CleanupApplyResponse {
	return api.CleanupApplyResponse{
		ActionID: id, Authority: approval.authority, PolicyID: approval.policy.ID, ProcUID: approval.decision.ProcUID,
		Result: result, Signal: "SIGTERM", AtNs: at.UnixNano(), Reason: reason,
		Evidence: json.RawMessage(evidence),
	}
}

// Actions implements api.Backend.
func (d *Daemon) Actions(ctx context.Context, opts api.ActionOptions) (api.ActionsResponse, error) {
	records, err := d.store.ListActions(ctx, storage.ActionFilter{
		ProcUID: opts.ProcUID, PolicyID: opts.PolicyID, Result: opts.Result, Limit: opts.Limit,
	})
	if err != nil {
		return api.ActionsResponse{}, err
	}
	response := api.ActionsResponse{Actions: make([]api.ActionView, 0, len(records))}
	for _, record := range records {
		view := api.ActionView{
			ActionID: record.ActionID, Authority: record.Authority,
			PolicyID: record.PolicyID, ProcUID: record.ProcUID,
			SessionID: record.SessionID, RequestedNs: record.RequestedNs, UpdatedNs: record.UpdatedNs,
			Result: record.Result, Signal: record.Signal, Reason: record.Reason,
		}
		_ = json.Unmarshal([]byte(record.EvidenceJSON), &view.Evidence)
		response.Actions = append(response.Actions, view)
	}
	return response, nil
}
