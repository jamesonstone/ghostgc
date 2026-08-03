package daemon_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func enforcementHarness(t *testing.T, global config.Mode, targets int, finalPath string) (*harness, []process.Process) {
	t.Helper()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	children := make([]process.Process, 0, targets)
	for i := range targets {
		child := mk(200+i, 100, "/opt/ghostgc-fixtures/safe-helper", time.Duration(i+2)*time.Second)
		child.PGID, child.SID = root.PGID, root.SID
		children = append(children, child)
	}
	snaps := []*process.Snapshot{snapshot(time.Minute, append([]process.Process{init, root}, children...)...)}
	for minute := 2; minute <= 8; minute++ {
		procs := []process.Process{init}
		for _, child := range children {
			procs = append(procs, withParent(child, 1))
		}
		snaps = append(snaps, snapshot(time.Duration(minute)*time.Minute, procs...))
	}
	final := []process.Process{init}
	for _, child := range children {
		child = withParent(child, 1)
		if finalPath != "" && child.PID == children[0].PID {
			child.ExecPath = finalPath
		}
		final = append(final, child)
	}
	snaps = append(snaps, snapshot(8*time.Minute+5*time.Second, final...))

	cfg := config.Default()
	cfg.GlobalMode = global
	cfg.Sampling.ActivitySample = config.Duration(time.Minute)
	cfg.Sampling.Classification = config.Duration(time.Minute)
	cfg.Sampling.PolicyEvaluation = config.Duration(time.Minute)
	cfg.Policies = []config.Policy{{
		ID: "safe-helper", Description: "enforce orphaned fixture helper",
		Enabled: true, Mode: config.ModeEnforce, Automatic: true,
		States: []string{"orphaned"}, Agents: []string{"codex"}, Executables: []string{"safe-helper"},
		RequireDetached: true, RequireSessionEnded: true,
		MinStable: config.Duration(5 * time.Minute), Cooldown: config.Duration(time.Hour),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("enforcement config: %v", err)
	}
	h := newHarnessConfig(t, cfg, snaps...)
	h.fake.SetActivity(root.Key(), completeSample(root.Key(), time.Minute, time.Second))
	for _, child := range children {
		var samples []process.ActivitySample
		for minute := 1; minute <= 8; minute++ {
			at := time.Duration(minute)*time.Minute + time.Second
			if minute == 7 {
				at = 7 * time.Minute
			}
			samples = append(samples, completeSample(child.Key(), at, time.Second))
		}
		samples = append(samples, completeSample(child.Key(), 8*time.Minute+6*time.Second, time.Second))
		h.fake.SetActivity(child.Key(), samples...)
	}
	return h, children
}

func TestAutomaticCleanupSignalsOneExactCurrentCandidate(t *testing.T) {
	h, children := enforcementHarness(t, config.ModeEnforce, 1, "")
	for range 8 {
		h.d.ScanNow(context.Background())
	}
	if h.fake.SignalAttempts != 1 || len(h.fake.Signals) != 1 || h.fake.Signals[0].Key != children[0].Key() {
		t.Fatalf("signals = %+v, attempts=%d", h.fake.Signals, h.fake.SignalAttempts)
	}
	actions, err := h.store.ListActions(context.Background(), storage.ActionFilter{})
	if err != nil || len(actions) != 1 || actions[0].Authority != "automatic" || actions[0].Result != "signalled" {
		t.Fatalf("automatic actions = %+v, %v", actions, err)
	}
	if !strings.Contains(actions[0].EvidenceJSON, "automatic-enforcement-v1") {
		t.Fatalf("automatic evidence = %s", actions[0].EvidenceJSON)
	}
	candidates, err := h.d.Candidates(context.Background())
	if err != nil || len(candidates.Enforceable) != 1 || len(candidates.Recommended) != 0 {
		t.Fatalf("enforceable projection = %+v, %v", candidates, err)
	}
	status, err := h.d.Status(context.Background())
	if err != nil || !status.SignallingEnabled || !status.AutomaticCleanupEnabled || status.CleanupCandidates != 1 {
		t.Fatalf("enforcement status = %+v, %v", status, err)
	}
	policies, err := h.d.Policies(context.Background())
	if err != nil || len(policies.Policies) != 1 || !policies.Policies[0].Automatic {
		t.Fatalf("enforcement policies = %+v, %v", policies, err)
	}
}

func TestAutomaticCleanupAttemptsAtMostOneCandidatePerEvaluation(t *testing.T) {
	h, _ := enforcementHarness(t, config.ModeEnforce, 2, "")
	for range 8 {
		h.d.ScanNow(context.Background())
	}
	actions, err := h.store.ListActions(context.Background(), storage.ActionFilter{})
	if err != nil || len(actions) != 1 || h.fake.SignalAttempts != 1 {
		t.Fatalf("bounded actions = %+v, %v; attempts=%d", actions, err, h.fake.SignalAttempts)
	}
}

func TestGlobalRecommendCapsEnforcePolicyAtAudit(t *testing.T) {
	h, _ := enforcementHarness(t, config.ModeRecommend, 1, "")
	for range 8 {
		h.d.ScanNow(context.Background())
	}
	candidates, err := h.d.Candidates(context.Background())
	if err != nil || len(candidates.Enforceable) != 0 || len(candidates.Audited) != 1 || h.fake.SignalAttempts != 0 {
		t.Fatalf("capped candidates = %+v, %v; attempts=%d", candidates, err, h.fake.SignalAttempts)
	}
}

func TestAutomaticCleanupRejectsChangedExecutable(t *testing.T) {
	h, _ := enforcementHarness(t, config.ModeEnforce, 1, "/different/safe-helper")
	for range 8 {
		h.d.ScanNow(context.Background())
	}
	actions, err := h.store.ListActions(context.Background(), storage.ActionFilter{})
	if err != nil || len(actions) != 1 || actions[0].Result != "rejected" || h.fake.SignalAttempts != 0 {
		t.Fatalf("changed-image actions = %+v, %v; attempts=%d", actions, err, h.fake.SignalAttempts)
	}
}
