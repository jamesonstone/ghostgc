package daemon

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
)

const cacheBoundaryNote = "Cache observation is limited to exact allowed Codex shell snapshots. Quarantine is reversible containment on the same filesystem and does not reclaim disk space."

// CacheArtifacts implements api.Backend.
func (d *Daemon) CacheArtifacts(ctx context.Context, opts api.CacheArtifactOptions) (api.CacheArtifactsResponse, error) {
	artifacts, err := d.store.ListCacheArtifacts(ctx, opts.Lifecycle, opts.Current)
	if err != nil {
		return api.CacheArtifactsResponse{}, err
	}
	return api.CacheArtifactsResponse{Artifacts: artifacts, Note: cacheBoundaryNote}, nil
}

// CacheArtifact implements api.Backend.
func (d *Daemon) CacheArtifact(ctx context.Context, id string) (api.CacheArtifactResponse, error) {
	artifact, err := d.store.CacheArtifact(ctx, id)
	if err != nil {
		return api.CacheArtifactResponse{}, err
	}
	return api.CacheArtifactResponse{Artifact: artifact, Note: cacheBoundaryNote}, nil
}

// CacheCandidates implements api.Backend.
func (d *Daemon) CacheCandidates(ctx context.Context) (api.CacheArtifactsResponse, error) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if !d.cacheHealthy {
		return api.CacheArtifactsResponse{Note: cacheBoundaryNote + " The newest cache observation is unavailable; no current authority exists."}, nil
	}
	artifacts, err := d.store.ListCacheArtifacts(ctx, "", true)
	if err != nil {
		return api.CacheArtifactsResponse{}, err
	}
	out := make([]cacheartifact.Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Lifecycle == cacheartifact.StateStaleCandidate || artifact.Lifecycle == cacheartifact.StateRecommended {
			out = append(out, artifact)
		}
	}
	return api.CacheArtifactsResponse{Artifacts: out, Note: cacheBoundaryNote}, nil
}

// CacheQuarantines implements api.Backend.
func (d *Daemon) CacheQuarantines(ctx context.Context) (api.CacheQuarantinesResponse, error) {
	items, err := d.store.ListCacheQuarantines(ctx, "quarantined")
	if err != nil {
		return api.CacheQuarantinesResponse{}, err
	}
	return api.CacheQuarantinesResponse{Artifacts: items, Note: cacheBoundaryNote}, nil
}

// CacheActions implements api.Backend.
func (d *Daemon) CacheActions(ctx context.Context, opts api.CacheActionOptions) (api.CacheActionsResponse, error) {
	actions, err := d.store.ListCacheActions(ctx, opts.ArtifactID, opts.Kind, opts.Result, opts.Limit)
	if err != nil {
		return api.CacheActionsResponse{}, err
	}
	return api.CacheActionsResponse{Actions: actions}, nil
}
