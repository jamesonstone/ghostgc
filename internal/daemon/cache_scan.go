package daemon

import (
	"context"
	"time"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/cachepolicy"
)

func (d *Daemon) runCacheScan(ctx context.Context, observedAt time.Time) {
	if !d.cfg.Cache.Enabled {
		return
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	start := time.Now()

	previous, err := d.store.CacheArtifactMap(ctx)
	if err != nil {
		d.recordCacheFailure(ctx, observedAt, start, err)
		return
	}
	sessions, err := d.cacheSessionFacts(ctx)
	if err != nil {
		d.recordCacheFailure(ctx, observedAt, start, err)
		return
	}
	result, err := d.cacheProvider.Observe(ctx, sessions, d.cacheFS, d.cfg.Cache.MaxEntriesPerScan)
	if err != nil {
		d.recordCacheFailure(ctx, observedAt, start, err)
		return
	}

	artifacts := make([]cacheartifact.Artifact, 0, len(result.Artifacts))
	decisions := make([]cacheartifact.Decision, 0, len(result.Artifacts))
	evaluation := cacheartifact.Evaluation{
		ObservedNs: observedAt.UnixNano(), ConfigurationDigest: d.cfg.Cache.Digest(),
		Complete: result.Complete, Inspected: result.Inspected,
	}
	for _, observed := range result.Artifacts {
		var prior *cacheartifact.Artifact
		if item, ok := previous[observed.ID]; ok {
			copy := item
			prior = &copy
		}
		artifact, decision := cachepolicy.Evaluate(d.cfg.Cache, observedAt, observed, prior)
		artifacts = append(artifacts, artifact)
		decisions = append(decisions, decision)
		switch artifact.Lifecycle {
		case cacheartifact.StateProtected:
			evaluation.Protected++
		case cacheartifact.StateStaleCandidate, cacheartifact.StateRecommended:
			evaluation.Candidates++
		}
	}
	if _, err := d.store.PersistCacheEvaluation(ctx, evaluation, artifacts, decisions); err != nil {
		d.recordCacheFailure(ctx, observedAt, start, err)
		return
	}
	d.cacheHealthy = result.Complete
	d.mu.Lock()
	d.metrics.cacheScanCount++
	d.metrics.lastCacheDuration = time.Since(start)
	d.metrics.cacheInspected += int64(evaluation.Inspected)
	d.metrics.cacheProtected += int64(evaluation.Protected)
	d.metrics.cacheCandidates += int64(evaluation.Candidates)
	d.mu.Unlock()
}

func (d *Daemon) recordCacheFailure(ctx context.Context, observedAt, startedAt time.Time, cause error) {
	d.cacheHealthy = false
	evaluation := cacheartifact.Evaluation{
		ObservedNs: observedAt.UnixNano(), ConfigurationDigest: d.cfg.Cache.Digest(),
		Complete: false, Error: cause.Error(),
	}
	_, _ = d.store.PersistCacheEvaluation(ctx, evaluation, nil, nil)
	d.mu.Lock()
	d.metrics.cacheScanFailures++
	d.metrics.lastCacheDuration = time.Since(startedAt)
	d.mu.Unlock()
	d.log.Error("cache scan failed", "error", cause)
}

// CacheScanAt runs one cache observation at a controlled time for tests.
func (d *Daemon) CacheScanAt(ctx context.Context, observedAt time.Time) {
	d.runCacheScan(ctx, observedAt)
}
