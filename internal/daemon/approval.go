package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const approvalLifetime = 2 * time.Minute
const maxApprovals = 128

type cleanupApproval struct {
	bindingDigest string
	decision      storage.PolicyDecisionRecord
	policy        config.Policy
	executable    process.ExecutableIdentity
	expires       time.Time
	used          bool
}

// CleanupPreview issues one ephemeral approval for an exact recommendation.
func (d *Daemon) CleanupPreview(ctx context.Context, req api.CleanupPreviewRequest) (api.CleanupPreviewResponse, error) {
	if req.PolicyID == "" || req.ProcUID == "" {
		return api.CleanupPreviewResponse{}, errors.New("cleanup preview requires policy_id and exact proc_uid")
	}
	if _, err := process.ParseKey(req.ProcUID); err != nil {
		return api.CleanupPreviewResponse{}, err
	}
	// A preview must bind one coherent committed scan. Otherwise a scan could
	// advance the decision or executable observation between the two reads.
	d.scanMu.Lock()
	defer d.scanMu.Unlock()
	if !d.manualCleanupEnabled() {
		return api.CleanupPreviewResponse{}, errors.New("manual cleanup is disabled: globalMode and the selected policy must both be recommend")
	}
	records, err := d.currentPolicyDecisions(ctx)
	if err != nil {
		return api.CleanupPreviewResponse{}, err
	}
	var decision storage.PolicyDecisionRecord
	for _, record := range records {
		if record.PolicyID == req.PolicyID && record.ProcUID == req.ProcUID && d.isRecommendation(record) {
			decision = record
			break
		}
	}
	if decision.ID == 0 {
		return api.CleanupPreviewResponse{}, errors.New("no current recommendation matches that exact policy and process")
	}
	key, _ := process.ParseKey(decision.ProcUID)
	d.mu.RLock()
	observed, present := d.snapshot.ByKey(key)
	d.mu.RUnlock()
	identity, err := exactExecutable(observed, present)
	if err != nil {
		return api.CleanupPreviewResponse{}, err
	}
	definition, _ := d.policyDefinition(decision.PolicyID)
	token, digest, err := newSecret(32)
	if err != nil {
		return api.CleanupPreviewResponse{}, err
	}
	binding, err := approvalBinding(decision, definition, identity)
	if err != nil {
		return api.CleanupPreviewResponse{}, err
	}
	now := time.Now()
	expires := now.Add(approvalLifetime)
	approval := &cleanupApproval{
		bindingDigest: binding, decision: decision, policy: definition,
		executable: identity, expires: expires,
	}
	d.actionMu.Lock()
	d.pruneApprovals(now)
	if len(d.approvals) >= maxApprovals {
		d.actionMu.Unlock()
		return api.CleanupPreviewResponse{}, errors.New("too many outstanding approvals; wait for an earlier preview to expire")
	}
	d.approvals[digest] = approval
	d.actionMu.Unlock()

	evidence, _ := json.Marshal([]map[string]any{{
		"rule": "approval-binding-v1", "binding_digest": binding,
		"evaluation_id": decision.EvaluationID, "decision_id": decision.ID,
		"expires_ns": expires.UnixNano(), "signal": "SIGTERM",
	}})
	if err := d.store.AppendAudit(ctx, storage.AuditRecord{
		TsNs: now.UnixNano(), Kind: "action.previewed", Subject: decision.ProcUID,
		Summary:      fmt.Sprintf("manual SIGTERM preview issued for policy %s; approval expires at %s", decision.PolicyID, expires.Format(time.RFC3339)),
		EvidenceJSON: string(evidence),
	}); err != nil {
		d.actionMu.Lock()
		delete(d.approvals, digest)
		d.actionMu.Unlock()
		return api.CleanupPreviewResponse{}, err
	}
	entry := candidateEntry(decision)
	command := "ghostgc cleanup --apply --approval " + token + " --yes"
	return api.CleanupPreviewResponse{
		Approval: token, ExpiresNs: expires.UnixNano(), Candidate: entry,
		Signal: "SIGTERM", Command: command,
		Revalidation: []string{
			"current evaluation and policy binding unchanged", "exact pid and start time still present",
			"ownership and session lifecycle freshly reconciled", "activity freshly sampled and classification unchanged",
			"all hard protections absent", "exact policy still recommends the target",
		},
		Note: "No signal has been sent. The approval is single-use, memory-only, and expires in two minutes.",
	}, nil
}

func (d *Daemon) consumeApproval(token string, now time.Time) (*cleanupApproval, string) {
	digest := secretDigest(token)
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	approval := d.approvals[digest]
	if approval == nil {
		return nil, "approval is unknown or was lost when the daemon restarted"
	}
	if approval.used {
		return approval, "approval has already been consumed"
	}
	approval.used = true
	if !now.Before(approval.expires) {
		return approval, "approval has expired"
	}
	return approval, ""
}

func (d *Daemon) pruneApprovals(now time.Time) {
	for digest, approval := range d.approvals {
		if approval.expires.Add(approvalLifetime).Before(now) {
			delete(d.approvals, digest)
		}
	}
}

func newSecret(n int) (token, digest string, err error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generating approval: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, secretDigest(token), nil
}

func secretDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func approvalBinding(decision storage.PolicyDecisionRecord, definition config.Policy,
	executable process.ExecutableIdentity) (string, error) {
	raw, err := json.Marshal(struct {
		Decision   storage.PolicyDecisionRecord `json:"decision"`
		Evidence   string                       `json:"decision_evidence"`
		Policy     config.Policy                `json:"policy"`
		Executable process.ExecutableIdentity   `json:"executable"`
	}{decision, decision.EvidenceJSON, definition, executable})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func exactExecutable(observed process.Process, present bool) (process.ExecutableIdentity, error) {
	if executable, ok := observed.Executable(); present && ok {
		return executable, nil
	}
	return process.ExecutableIdentity{}, errors.New("exact executable identity is unavailable")
}
