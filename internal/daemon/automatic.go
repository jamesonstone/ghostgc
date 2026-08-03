package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// runAutomaticCleanupLocked selects at most one exact current candidate from
// the just-committed evaluation. The caller owns scanMu through this method.
func (d *Daemon) runAutomaticCleanupLocked(ctx context.Context, batch policyBatch) {
	if !batch.due || !d.automaticCleanupEnabled() {
		return
	}
	records, err := d.store.CurrentPolicyDecisions(ctx)
	if err != nil {
		d.recordAutomaticFailure(ctx, "reading current decisions: "+err.Error())
		return
	}
	for _, decision := range records {
		if !d.isEnforceable(decision) {
			continue
		}
		d.executeAutomaticDecision(ctx, decision)
		return
	}
}

func (d *Daemon) executeAutomaticDecision(ctx context.Context, decision storage.PolicyDecisionRecord) {
	definition, ok := d.policyDefinition(decision.PolicyID)
	key, err := process.ParseKey(decision.ProcUID)
	if !ok || err != nil {
		d.recordAutomaticFailure(ctx, "selected decision has invalid policy or process identity")
		return
	}
	d.mu.RLock()
	observed, present := d.snapshot.ByKey(key)
	d.mu.RUnlock()
	executable, err := exactExecutable(observed, present)
	if err != nil {
		d.recordAutomaticFailure(ctx, "selected decision lacks exact executable identity: "+err.Error())
		return
	}
	binding, err := approvalBinding(decision, definition, executable)
	if err != nil {
		d.recordAutomaticFailure(ctx, "binding automatic authority: "+err.Error())
		return
	}
	actionID, err := newActionID()
	if err != nil {
		d.recordAutomaticFailure(ctx, "creating action identity: "+err.Error())
		return
	}
	authority := &cleanupApproval{
		bindingDigest: binding, decision: decision, policy: definition,
		executable: executable, authority: authorityAutomatic,
	}
	if _, err := d.executeCleanupLocked(ctx, actionID, authority, time.Now()); err != nil {
		d.recordAutomaticFailure(ctx, fmt.Sprintf("action %s: %v", actionID, err))
	}
}

func (d *Daemon) recordAutomaticFailure(ctx context.Context, reason string) {
	d.log.Error("automatic cleanup failed closed", "error", reason)
	d.mu.Lock()
	d.degraded = append(d.degraded, "automatic cleanup: "+reason)
	d.mu.Unlock()
	d.audit(ctx, "action.automatic.skipped", "daemon", "automatic cleanup failed closed: "+reason)
}
