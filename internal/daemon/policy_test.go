package daemon_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/daemon"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func TestPolicyCandidateCooldownAndLiveProjection(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mk(200, 100, "/opt/ghostgc-fixtures/safe-helper", 2*time.Second)
	child.PGID, child.SID = root.PGID, root.SID
	var snaps []*process.Snapshot
	snaps = append(snaps, snapshot(time.Minute, init, root, child))
	for minute := 2; minute <= 8; minute++ {
		snaps = append(snaps, snapshot(time.Duration(minute)*time.Minute, init, withParent(child, 1)))
	}
	snaps = append(snaps, snapshot(9*time.Minute, init))

	cfg := config.Default()
	cfg.Sampling.ActivitySample = config.Duration(time.Minute)
	cfg.Sampling.Classification = config.Duration(time.Minute)
	cfg.Sampling.PolicyEvaluation = config.Duration(time.Minute)
	cfg.Policies = []config.Policy{{
		ID: "safe-helper", Description: "audit completed fixture helper", Enabled: true, Mode: config.ModeAudit,
		States: []string{"orphaned"}, Agents: []string{"codex"}, Executables: []string{"safe-helper"},
		RequireDetached: true, RequireSessionEnded: true,
		MinStable: config.Duration(5 * time.Minute), Cooldown: config.Duration(time.Hour),
	}}
	h := newHarnessConfig(t, cfg, snaps...)
	h.fake.SetActivity(root.Key(), completeSample(root.Key(), time.Minute, time.Second))
	var samples []process.ActivitySample
	for minute := 1; minute <= 8; minute++ {
		samples = append(samples, completeSample(child.Key(), time.Duration(minute)*time.Minute, time.Second))
	}
	h.fake.SetActivity(child.Key(), samples...)

	for range 7 {
		h.d.ScanNow(ctx)
	}
	resp, err := h.d.Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Enforceable) != 0 || len(resp.Audited) != 1 || resp.Audited[0].Result != "candidate" {
		t.Fatalf("candidate projection = %+v", resp)
	}
	if len(resp.Audited[0].Evidence) == 0 || h.fake.SignalAttempts != 0 {
		t.Fatalf("candidate lacks evidence or attempted a signal: %+v, signals=%d", resp.Audited[0], h.fake.SignalAttempts)
	}
	cold, err := daemon.New(daemon.Options{
		Config: cfg, Paths: h.paths, Store: h.store, Platform: platformtest.New(testUID),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	coldResp, err := cold.Candidates(ctx)
	if err != nil || len(coldResp.Audited) != 0 {
		t.Fatalf("persisted decision leaked before a current snapshot: %+v, %v", coldResp.Audited, err)
	}

	h.d.ScanNow(ctx)
	resp, err = h.d.Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Audited) != 1 || resp.Audited[0].Result != "cooldown" {
		t.Fatalf("second evaluation = %+v, want cooldown", resp.Audited)
	}
	explain, err := h.d.Explain(ctx, child.PID)
	if err != nil || len(explain.PolicyDecisions) != 1 {
		t.Fatalf("explain policy = %+v, %v", explain.PolicyDecisions, err)
	}

	h.d.ScanNow(ctx)
	resp, err = h.d.Candidates(ctx)
	if err != nil || len(resp.Audited) != 0 {
		t.Fatalf("exited process remained current: %+v, %v", resp.Audited, err)
	}
	if h.fake.SignalAttempts != 0 {
		t.Fatalf("policy evaluation attempted %d signals", h.fake.SignalAttempts)
	}
}
