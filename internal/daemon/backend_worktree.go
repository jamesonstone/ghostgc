package daemon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Worktrees implements api.Backend.
func (d *Daemon) Worktrees(ctx context.Context, opts api.WorktreeOptions) (api.WorktreesResponse, error) {
	records, err := d.store.ListWorktrees(ctx, storage.WorktreeFilter{State: opts.State, Source: opts.Source, Limit: opts.Limit})
	if err != nil {
		return api.WorktreesResponse{}, err
	}
	response := api.WorktreesResponse{Worktrees: make([]api.WorktreeView, 0, len(records))}
	for _, record := range records {
		response.Worktrees = append(response.Worktrees, worktreeView(record, time.Now()))
	}
	return response, nil
}

// Worktree implements api.Backend.
func (d *Daemon) Worktree(ctx context.Context, idOrPrefix string) (api.WorktreeView, error) {
	record, err := d.store.GetWorktree(ctx, idOrPrefix)
	if err != nil {
		return api.WorktreeView{}, err
	}
	return worktreeView(record, time.Now()), nil
}

func worktreeView(record storage.WorktreeRecord, now time.Time) api.WorktreeView {
	view := api.WorktreeView{
		WorktreeID: record.WorktreeID, ShortID: shortID(record.WorktreeID), Path: record.Path,
		Branch: record.Branch, HEAD: record.HEAD, Ref: record.Ref, State: record.State,
		FirstSeenNs: record.FirstSeenNs, LastSeenNs: record.LastSeenNs,
		LastActivityNs: record.LastActivityNs, InactiveSinceNs: record.InactiveSinceNs,
		Complete: record.Complete, RemovedNs: record.RemovedNs, RecreateCommand: record.RecreateCommand,
		OriginalPath: record.OriginalPath, RetiredNs: record.RetiredNs,
		RetirementGraceNs: record.RetirementGraceNs,
	}
	if record.InactiveSinceNs > 0 {
		view.InactiveSeconds = now.Sub(time.Unix(0, record.InactiveSinceNs)).Seconds()
		if view.InactiveSeconds < 0 {
			view.InactiveSeconds = 0
		}
	}
	_ = json.Unmarshal([]byte(record.SourcesJSON), &view.Sources)
	_ = json.Unmarshal([]byte(record.ProtectionJSON), &view.Protection)
	if json.Valid([]byte(record.EvidenceJSON)) {
		view.Evidence = json.RawMessage(record.EvidenceJSON)
	} else {
		view.Evidence = json.RawMessage(`{}`)
	}
	if view.Sources == nil {
		view.Sources = []string{}
	}
	if view.Protection == nil {
		view.Protection = []string{}
	}
	return view
}

// WorktreeActions implements api.Backend.
func (d *Daemon) WorktreeActions(ctx context.Context, opts api.WorktreeActionOptions) (api.WorktreeActionsResponse, error) {
	filterID := opts.WorktreeID
	if filterID != "" {
		record, err := d.store.GetWorktree(ctx, filterID)
		if err != nil {
			return api.WorktreeActionsResponse{}, err
		}
		filterID = record.WorktreeID
	}
	records, err := d.store.ListWorktreeActions(ctx, storage.WorktreeActionFilter{WorktreeID: filterID, Result: opts.Result, Limit: opts.Limit})
	if err != nil {
		return api.WorktreeActionsResponse{}, err
	}
	response := api.WorktreeActionsResponse{Actions: make([]api.WorktreeActionView, 0, len(records))}
	for _, record := range records {
		evidence := json.RawMessage(`[]`)
		if json.Valid([]byte(record.EvidenceJSON)) {
			evidence = json.RawMessage(record.EvidenceJSON)
		}
		response.Actions = append(response.Actions, api.WorktreeActionView{
			ActionID: record.ActionID, WorktreeID: record.WorktreeID, Path: record.Path,
			Branch: record.Branch, RequestedNs: record.RequestedNs, UpdatedNs: record.UpdatedNs,
			Result: record.Result, Reason: record.Reason, Evidence: evidence,
			RecreateCommand: record.RecreateCommand,
		})
	}
	return response, nil
}
