package daemon

import (
	"context"
	"errors"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachepolicy"
)

func (d *Daemon) freshCleanupArtifact(ctx context.Context, approval *cacheApproval) (cacheartifact.Artifact, []string, error) {
	if approval.configuration != d.cfg.Cache.Digest() || !d.cachePolicyEnabled(approval.artifact.PolicyID) {
		return cacheartifact.Artifact{}, nil, errors.New("cache configuration or recommendation authority changed")
	}
	current, err := d.currentCacheArtifact(ctx, approval.artifact.ID)
	if err != nil {
		return cacheartifact.Artifact{}, nil, err
	}
	binding, err := cacheApprovalBinding("cleanup", current, nil, approval.destination, d.cfg.Cache.Digest())
	if err != nil || binding != approval.bindingDigest {
		return cacheartifact.Artifact{}, nil, errors.New("committed cache decision or approval binding changed")
	}
	sessions, err := d.cacheSessionFacts(ctx)
	if err != nil {
		return cacheartifact.Artifact{}, nil, err
	}
	observed, err := d.cacheProvider.Observe(ctx, sessions, d.cacheFS, d.cfg.Cache.MaxEntriesPerScan)
	if err != nil {
		return cacheartifact.Artifact{}, nil, err
	}
	if !observed.Complete {
		return cacheartifact.Artifact{}, nil, errors.New("fresh provider inspection was incomplete")
	}
	var exact *cacheartifact.Artifact
	for i := range observed.Artifacts {
		if observed.Artifacts[i].ID == current.ID {
			exact = &observed.Artifacts[i]
			break
		}
	}
	if exact == nil {
		return cacheartifact.Artifact{}, nil, errors.New("approved artifact is absent from the fresh provider observation")
	}
	fresh, _ := cachepolicy.Evaluate(d.cfg.Cache, d.cacheClock(), *exact, &current)
	if fresh.Lifecycle != cacheartifact.StateRecommended || fresh.PolicyID != current.PolicyID {
		return fresh, fresh.Evidence, errors.New("fresh provider, session or policy facts no longer recommend the artifact")
	}
	if !fresh.Identity.Equal(current.Identity) || fresh.ManifestDigest != current.ManifestDigest ||
		fresh.SessionID != current.SessionID || fresh.RootPath != current.RootPath ||
		fresh.RelativePath != current.RelativePath || !fresh.RootIdentity.SameObject(current.RootIdentity) {
		return fresh, fresh.Evidence, errors.New("exact artifact identity, manifest, ownership or provider root changed")
	}
	evidence := append([]string(nil), fresh.Evidence...)
	evidence = append(evidence, "fresh provider observation retained the exact approved identity and recommendation")
	return fresh, evidence, nil
}

func (d *Daemon) currentQuarantineApproval(ctx context.Context, approval *cacheApproval) (cacheartifact.Quarantine, []string, error) {
	if approval.configuration != d.cfg.Cache.Digest() || !d.cachePolicyEnabled(approval.artifact.PolicyID) {
		return cacheartifact.Quarantine{}, nil, errors.New("cache configuration or global authority changed")
	}
	artifact, err := d.store.CacheArtifact(ctx, approval.artifact.ID)
	if err != nil {
		return cacheartifact.Quarantine{}, nil, err
	}
	item, err := d.store.CacheQuarantine(ctx, approval.artifact.ID)
	if err != nil {
		return cacheartifact.Quarantine{}, nil, err
	}
	if item.Status != "quarantined" {
		return item, nil, errors.New("artifact is no longer quarantined")
	}
	expectedManifest := cacheartifact.ManifestDigest(item.QuarantinePath, item.Identity)
	if item.ManifestDigest != expectedManifest || !artifact.Identity.Equal(item.Identity) ||
		artifact.ManifestDigest != item.ManifestDigest || artifact.QuarantineDigest != item.ManifestDigest ||
		artifact.QuarantinePath != item.QuarantinePath || artifact.RootPath != item.RootPath {
		return item, nil, errors.New("durable quarantine identity or metadata-only manifest is inconsistent")
	}
	binding, err := cacheApprovalBinding(approval.kind, artifact, &item, approval.destination, d.cfg.Cache.Digest())
	if err != nil || binding != approval.bindingDigest {
		return item, nil, errors.New("quarantine identity, manifest, destination or approval binding changed")
	}
	evidence := []string{"durable quarantine record and configuration still match the single-use approval"}
	return item, evidence, nil
}
