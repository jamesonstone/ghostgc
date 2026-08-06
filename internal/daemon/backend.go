package daemon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/repository"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/version"
)

// shortIDLength is how much of a session id the CLI shows by default. Lookups
// accept any unambiguous prefix, so a collision is reported rather than
// silently resolved.
const shortIDLength = 8

// authorityNote states the current action-authority boundary.
const authorityNote = "Automatic cleanup can enforce at most one exact current orphaned candidate per evaluation when global and policy automatic authority are explicit. Manual cleanup remains available; every path fully revalidates and sends SIGTERM only."

func shortID(id string) string {
	if len(id) <= shortIDLength {
		return id
	}
	return id[:shortIDLength]
}

// Status implements api.Backend.
func (d *Daemon) Status(ctx context.Context) (api.StatusResponse, error) {
	d.mu.RLock()
	snap := d.snapshot
	degraded := append([]string(nil), d.degraded...)
	d.mu.RUnlock()

	manualCleanup := d.manualCleanupEnabled()
	resp := api.StatusResponse{
		Health:                  api.HealthHealthy,
		Mode:                    string(d.cfg.GlobalMode),
		Version:                 version.String(),
		Platform:                d.plat.Name(),
		PID:                     d.selfPI,
		StartedNs:               d.startedAt.UnixNano(),
		UptimeSeconds:           time.Since(d.startedAt).Seconds(),
		Agents:                  d.agentIDs(),
		SessionsByState:         map[string]int{},
		ClassificationsByState:  map[string]int{},
		WorktreesByState:        map[string]int{},
		CleanupCandidates:       0,
		SignallingEnabled:       manualCleanup,
		ManualCleanupEnabled:    manualCleanup,
		AutomaticCleanupEnabled: d.automaticCleanupEnabled(),
		CacheEnabled:            d.cfg.Cache.Enabled,
		CacheMode:               string(d.cfg.Cache.GlobalMode),
		Degraded:                degraded,
	}
	switch {
	case len(degraded) > 0:
		resp.Health = api.HealthDegraded
	case snap == nil:
		resp.Health = api.HealthStarting
	}

	recs, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return api.StatusResponse{}, err
	}
	for _, s := range recs {
		resp.SessionsByState[s.State]++
	}
	resp.Sessions = len(recs)
	classCounts, err := d.store.ClassificationCounts(ctx, "")
	if err != nil {
		return api.StatusResponse{}, err
	}
	resp.ClassificationsByState = classCounts
	worktreeCounts, err := d.store.WorktreeStateCounts(ctx)
	if err != nil {
		return api.StatusResponse{}, err
	}
	resp.WorktreesByState = worktreeCounts
	resp.StaleWorktrees = worktreeCounts["stale"]
	resp.ProtectedWorktrees = worktreeCounts["protected"]
	decisions, err := d.currentPolicyDecisions(ctx)
	if err != nil {
		return api.StatusResponse{}, err
	}
	for _, decision := range decisions {
		if d.isRecommendation(decision) || d.isEnforceable(decision) {
			resp.CleanupCandidates++
		}
	}
	resp.CandidateDiagnostics = d.candidateDiagnostics(recs, classCounts, decisions, snap)
	cacheCandidates, err := d.CacheCandidates(ctx)
	if err != nil {
		return api.StatusResponse{}, err
	}
	resp.CacheCandidates = len(cacheCandidates.Artifacts)
	quarantines, err := d.store.ListCacheQuarantines(ctx, "quarantined")
	if err != nil {
		return api.StatusResponse{}, err
	}
	resp.CacheQuarantined = len(quarantines)

	if scan, err := d.store.LastScan(ctx); err == nil {
		resp.LastScan = &api.ScanSummary{
			StartedNs:           scan.StartedNs,
			AgeSeconds:          time.Since(time.Unix(0, scan.StartedNs)).Seconds(),
			DurationMs:          float64(scan.DurationUs) / 1000,
			VisibleProcesses:    scan.VisibleProcesses,
			InspectedProcesses:  scan.InspectedProcesses,
			AttributedProcesses: scan.AttributedProcesses,
			Error:               scan.Error,
		}
	}
	return resp, nil
}

// Sessions implements api.Backend.
func (d *Daemon) Sessions(ctx context.Context, opts api.ListOptions) (api.SessionsResponse, error) {
	filter := storage.SessionFilter{AgentID: opts.AgentID, Limit: opts.Limit}
	if opts.State != "" {
		filter.States = []string{opts.State}
	}
	recs, err := d.store.ListSessions(ctx, filter)
	if err != nil {
		return api.SessionsResponse{}, err
	}
	out := api.SessionsResponse{Sessions: make([]api.SessionSummary, 0, len(recs))}
	for _, rec := range recs {
		summary, err := d.sessionSummary(ctx, rec)
		if err != nil {
			return api.SessionsResponse{}, err
		}
		out.Sessions = append(out.Sessions, summary)
	}
	return out, nil
}

func (d *Daemon) sessionSummary(ctx context.Context, rec storage.SessionRecord) (api.SessionSummary, error) {
	procs, err := d.store.ListProcesses(ctx, storage.ProcessFilter{SessionID: rec.SessionID})
	if err != nil {
		return api.SessionSummary{}, err
	}
	live := 0
	for _, p := range procs {
		if p.ExitedAtNs == nil {
			live++
		}
	}
	end := time.Now()
	if rec.EndedNs != nil {
		end = time.Unix(0, *rec.EndedNs)
	}
	summary := api.SessionSummary{
		SessionID:       rec.SessionID,
		ShortID:         shortID(rec.SessionID),
		AgentID:         rec.AgentID,
		Repository:      repository.Name(rec.RepositoryPath),
		RepositoryPath:  rec.RepositoryPath,
		WorkingDir:      rec.WorkingDir,
		State:           rec.State,
		Confidence:      rec.Confidence,
		RootPID:         rec.RootPID,
		TTY:             rec.TTY,
		AgeSeconds:      end.Sub(time.Unix(0, rec.StartedNs)).Seconds(),
		Processes:       len(procs),
		LiveProcesses:   live,
		StartedNs:       rec.StartedNs,
		LastSeenNs:      rec.LastSeenNs,
		EndedNs:         rec.EndedNs,
		NativeSessionID: rec.NativeSessionID,
		PreviousState:   rec.PreviousState,
		StateChangedNs:  rec.StateChangedNs,
		Branch:          rec.Branch,
		RepositoryBusy:  rec.RepositoryBusy,
		TerminalSID:     rec.TerminalSID,
		LaunchedByPID:   rec.HostPID,
		LaunchedByPath:  rec.HostExecPath,
		Classifications: map[string]int{},
	}
	classes, err := d.store.ClassificationCounts(ctx, rec.SessionID)
	if err != nil {
		return api.SessionSummary{}, err
	}
	summary.Classifications = classes
	if rec.HostName != "" {
		summary.LaunchedBy = rec.HostName
	}
	return summary, nil
}

// relationshipViews converts stored edges for display, resolving the PID of
// each end so the output is readable without a lookup table.
func relationshipViews(recs []storage.RelationshipRecord) []api.RelationshipView {
	out := make([]api.RelationshipView, 0, len(recs))
	for _, rec := range recs {
		view := api.RelationshipView{
			Kind:        rec.Kind,
			From:        rec.FromProcUID,
			To:          rec.ToProcUID,
			Detail:      rec.Detail,
			Attributing: sessions.AttributingKinds[sessions.RelationshipKind(rec.Kind)],
			FirstSeenNs: rec.FirstSeenNs,
			LastSeenNs:  rec.LastSeenNs,
		}
		if key, err := process.ParseKey(rec.FromProcUID); err == nil {
			view.FromPID = key.PID
		}
		if rec.ToProcUID != "" {
			if key, err := process.ParseKey(rec.ToProcUID); err == nil {
				view.ToPID = key.PID
			}
		}
		out = append(out, view)
	}
	return out
}

// Session implements api.Backend.
func (d *Daemon) Session(ctx context.Context, idOrPrefix string) (api.SessionDetail, error) {
	rec, err := d.store.GetSession(ctx, idOrPrefix)
	if err != nil {
		return api.SessionDetail{}, err
	}
	summary, err := d.sessionSummary(ctx, rec)
	if err != nil {
		return api.SessionDetail{}, err
	}
	procs, err := d.store.ListProcesses(ctx, storage.ProcessFilter{SessionID: rec.SessionID})
	if err != nil {
		return api.SessionDetail{}, err
	}
	audit, err := d.store.ListAudit(ctx, storage.AuditFilter{Subject: rec.SessionID, Limit: 50})
	if err != nil {
		return api.SessionDetail{}, err
	}
	rels, err := d.store.SessionRelationships(ctx, rec.SessionID)
	if err != nil {
		return api.SessionDetail{}, err
	}

	detail := api.SessionDetail{Session: summary, Audit: audit, Relationships: relationshipViews(rels)}
	if err := json.Unmarshal([]byte(rec.EvidenceJSON), &detail.Evidence); err != nil {
		detail.Evidence = nil
	}
	for _, p := range procs {
		detail.Processes = append(detail.Processes, d.processSummary(p))
	}
	return detail, nil
}
