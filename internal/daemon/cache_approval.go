package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const cacheApprovalLifetime = 2 * time.Minute
const maxCacheApprovals = 128

type cacheApproval struct {
	kind          string
	bindingDigest string
	artifact      cacheartifact.Artifact
	quarantine    *cacheartifact.Quarantine
	destination   string
	configuration string
	expires       time.Time
	used          bool
}

// CacheCleanupPreview issues quarantine authority for one current recommendation.
func (d *Daemon) CacheCleanupPreview(ctx context.Context, req api.CachePreviewRequest) (api.CachePreviewResponse, error) {
	if req.ArtifactID == "" || req.PolicyID == "" {
		return api.CachePreviewResponse{}, errors.New("cache cleanup preview requires artifact_id and policy_id")
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if !d.cachePolicyEnabled(req.PolicyID) {
		return api.CachePreviewResponse{}, errors.New("cache recommendation authority is disabled")
	}
	artifact, err := d.currentCacheArtifact(ctx, req.ArtifactID)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	if artifact.Lifecycle != cacheartifact.StateRecommended || artifact.PolicyID != req.PolicyID {
		return api.CachePreviewResponse{}, errors.New("artifact is not a current recommendation for that exact policy")
	}
	if d.cfg.Cache.MaxEntriesPerAction < 1 || artifact.Identity.Size > int64(d.cfg.Cache.MaxBytesPerAction) {
		return api.CachePreviewResponse{}, errors.New("artifact exceeds the current cache action bounds")
	}
	destination := filepath.Join(cacheartifact.QuarantineDirectory, artifact.ID)
	return d.issueCacheApproval(ctx, "cleanup", artifact, nil, destination, req.PolicyID)
}

// CacheRestorePreview issues exact restoration authority.
func (d *Daemon) CacheRestorePreview(ctx context.Context, req api.CachePreviewRequest) (api.CachePreviewResponse, error) {
	if req.ArtifactID == "" {
		return api.CachePreviewResponse{}, errors.New("cache restore preview requires artifact_id")
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	artifact, err := d.store.CacheArtifact(ctx, req.ArtifactID)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	if !d.cachePolicyEnabled(artifact.PolicyID) {
		return api.CachePreviewResponse{}, errors.New("cache restore authority is disabled")
	}
	item, err := d.store.CacheQuarantine(ctx, req.ArtifactID)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	if item.Status != "quarantined" {
		return api.CachePreviewResponse{}, errors.New("artifact is not currently quarantined")
	}
	if item.Identity.Size > int64(d.cfg.Cache.MaxBytesPerAction) {
		return api.CachePreviewResponse{}, errors.New("artifact exceeds the current cache action byte bound")
	}
	return d.issueCacheApproval(ctx, "restore", artifact, &item, item.OriginalPath, "")
}

// CachePurgePreview issues separate grace-gated purge authority.
func (d *Daemon) CachePurgePreview(ctx context.Context, req api.CachePreviewRequest) (api.CachePreviewResponse, error) {
	if req.ArtifactID == "" || req.PolicyID == "" {
		return api.CachePreviewResponse{}, errors.New("cache purge preview requires artifact_id and policy_id")
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if !d.cachePolicyEnabled(req.PolicyID) {
		return api.CachePreviewResponse{}, errors.New("cache purge authority is disabled")
	}
	artifact, err := d.store.CacheArtifact(ctx, req.ArtifactID)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	if artifact.PolicyID != req.PolicyID {
		return api.CachePreviewResponse{}, errors.New("purge policy does not match the artifact quarantine authority")
	}
	item, err := d.store.CacheQuarantine(ctx, req.ArtifactID)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	if item.Status != "quarantined" || d.cacheClock().UnixNano() < item.GraceUntilNs {
		return api.CachePreviewResponse{}, errors.New("artifact is not purge-eligible or its quarantine grace period has not elapsed")
	}
	if item.Identity.Size > int64(d.cfg.Cache.MaxBytesPerAction) {
		return api.CachePreviewResponse{}, errors.New("artifact exceeds the current cache action byte bound")
	}
	return d.issueCacheApproval(ctx, "purge", artifact, &item, item.QuarantinePath, req.PolicyID)
}

func (d *Daemon) issueCacheApproval(ctx context.Context, kind string, artifact cacheartifact.Artifact,
	quarantine *cacheartifact.Quarantine, destination, policyID string) (api.CachePreviewResponse, error) {
	token, digest, err := newSecret(32)
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	binding, err := cacheApprovalBinding(kind, artifact, quarantine, destination, d.cfg.Cache.Digest())
	if err != nil {
		return api.CachePreviewResponse{}, err
	}
	now := d.cacheClock()
	approval := &cacheApproval{
		kind: kind, bindingDigest: binding, artifact: artifact, quarantine: quarantine,
		destination: destination, configuration: d.cfg.Cache.Digest(), expires: now.Add(cacheApprovalLifetime),
	}
	d.actionMu.Lock()
	d.pruneCacheApprovals(now)
	if len(d.cacheApprovals) >= maxCacheApprovals {
		d.actionMu.Unlock()
		return api.CachePreviewResponse{}, errors.New("too many outstanding cache approvals")
	}
	d.cacheApprovals[digest] = approval
	d.actionMu.Unlock()
	if err := d.store.AppendAudit(ctx, storage.AuditRecord{
		TsNs: now.UnixNano(), Kind: "cache." + kind + ".previewed", Subject: artifact.ID,
		Summary:      fmt.Sprintf("single-use cache %s preview issued; expires at %s", kind, approval.expires.Format(time.RFC3339)),
		EvidenceJSON: mustJSON([]string{"binding " + binding, "configuration " + approval.configuration}),
	}); err != nil {
		d.actionMu.Lock()
		delete(d.cacheApprovals, digest)
		d.actionMu.Unlock()
		return api.CachePreviewResponse{}, err
	}
	command := fmt.Sprintf("ghostgc cache %s --apply --approval %s --yes", kind, token)
	if kind == "purge" {
		command += " --confirm " + artifact.ID
	}
	return api.CachePreviewResponse{
		Action: kind, Approval: token, ExpiresNs: approval.expires.UnixNano(), Artifact: artifact,
		Quarantine: quarantine, Destination: destination, Command: command,
		Revalidation: []string{"configuration and one-artifact authority unchanged", "provider root and exact inode unchanged", "no symlink, hard-link, uid, mount or bound changed", "complete metadata-only manifest unchanged", "destination remains absent"},
		Note:         "No filesystem mutation occurred. The approval is memory-only, single-use and expires in two minutes.",
	}, nil
}

func (d *Daemon) currentCacheArtifact(ctx context.Context, id string) (cacheartifact.Artifact, error) {
	artifacts, err := d.store.ListCacheArtifacts(ctx, "", true)
	if err != nil {
		return cacheartifact.Artifact{}, err
	}
	for _, artifact := range artifacts {
		if artifact.ID == id {
			return artifact, nil
		}
	}
	return cacheartifact.Artifact{}, storage.ErrNotFound
}

func (d *Daemon) cachePolicyEnabled(id string) bool {
	if !d.cacheHealthy || !d.filesystemMutationsHealthy() || !d.cacheGlobalRecommendEnabled() {
		return false
	}
	for _, policy := range d.cfg.Cache.Policies {
		if policy.ID == id && policy.Enabled && policy.Mode == config.ModeRecommend {
			return true
		}
	}
	return false
}

func (d *Daemon) cacheGlobalRecommendEnabled() bool {
	return d.cfg.Cache.Enabled && d.cfg.Cache.GlobalMode == config.ModeRecommend
}

func (d *Daemon) consumeCacheApproval(token string, now time.Time) (*cacheApproval, string) {
	digest := secretDigest(token)
	d.actionMu.Lock()
	defer d.actionMu.Unlock()
	approval := d.cacheApprovals[digest]
	if approval == nil {
		return nil, "cache approval is unknown or was lost when the daemon restarted"
	}
	if approval.used {
		return approval, "cache approval has already been consumed"
	}
	approval.used = true
	if !now.Before(approval.expires) {
		return approval, "cache approval has expired"
	}
	return approval, ""
}

func (d *Daemon) pruneCacheApprovals(now time.Time) {
	for digest, approval := range d.cacheApprovals {
		if approval.expires.Add(cacheApprovalLifetime).Before(now) {
			delete(d.cacheApprovals, digest)
		}
	}
}

func cacheApprovalBinding(kind string, artifact cacheartifact.Artifact, quarantine *cacheartifact.Quarantine, destination, configuration string) (string, error) {
	raw, err := json.Marshal(struct {
		Kind          string                    `json:"kind"`
		Artifact      cacheartifact.Artifact    `json:"artifact"`
		Quarantine    *cacheartifact.Quarantine `json:"quarantine,omitempty"`
		Destination   string                    `json:"destination"`
		Configuration string                    `json:"configuration"`
	}{kind, artifact, quarantine, destination, configuration})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
