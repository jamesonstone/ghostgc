package daemon_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

// The end-to-end shape from the specification: a fake agent session with a
// child, the session ends, the child survives, and nothing is ever signalled.
func TestObservationLifecycle(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mk(200, 100, "/usr/local/bin/chrome-headless-shell", 2*time.Second)

	h := newHarness(t,
		snapshot(time.Minute, init, root, child),
		snapshot(5*time.Minute, init, withParent(child, 1)),
	)

	h.d.ScanNow(ctx)

	sessionsResp, err := h.d.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionsResp.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessionsResp.Sessions))
	}
	sess := sessionsResp.Sessions[0]
	if sess.State != "active" {
		t.Fatalf("session state = %q, want active", sess.State)
	}
	if sess.AgentID != "codex" {
		t.Fatalf("agent = %q, want codex", sess.AgentID)
	}
	if sess.Processes != 2 {
		t.Fatalf("session has %d processes, want the root and its child", sess.Processes)
	}

	procs, err := h.d.Processes(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(procs.Processes) != 2 {
		t.Fatalf("got %d attributed processes, want 2", len(procs.Processes))
	}

	// Second cycle: the session root is gone and the child was reparented.
	h.d.ScanNow(ctx)

	sessionsResp, err = h.d.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionsResp.Sessions[0].State; got != "completed" {
		t.Fatalf("session state = %q, want completed once the root exited", got)
	}

	explain, err := h.d.Explain(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}
	if explain.SessionID != sess.SessionID {
		t.Fatalf("the surviving child lost its session: %q", explain.SessionID)
	}
	if explain.Relation != "recorded" {
		t.Fatalf("relation = %q, want recorded ownership after reparenting", explain.Relation)
	}
	if explain.ParentLink != "reparented" {
		t.Fatalf("parent link = %q, want reparented", explain.ParentLink)
	}
	if !explain.Protection.Protected {
		t.Fatal("a detached process must still be protected; detached does not mean orphaned")
	}
	if len(explain.Evidence) == 0 {
		t.Fatal("every conclusion must carry evidence")
	}
	if !explain.Detached {
		t.Fatal("losing an observed original parent must be reported as detached")
	}
	foundParentLoss := false
	for _, evidence := range explain.ActivityEvidence {
		foundParentLoss = foundParentLoss || evidence.Rule == "observed-parent-loss-v1"
	}
	if !foundParentLoss {
		t.Fatalf("detachment lacks parent-loss evidence: %+v", explain.ActivityEvidence)
	}

	// The root exited, so the process row must be marked as such rather than
	// silently disappearing.
	rootRow, err := h.store.GetProcess(ctx, root.Key().UID())
	if err != nil {
		t.Fatal(err)
	}
	if rootRow.ExitedAtNs == nil {
		t.Fatal("the exited root must be recorded as exited")
	}

	if h.fake.SignalAttempts != 0 {
		t.Fatalf("observation attempted to signal a process %d time(s); this build must never try",
			h.fake.SignalAttempts)
	}
}

func TestExplainRefusesToGuessAboutUnknownProcesses(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))
	h.d.ScanNow(ctx)

	explain, err := h.d.Explain(ctx, 999999)
	if err != nil {
		t.Fatal(err)
	}
	if explain.Found {
		t.Fatal("a pid that is not in the snapshot must not be reported as found")
	}
	if explain.Classification != "unknown" {
		t.Fatalf("classification = %q, want unknown", explain.Classification)
	}
	if !explain.Protection.Protected {
		t.Fatal("unknown must be protected")
	}
}

func TestUnattributedProcessIsExplainedAsProtected(t *testing.T) {
	ctx := context.Background()
	vim := mk(4000, 1, "/usr/bin/vim", time.Second)
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0), vim))
	h.d.ScanNow(ctx)

	explain, err := h.d.Explain(ctx, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if explain.SessionID != "" {
		t.Fatalf("an unrelated editor was attributed to session %q", explain.SessionID)
	}
	if !explain.Protection.Protected {
		t.Fatal("an unattributed process must be protected")
	}
	if explain.Message == "" {
		t.Fatal("the user must be told why the process was ignored")
	}
}

func TestCleanupSurfacesAreEmptyAndSayWhy(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))

	candidates, err := h.d.Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.Enforceable) != 0 || len(candidates.Audited) != 0 {
		t.Fatal("this build must report no cleanup candidates of any kind")
	}
	if !strings.Contains(strings.ToLower(candidates.Note), "phase") {
		t.Fatalf("the empty result must explain itself: %q", candidates.Note)
	}

	policies, err := h.d.Policies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(policies.Policies) != 0 {
		t.Fatal("no policies can exist in this build")
	}
	if policies.GlobalMode != "audit" {
		t.Fatalf("global mode = %q, want audit", policies.GlobalMode)
	}
}

func TestStatusReportsSignallingDisabled(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))
	h.d.ScanNow(ctx)

	status, err := h.d.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.SignallingEnabled {
		t.Fatal("status must report that signalling is disabled")
	}
	if status.Mode != "audit" {
		t.Fatalf("mode = %q, want audit", status.Mode)
	}
	if status.LastScan == nil {
		t.Fatal("status must report the last scan")
	}
	if status.LastScan.VisibleProcesses == 0 {
		t.Fatal("the scan summary must include how many processes were visible")
	}
}

func TestDoctorProvesSignalSafetyGate(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))
	h.d.ScanNow(ctx)

	doc, err := h.d.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range doc.Checks {
		if c.Name != "signal-safety-gate" {
			continue
		}
		found = true
		if c.Status != api.CheckOK {
			t.Fatalf("signal-safety-gate check = %q: %s", c.Status, c.Detail)
		}
	}
	if !found {
		t.Fatal("doctor must verify the runtime signal safety gate")
	}
	if h.fake.SignalAttempts == 0 {
		t.Fatal("the doctor check should actually exercise the safety gate, not read a constant")
	}
}

func TestFailedScanIsRecordedAndObservationContinues(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0), codexRoot(100, 1, time.Second)))

	h.fake.Err = errUnavailable{}
	h.d.ScanNow(ctx)

	status, err := h.d.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Health != api.HealthDegraded {
		t.Fatalf("health = %q, want degraded after a failed scan", status.Health)
	}

	logs, err := h.d.Logs(ctx, api.LogOptions{Kind: "scan.failed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs.Entries) != 1 {
		t.Fatalf("got %d scan.failed entries, want 1", len(logs.Entries))
	}
	if !strings.Contains(logs.Entries[0].Summary, "no conclusion") {
		t.Fatalf("a failed scan must state that nothing was concluded: %q", logs.Entries[0].Summary)
	}

	// The next cycle succeeds and the daemon recovers on its own.
	h.d.ScanNow(ctx)
	status, err = h.d.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Health != api.HealthHealthy {
		t.Fatalf("health = %q, want healthy after a successful scan", status.Health)
	}
	if len(status.SessionsByState) == 0 {
		t.Fatal("observation should have resumed and found the session")
	}
}

func (errUnavailable) Error() string { return "process table temporarily unavailable" }
