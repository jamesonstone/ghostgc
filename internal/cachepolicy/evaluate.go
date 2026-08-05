// Package cachepolicy evaluates cache candidates independently from process policy.
package cachepolicy

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/config"
)

// Evaluate advances one observation using only committed previous state.
func Evaluate(cacheConfig config.Cache, now time.Time, observed cacheartifact.Artifact, previous *cacheartifact.Artifact) (cacheartifact.Artifact, cacheartifact.Decision) {
	artifact := observed
	artifact.Configuration = cacheConfig.Digest()
	artifact.FirstObservedNs = now.UnixNano()
	artifact.LastObservedNs = now.UnixNano()
	artifact.StableSinceNs = now.UnixNano()
	decision := cacheartifact.Decision{ArtifactID: artifact.ID, Result: "protected", Reason: artifact.Reason, Evidence: append([]string(nil), artifact.Evidence...)}
	if artifact.Identity.Size < 0 || artifact.Identity.Size > int64(cacheConfig.MaxBytesPerAction) {
		artifact.Lifecycle = cacheartifact.StateProtected
		artifact.Reason = "artifact size exceeds the configured cache action byte bound"
		artifact.Evidence = append(artifact.Evidence, artifact.Reason)
		decision.Reason = artifact.Reason
		decision.Evidence = artifact.Evidence
	}

	if previous != nil {
		artifact.FirstObservedNs = previous.FirstObservedNs
		if stableHistoryEligible(previous.Lifecycle) && previous.IdentityDigest == artifact.IdentityDigest && previous.ManifestDigest == artifact.ManifestDigest {
			artifact.StableSinceNs = previous.StableSinceNs
		}
	}
	if artifact.Lifecycle == cacheartifact.StateProtected {
		return artifact, decision
	}
	stableFor := now.Sub(time.Unix(0, artifact.StableSinceNs))
	if previous == nil || stableFor < cacheConfig.MinStable.D() {
		artifact.Lifecycle = cacheartifact.StateSettling
		artifact.Reason = fmt.Sprintf("unchanged committed observations span %s; %s required", stableFor.Truncate(time.Second), cacheConfig.MinStable.D())
		artifact.Evidence = append(artifact.Evidence, artifact.Reason)
		decision.Result = "settling"
		decision.Reason = artifact.Reason
		decision.Evidence = artifact.Evidence
		return artifact, decision
	}

	artifact.Lifecycle = cacheartifact.StateStaleCandidate
	artifact.Reason = "two committed observations prove unchanged identity and manifest across the stable window"
	artifact.Evidence = append(artifact.Evidence, artifact.Reason)
	decision.Result = "no_policy"
	decision.Reason = "no enabled exact cache policy grants recommendation authority"
	decision.Evidence = artifact.Evidence

	policy := enabledPolicy(cacheConfig.Policies)
	if policy == nil {
		return artifact, decision
	}
	artifact.PolicyID = policy.ID
	decision.PolicyID = policy.ID
	if cacheConfig.GlobalMode == config.ModeRecommend && policy.Mode == config.ModeRecommend {
		artifact.Lifecycle = cacheartifact.StateRecommended
		artifact.Reason = "cache-global recommend authority and one exact policy permit a manual preview"
		decision.Result = "recommended"
		decision.Reason = artifact.Reason
	} else {
		decision.Result = "audit"
		decision.Reason = "candidate is audit-only; cache-global and policy recommend authority are both required"
	}
	decision.Evidence = artifact.Evidence
	return artifact, decision
}

func stableHistoryEligible(lifecycle cacheartifact.Lifecycle) bool {
	switch lifecycle {
	case cacheartifact.StateObserved, cacheartifact.StateSettling,
		cacheartifact.StateStaleCandidate, cacheartifact.StateRecommended,
		cacheartifact.StateRestored:
		return true
	default:
		return false
	}
}

func enabledPolicy(policies []config.CachePolicy) *config.CachePolicy {
	for i := range policies {
		if policies[i].Enabled {
			return &policies[i]
		}
	}
	return nil
}
