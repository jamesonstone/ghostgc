package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/protection"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Explain implements api.Backend.
//
// Explain answers for any PID on the machine, not only attributed ones,
// because "why is ghostgc ignoring this process" is as important a question as
// "why does ghostgc think this belongs to a session".
func (d *Daemon) Explain(ctx context.Context, pid int) (api.ExplainResponse, error) {
	d.mu.RLock()
	snap := d.snapshot
	tree := d.tree
	last := d.last
	d.mu.RUnlock()

	resp := api.ExplainResponse{PID: pid, PolicyNote: authorityNote}
	if snap == nil {
		resp.Classification = "unknown"
		resp.Message = "no snapshot has been taken yet; the daemon has just started"
		return resp, nil
	}

	p, ok := snap.ByPID(pid)
	if !ok {
		resp.Classification = "unknown"
		resp.Message = fmt.Sprintf("pid %d was not present in the snapshot taken %s ago; it either exited, or it is owned by another user and is therefore never inspected",
			pid, time.Since(snap.Taken).Truncate(time.Second))
		resp.Protection = protection.Result{Protected: true, Rules: []protection.Rule{{
			ID:     "protected-not-observed-v1",
			Reason: "the process is not in the current snapshot, so nothing has been established about it; unknown is protected",
		}}}
		return resp, nil
	}

	resp.Found = true
	resp.ProcUID = p.Key().UID()
	resp.Name = p.Name()
	resp.ExecPath = p.ExecPath
	resp.Cmdline = process.RedactArgs(p.Args)
	resp.ParentLink = string(tree.Link(p.PID))
	resp.Descendants = tree.Descendants(p.PID)
	resp.EnvironmentReadable = p.EnvReadable

	var (
		attr        sessions.Attribution
		isRoot      bool
		sessionLive bool
	)
	if last != nil {
		if a, found := last.Attributions[resp.ProcUID]; found {
			attr = a
		}
		if root, found := last.Roots[p.PID]; found && root.Key == p.Key() {
			isRoot = true
			resp.Conflicts = root.Conflicts
		}
	}
	resp.AgentID = attr.AgentID
	resp.SessionID = attr.SessionID
	resp.Relation = string(attr.Relation)
	resp.Confidence = attr.Confidence
	resp.Evidence = attr.Evidence
	resp.OriginalPPID = attr.OriginalPPID
	resp.OriginalParentObserved = attr.OriginalParentObserved
	resp.RepositoryPath = attr.RepositoryPath
	if rels, relErr := d.store.ProcessRelationships(ctx, resp.ProcUID); relErr == nil {
		resp.Relationships = relationshipViews(rels)
	}
	if len(attr.Conflicts) > 0 {
		resp.Conflicts = append(resp.Conflicts, attr.Conflicts...)
	}

	if attr.SessionID != "" {
		if rec, err := d.store.GetSession(ctx, attr.SessionID); err == nil {
			resp.SessionState = rec.State
			sessionLive = rec.EndedNs == nil
		}
	}
	if classes, err := d.store.ListClassifications(ctx, storage.ClassificationFilter{ProcUID: resp.ProcUID, Latest: true, Limit: 1}); err == nil && len(classes) == 1 {
		resp.ActivityState = classes[0].State
		resp.Detached = classes[0].Detached
		_ = json.Unmarshal([]byte(classes[0].EvidenceJSON), &resp.ActivityEvidence)
	}
	if candidates, err := d.Candidates(ctx); err == nil {
		resp.PolicyDecisions = policyEntriesForProcess(candidates.Audited, resp.ProcUID)
	}

	resp.Protection = d.protectionFor(p, tree, attr.Confidence, isRoot, sessionLive)

	switch {
	case resp.Protection.Protected:
		resp.Classification = "protected"
	case attr.Attributed():
		resp.Classification = "attributed"
	default:
		resp.Classification = "unknown"
	}
	if attr.SessionID == "" {
		resp.Message = "no agent adapter claimed this process and no ownership was ever recorded for it, so it is unattributed and protected"
		if !p.EnvReadable {
			// Saying nothing here would let "no agent variables were set" stand
			// in for "the daemon was not permitted to look", which are very
			// different facts.
			resp.Message += ". Its environment could not be read either: the operating system withholds the environment of system binaries from unprivileged callers, so environment-derived membership could not be evaluated at all"
		}
	}
	return resp, nil
}

// Logs implements api.Backend.
func (d *Daemon) Logs(ctx context.Context, opts api.LogOptions) (api.LogsResponse, error) {
	entries, err := d.store.ListAudit(ctx, storage.AuditFilter{
		SinceNs: opts.SinceNs,
		Kind:    opts.Kind,
		Subject: opts.Subject,
		Limit:   opts.Limit,
	})
	if err != nil {
		return api.LogsResponse{}, err
	}
	return api.LogsResponse{Entries: entries}, nil
}

// Metrics implements api.Backend.
func (d *Daemon) Metrics(ctx context.Context) (api.MetricsResponse, error) {
	d.mu.RLock()
	m := d.metrics
	d.mu.RUnlock()

	counts, err := d.store.Counts(ctx)
	if err != nil {
		return api.MetricsResponse{}, err
	}
	resp := api.MetricsResponse{
		ScanCount:             m.scanCount,
		ScanFailures:          m.scanFailures,
		LastScanDurationMs:    float64(m.lastScanDuration.Microseconds()) / 1000,
		MaxScanDurationMs:     float64(m.maxScanDuration.Microseconds()) / 1000,
		LastReconcileMs:       float64(m.lastReconcile.Microseconds()) / 1000,
		LastPersistMs:         float64(m.lastPersist.Microseconds()) / 1000,
		LastActivityMs:        float64(m.lastActivity.Microseconds()) / 1000,
		ActivitySamples:       m.activitySamples,
		Classifications:       m.classifications,
		PolicyDecisions:       m.policyDecisions,
		VisibleProcesses:      m.visibleProcesses,
		InspectedProcesses:    m.inspectedProcesses,
		AttributedProcesses:   m.attributed,
		ActiveSessions:        int(counts.ActiveSessions),
		DatabaseBytes:         d.store.SizeBytes(),
		DatabaseCounts:        counts,
		RSSBytes:              rssBytes(),
		Goroutines:            runtime.NumGoroutine(),
		RetentionRuns:         m.retentionRuns,
		LastRetentionDeleted:  m.lastRetentionRows,
		CacheScanCount:        m.cacheScanCount,
		CacheScanFailures:     m.cacheScanFailures,
		LastCacheScanMs:       float64(m.lastCacheDuration.Microseconds()) / 1000,
		CacheInspected:        m.cacheInspected,
		CacheProtected:        m.cacheProtected,
		CacheCandidates:       m.cacheCandidates,
		CacheQuarantinedBytes: m.cacheQuarantinedBytes,
		CachePurgedBytes:      m.cachePurgedBytes,
	}
	resp.ActionsAttempted, resp.ActionsRejected, resp.ActionsCompleted, err = d.store.ActionCounts(ctx)
	if err != nil {
		return api.MetricsResponse{}, err
	}
	decisions, err := d.currentPolicyDecisions(ctx)
	if err != nil {
		return api.MetricsResponse{}, err
	}
	for _, decision := range decisions {
		if d.isRecommendation(decision) {
			resp.CleanupCandidates++
		}
	}
	if m.scanCount > 0 {
		resp.MeanScanDurationMs = float64(m.totalScanDuration.Microseconds()) / 1000 / float64(m.scanCount)
	}
	return resp, nil
}
