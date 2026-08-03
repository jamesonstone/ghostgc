package sessions

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// The central behaviour of the whole daemon: when the root exits and the
// operating system reparents the survivors to init, the live process tree can
// no longer prove ownership — but the recorded ownership stands.
func TestOwnershipSurvivesReparenting(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()

	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/usr/local/bin/chrome-headless-shell", 2*time.Second)

	s1, tree1 := snap(time.Minute, init, root, child)
	res1, err := r.Reconcile(ctx, s1, tree1, true)
	if err != nil {
		t.Fatal(err)
	}
	r.Commit(res1)
	sessionID := res1.Sessions[0].SessionID

	// The Codex session exits. The child is reparented to init.
	orphan := child
	orphan.PPID = 1
	s2, tree2 := snap(2*time.Minute, init, orphan)
	res2, err := r.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}

	attr, ok := res2.Attributions[child.Key().UID()]
	if !ok {
		t.Fatal("the reparented process must still be attributed; losing a parent link is not a change of ownership")
	}
	if attr.SessionID != sessionID {
		t.Fatalf("session = %q, want the originally recorded %q", attr.SessionID, sessionID)
	}
	if attr.Relation != adapters.RelationRecorded {
		t.Fatalf("relation = %q, want %q", attr.Relation, adapters.RelationRecorded)
	}
	if attr.LinkState != process.LinkReparented {
		t.Fatalf("parent link = %q, want reparented", attr.LinkState)
	}

	if len(res2.Ended) != 1 || res2.Ended[0].SessionID != sessionID {
		t.Fatalf("the session should have ended: %+v", res2.Ended)
	}
	if res2.Ended[0].State != StateCompleted {
		t.Fatalf("ended state = %q, want completed", res2.Ended[0].State)
	}

	// Detached is not orphaned: nothing in this result claims the process is
	// disposable, only that its session finished.
	if attr.Confidence < adapters.ConfidenceAttributable {
		t.Fatalf("recorded ownership lost confidence: %.2f", attr.Confidence)
	}

	var retained bool
	for _, a := range res2.Audit {
		if a.Kind == AuditOwnershipRetained {
			retained = true
		}
	}
	if !retained {
		t.Fatalf("the transition to recorded ownership must be audited: %+v", res2.Audit)
	}
}

// A session must not be kept alive by a recycled PID, and a recycled PID must
// not inherit the previous process's session.
func TestRecycledRootPIDDoesNotResurrectASession(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()

	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)

	s1, tree1 := snap(time.Minute, init, root)
	res1, _ := r.Reconcile(ctx, s1, tree1, true)
	r.Commit(res1)
	firstSession := res1.Sessions[0].SessionID

	// pid 100 is reused by something unrelated, started much later.
	imposter := mkProc(100, 1, "/usr/bin/tail", time.Hour)
	s2, tree2 := snap(2*time.Hour, init, imposter)
	res2, err := r.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}
	r.Commit(res2)

	if len(res2.Ended) != 1 || res2.Ended[0].SessionID != firstSession {
		t.Fatalf("the original session must end even though its pid is in use: %+v", res2.Ended)
	}
	if _, ok := res2.Attributions[imposter.Key().UID()]; ok {
		t.Fatal("a process that merely reuses a pid must not inherit the old session")
	}
}

func TestSessionIdentityChangesWhenTheRootIsANewProcess(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)

	s1, tree1 := snap(time.Minute, init, codexRoot(100, 1, time.Second))
	res1, _ := r.Reconcile(ctx, s1, tree1, true)
	r.Commit(res1)

	// Same pid, different start time: a new Codex run, so a new session.
	s2, tree2 := snap(2*time.Hour, init, codexRoot(100, 1, time.Hour))
	res2, _ := r.Reconcile(ctx, s2, tree2, true)

	if res1.Sessions[0].SessionID == res2.Sessions[0].SessionID {
		t.Fatal("a new root process must produce a new session identifier")
	}
}

// Cross-cycle state must only advance after the caller has persisted the
// result, otherwise a failed write silently suppresses the audit entry for a
// change that was never recorded.
func TestStateOnlyAdvancesOnCommit(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)

	s, tree := snap(time.Minute, init, root)
	first, err := r.Reconcile(ctx, s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(first.Audit, AuditSessionStarted) != 1 {
		t.Fatalf("expected a session.started entry: %+v", first.Audit)
	}
	if len(r.Ownership()) != 0 {
		t.Fatal("ownership must not be advanced before the result is persisted")
	}

	// The write failed; reconcile again. The audit entry must be produced
	// again because it was never recorded.
	second, err := r.Reconcile(ctx, s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(second.Audit, AuditSessionStarted) != 1 {
		t.Fatalf("a suppressed-then-lost audit entry must be re-emitted: %+v", second.Audit)
	}

	r.Commit(second)
	third, err := r.Reconcile(ctx, s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(third.Audit, AuditSessionStarted) != 0 {
		t.Fatalf("an unchanged session must not re-emit session.started: %+v", third.Audit)
	}
	if len(r.Ownership()) == 0 {
		t.Fatal("ownership should be recorded after Commit")
	}
}

func TestSeedRestoresOwnershipAcrossRestart(t *testing.T) {
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/usr/local/bin/chrome-headless-shell", 2*time.Second)

	first := newReconciler()
	s1, tree1 := snap(time.Minute, init, root, child)
	res1, _ := first.Reconcile(ctx, s1, tree1, true)
	first.Commit(res1)

	// A new daemon process starts and reloads state from storage.
	restarted := newReconciler()
	restarted.Seed(res1.Sessions, ownershipMap(res1.Ownership))

	orphan := child
	orphan.PPID = 1
	s2, tree2 := snap(2*time.Minute, init, orphan)
	res2, err := restarted.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}
	attr, ok := res2.Attributions[child.Key().UID()]
	if !ok || attr.Relation != adapters.RelationRecorded {
		t.Fatalf("ownership must survive a daemon restart, got %+v", attr)
	}
}

func TestOriginalParentIsUnknownWhenItWasNeverObserved(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRootWithSession(100, 1, time.Second, "abc")

	// An already-reparented process carrying the session identifier: ghostgc
	// never saw whatever created it.
	orphan := mkProc(400, 1, "/usr/local/bin/chrome-headless-shell", 2*time.Second)
	orphan.Env = map[string]string{"CODEX_SESSION_ID": "abc"}

	s1, tree1 := snap(time.Minute, init, root)
	res1, _ := r.Reconcile(context.Background(), s1, tree1, true)
	r.Commit(res1)

	s2, tree2 := snap(2*time.Minute, init, root, orphan)
	res2, err := r.Reconcile(context.Background(), s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}

	attr := res2.Attributions[orphan.Key().UID()]
	if attr.OriginalParentObserved {
		t.Fatal("the creator was never seen, so it must not be reported as observed")
	}
	for _, rec := range res2.Processes {
		if rec.PID != 400 {
			continue
		}
		if rec.OriginalParentObserved {
			t.Fatal("the stored row must also record that the creator was never observed")
		}
	}
	if hasRelationshipFrom(res2.Relationships, string(RelOriginalParent), orphan.Key().UID()) {
		t.Fatal("no original-parent edge may be claimed for a process whose creator was never seen")
	}
}

func TestOriginalParentIsRecordedWhenItWasObserved(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/bin/sh", 2*time.Second)

	s, tree := snap(time.Minute, init, root, child)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	attr := res.Attributions[child.Key().UID()]
	if !attr.OriginalParentObserved || attr.OriginalPPID != 100 {
		t.Fatalf("creator = %d observed=%t, want pid 100 observed", attr.OriginalPPID, attr.OriginalParentObserved)
	}
	if !hasRelationshipFrom(res.Relationships, string(RelOriginalParent), child.Key().UID()) {
		t.Fatal("an original-parent edge must be recorded when the creator was seen")
	}
	if !hasRelationshipFrom(res.Relationships, string(RelParentChild), child.Key().UID()) {
		t.Fatal("a live parent-child edge must be recorded")
	}
}
