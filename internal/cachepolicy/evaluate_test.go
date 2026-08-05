package cachepolicy

import (
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/cacheartifact"
	"github.com/jamesonstone/ghostgc/internal/config"
)

func TestRecommendationRequiresTwoCommittedStableObservations(t *testing.T) {
	cfg := recommendationConfig()
	observed := eligibleArtifact()
	t0 := time.Unix(100, 0)

	first, decision := Evaluate(cfg, t0, observed, nil)
	if first.Lifecycle != cacheartifact.StateSettling || decision.Result != "settling" {
		t.Fatalf("first observation = %q %q", first.Lifecycle, decision.Result)
	}
	second, decision := Evaluate(cfg, t0.Add(cfg.MinStable.D()), observed, &first)
	if second.Lifecycle != cacheartifact.StateRecommended || decision.Result != "recommended" {
		t.Fatalf("second stable observation = %q %q", second.Lifecycle, decision.Result)
	}
	if second.FirstObservedNs != first.FirstObservedNs || second.StableSinceNs != first.StableSinceNs {
		t.Fatal("stable history must be carried only from committed prior state")
	}
}

func TestChangedMetadataRestartsSettlingWindow(t *testing.T) {
	cfg := recommendationConfig()
	t0 := time.Unix(100, 0)
	first, _ := Evaluate(cfg, t0, eligibleArtifact(), nil)
	changed := eligibleArtifact()
	changed.Identity.Size++
	changed.IdentityDigest = changed.Identity.Digest()
	changed.ManifestDigest = cacheartifact.ManifestDigest(changed.RelativePath, changed.Identity)

	second, _ := Evaluate(cfg, t0.Add(2*cfg.MinStable.D()), changed, &first)
	if second.Lifecycle != cacheartifact.StateSettling || second.StableSinceNs == first.StableSinceNs {
		t.Fatalf("changed identity must restart settling: %#v", second)
	}
}

func TestAuditAndMissingPolicyNeverRecommend(t *testing.T) {
	t0 := time.Unix(100, 0)
	for name, mutate := range map[string]func(*config.Cache){
		"global audit":  func(c *config.Cache) { c.GlobalMode = config.ModeAudit },
		"policy audit":  func(c *config.Cache) { c.Policies[0].Mode = config.ModeAudit },
		"policy absent": func(c *config.Cache) { c.Policies = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := recommendationConfig()
			mutate(&cfg)
			first, _ := Evaluate(cfg, t0, eligibleArtifact(), nil)
			second, decision := Evaluate(cfg, t0.Add(cfg.MinStable.D()), eligibleArtifact(), &first)
			if second.Lifecycle == cacheartifact.StateRecommended || decision.Result == "recommended" {
				t.Fatalf("%s unexpectedly granted mutation authority", name)
			}
		})
	}
}

func TestProviderProtectionCannotBeOverriddenByPolicy(t *testing.T) {
	artifact := eligibleArtifact()
	artifact.Lifecycle = cacheartifact.StateProtected
	artifact.Reason = "active session"
	prior := artifact
	prior.FirstObservedNs = 1
	prior.StableSinceNs = 1
	got, decision := Evaluate(recommendationConfig(), time.Unix(1000, 0), artifact, &prior)
	if got.Lifecycle != cacheartifact.StateProtected || decision.Result != "protected" {
		t.Fatalf("policy overrode provider protection: %#v %#v", got, decision)
	}
}

func TestProtectedHistoryCannotSatisfyStableWindow(t *testing.T) {
	cfg := recommendationConfig()
	previous := eligibleArtifact()
	previous.Lifecycle = cacheartifact.StateProtected
	previous.FirstObservedNs = time.Unix(1, 0).UnixNano()
	previous.StableSinceNs = previous.FirstObservedNs
	got, _ := Evaluate(cfg, time.Unix(1000, 0), eligibleArtifact(), &previous)
	if got.Lifecycle != cacheartifact.StateSettling || got.StableSinceNs == previous.StableSinceNs {
		t.Fatalf("protected or incomplete history advanced stability: %#v", got)
	}
}

func TestArtifactExceedingActionBytesIsProtected(t *testing.T) {
	cfg := recommendationConfig()
	artifact := eligibleArtifact()
	artifact.Identity.Size = int64(cfg.MaxBytesPerAction) + 1
	artifact.IdentityDigest = artifact.Identity.Digest()
	artifact.ManifestDigest = cacheartifact.ManifestDigest(artifact.RelativePath, artifact.Identity)
	got, decision := Evaluate(cfg, time.Unix(100, 0), artifact, nil)
	if got.Lifecycle != cacheartifact.StateProtected || decision.Result != "protected" {
		t.Fatalf("oversized artifact = %#v %#v", got, decision)
	}
}

func recommendationConfig() config.Cache {
	cfg := config.DefaultCache()
	cfg.Enabled = true
	cfg.GlobalMode = config.ModeRecommend
	cfg.MinStable = config.Duration(time.Minute)
	cfg.Policies = []config.CachePolicy{{
		ID: "codex-snapshots", Description: "test", Enabled: true, Mode: config.ModeRecommend,
		Provider: cacheartifact.ProviderCodexShellSnapshot, Agent: cacheartifact.AgentCodex,
		ArtifactKind: cacheartifact.KindShellSnapshot, SessionState: "completed",
	}}
	return cfg
}

func eligibleArtifact() cacheartifact.Artifact {
	identity := cacheartifact.Identity{UID: 501, Device: 1, Inode: 2, Nlink: 1, Size: 8, EntryType: "regular"}
	return cacheartifact.Artifact{
		ID: "ca_test", Provider: cacheartifact.ProviderCodexShellSnapshot, Agent: cacheartifact.AgentCodex,
		SessionID: "session-1", Kind: cacheartifact.KindShellSnapshot, RootPath: "/cache", RelativePath: "thread.1.sh",
		Identity: identity, RootIdentity: cacheartifact.Identity{UID: 501, Device: 1, Inode: 1, EntryType: "directory"},
		IdentityDigest: identity.Digest(), ManifestDigest: cacheartifact.ManifestDigest("thread.1.sh", identity),
		Lifecycle: cacheartifact.StateObserved,
	}
}
