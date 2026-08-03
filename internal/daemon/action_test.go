package daemon_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func recommendHarness(t *testing.T, targetPresent bool) (*harness, process.Process) {
	return recommendHarnessWithFinalPath(t, targetPresent, "")
}

func recommendHarnessWithFinalPath(t *testing.T, targetPresent bool, finalPath string) (*harness, process.Process) {
	t.Helper()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mk(200, 100, "/opt/ghostgc-fixtures/safe-helper", 2*time.Second)
	child.PGID, child.SID = root.PGID, root.SID
	zombie := withParent(child, 1)
	zombie.Status = process.StatusZombie
	finalTarget := zombie
	if finalPath != "" {
		finalTarget.ExecPath = finalPath
	}
	actionSnapshot := snapshot(3*time.Minute, init)
	if targetPresent {
		actionSnapshot = snapshot(3*time.Minute, init, finalTarget)
	}
	cfg := config.Default()
	cfg.GlobalMode = config.ModeRecommend
	cfg.Sampling.ActivitySample = config.Duration(time.Minute)
	cfg.Sampling.Classification = config.Duration(time.Minute)
	cfg.Sampling.PolicyEvaluation = config.Duration(time.Minute)
	cfg.Policies = []config.Policy{{
		ID: "safe-helper", Description: "recommend crashed fixture helper",
		Enabled: true, Mode: config.ModeRecommend, States: []string{"crashed"},
		Agents: []string{"codex"}, Executables: []string{"safe-helper"},
		RequireSessionEnded: true, MinStable: 0, Cooldown: config.Duration(time.Hour),
	}}
	h := newHarnessConfig(t, cfg,
		snapshot(time.Minute, init, root, child),
		snapshot(2*time.Minute, init, zombie),
		actionSnapshot,
	)
	h.fake.SetActivity(root.Key(), completeSample(root.Key(), time.Minute, time.Second))
	h.fake.SetActivity(child.Key(),
		completeSample(child.Key(), time.Minute, time.Second),
		completeSample(child.Key(), 2*time.Minute, time.Second),
		completeSample(child.Key(), 3*time.Minute, time.Second),
	)
	return h, child
}

func previewRecommendation(t *testing.T, h *harness, child process.Process) api.CleanupPreviewResponse {
	t.Helper()
	ctx := context.Background()
	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	candidates, err := h.d.Candidates(ctx)
	if err != nil || len(candidates.Recommended) != 1 || len(candidates.Enforceable) != 0 {
		t.Fatalf("recommendations = %+v, %v", candidates, err)
	}
	preview, err := h.d.CleanupPreview(ctx, api.CleanupPreviewRequest{
		PolicyID: "safe-helper", ProcUID: child.Key().UID(),
	})
	if err != nil || preview.Approval == "" || preview.Command == "" || preview.Signal != "SIGTERM" {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	return preview
}

func TestManualCleanupSignalsOnceAndRejectsReplay(t *testing.T) {
	h, child := recommendHarness(t, true)
	preview := previewRecommendation(t, h, child)

	result, err := h.d.CleanupApply(context.Background(), api.CleanupApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "signalled" || result.Authority != "manual" || h.fake.SignalAttempts != 1 {
		t.Fatalf("apply = %+v, %v; signals=%d", result, err, h.fake.SignalAttempts)
	}
	replay, err := h.d.CleanupApply(context.Background(), api.CleanupApplyRequest{Approval: preview.Approval})
	if err != nil || replay.Result != "rejected" || h.fake.SignalAttempts != 1 {
		t.Fatalf("replay = %+v, %v; signals=%d", replay, err, h.fake.SignalAttempts)
	}
	actions, err := h.store.ListActions(context.Background(), storage.ActionFilter{})
	if err != nil || len(actions) != 2 || actions[1].Result != "signalled" || actions[0].Result != "rejected" {
		t.Fatalf("actions = %+v, %v", actions, err)
	}
	metrics, err := h.d.Metrics(context.Background())
	if err != nil || metrics.ActionsAttempted != 1 || metrics.ActionsRejected != 1 || metrics.ActionsCompleted != 1 {
		t.Fatalf("action metrics = %+v, %v", metrics, err)
	}
	logs, err := h.store.ListAudit(context.Background(), storage.AuditFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range logs {
		if strings.Contains(entry.Summary, preview.Approval) || strings.Contains(entry.EvidenceJSON, preview.Approval) {
			t.Fatal("approval bearer token was persisted in the audit log")
		}
	}
}

func TestManualCleanupRejectsChangedExactIdentity(t *testing.T) {
	h, child := recommendHarness(t, false)
	preview := previewRecommendation(t, h, child)
	result, err := h.d.CleanupApply(context.Background(), api.CleanupApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "rejected" || h.fake.SignalAttempts != 0 {
		t.Fatalf("changed identity apply = %+v, %v; signals=%d", result, err, h.fake.SignalAttempts)
	}
	if result.Reason == "" {
		t.Fatal("rejection lacks a reason")
	}
}

func TestManualCleanupRejectsChangedExecutableAtSameExactKey(t *testing.T) {
	h, child := recommendHarnessWithFinalPath(t, true, "/another/location/safe-helper")
	preview := previewRecommendation(t, h, child)

	result, err := h.d.CleanupApply(context.Background(), api.CleanupApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "rejected" || h.fake.SignalAttempts != 0 {
		t.Fatalf("changed executable apply = %+v, %v; signals=%d", result, err, h.fake.SignalAttempts)
	}
	if !strings.Contains(result.Reason, "executable identity changed") {
		t.Fatalf("rejection reason = %q", result.Reason)
	}
}

func TestManualCleanupRoundTripsOverOwnerOnlySocket(t *testing.T) {
	h, child := recommendHarness(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	server := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := server.Listen(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	client := api.NewClient(h.paths.Socket)
	preview, err := client.CleanupPreview(ctx, api.CleanupPreviewRequest{
		PolicyID: "safe-helper", ProcUID: child.Key().UID(),
	})
	if err != nil || preview.Approval == "" {
		t.Fatalf("socket preview = %+v, %v", preview, err)
	}
	result, err := client.CleanupApply(ctx, api.CleanupApplyRequest{Approval: preview.Approval})
	if err != nil || result.Result != "signalled" {
		t.Fatalf("socket apply = %+v, %v", result, err)
	}
	actions, err := client.Actions(ctx, api.ActionOptions{ProcUID: child.Key().UID()})
	if err != nil || len(actions.Actions) != 1 || actions.Actions[0].Authority != "manual" || len(actions.Actions[0].Evidence) == 0 {
		t.Fatalf("socket actions = %+v, %v", actions, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
