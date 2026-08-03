package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/storage"
)

func TestSessionStartsInStartingAndBecomesActive(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)

	// First observation, four seconds after the root started.
	s1, tree1 := snap(5*time.Second, init, root)
	res1, err := r.Reconcile(ctx, s1, tree1, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := res1.Sessions[0].State; got != string(StateStarting) {
		t.Fatalf("state = %q, want starting: the session has not yet had a chance to do anything", got)
	}
	r.Commit(res1)

	// A minute later it is unambiguously running.
	s2, tree2 := snap(time.Minute, init, root)
	res2, err := r.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := res2.Sessions[0].State; got != string(StateActive) {
		t.Fatalf("state = %q, want active", got)
	}
	if res2.Sessions[0].PreviousState != string(StateStarting) {
		t.Fatalf("previous_state = %q, want starting", res2.Sessions[0].PreviousState)
	}
	if res2.Sessions[0].StateChangedNs == 0 {
		t.Fatal("a state change must record when it happened")
	}
	if countKind(res2.Audit, AuditSessionState) != 1 {
		t.Fatalf("the transition must be audited: %+v", res2.Audit)
	}
	r.Commit(res2)

	// A third cycle with nothing changed must not re-announce the state.
	s3, tree3 := snap(2*time.Minute, init, root)
	res3, _ := r.Reconcile(ctx, s3, tree3, true)
	if countKind(res3.Audit, AuditSessionState) != 0 {
		t.Fatalf("an unchanged state must not be re-announced: %+v", res3.Audit)
	}
	if res3.Sessions[0].StateChangedNs != 0 {
		t.Fatal("state_changed_ns must only be set on a cycle that changed the state")
	}
}

// A session observed for the first time long after it started never passes
// through starting: claiming it did would be inventing a transition nobody saw.
func TestLongRunningSessionIsActiveOnFirstObservation(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, 0)

	s, tree := snap(4*time.Hour, init, root)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Sessions[0].State; got != string(StateActive) {
		t.Fatalf("state = %q, want active", got)
	}
}

func TestEveryStateTransitionCarriesEvidence(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)

	s1, tree1 := snap(5*time.Second, init, root)
	res1, _ := r.Reconcile(ctx, s1, tree1, true)
	r.Commit(res1)
	s2, tree2 := snap(time.Minute, init, root)
	res2, _ := r.Reconcile(ctx, s2, tree2, true)
	r.Commit(res2)
	s3, tree3 := snap(2*time.Minute, init)
	res3, _ := r.Reconcile(ctx, s3, tree3, true)

	for _, batch := range [][]storage.AuditRecord{res1.Audit, res2.Audit, res3.Audit} {
		for _, entry := range batch {
			switch entry.Kind {
			case AuditSessionStarted, AuditSessionState, AuditSessionEnded:
			default:
				continue
			}
			if entry.EvidenceJSON == "" || entry.EvidenceJSON == "[]" || entry.EvidenceJSON == "null" {
				t.Fatalf("state transition %q for %s carries no evidence", entry.Kind, entry.Subject)
			}
			if !strings.Contains(entry.EvidenceJSON, "session state") {
				t.Fatalf("transition evidence should say what changed: %s", entry.EvidenceJSON)
			}
		}
	}
}

// Six Codex servers started by six editor windows and six left behind by a
// crashed script look identical without this.
func TestLaunchContextRecordsWhoStartedTheSession(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	editor := mkProc(500, 1, "/Applications/Editor.app/Contents/MacOS/Editor Helper (Plugin)", time.Second)
	root := codexRoot(100, 500, 2*time.Second)

	s, tree := snap(time.Minute, init, editor, root)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}

	sess := res.Sessions[0]
	if sess.HostPID != 500 {
		t.Fatalf("host pid = %d, want the launching editor helper 500", sess.HostPID)
	}
	if sess.HostName != "Editor Helper (Plugin)" {
		t.Fatalf("host name = %q", sess.HostName)
	}
	launch := res.Launch[sess.SessionID]
	if !launch.Observed {
		t.Fatal("the launcher was present, so it must be reported as observed")
	}
	if !hasRelationship(res.Relationships, string(RelLaunch)) {
		t.Fatalf("a launch edge must be recorded: %+v", res.Relationships)
	}
}

// A root that was already reparented when ghostgc first looked has no
// discoverable launcher, and the daemon must say so rather than name init.
func TestLaunchContextIsUnknownWhenTheRootWasAlreadyReparented(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)

	s, tree := snap(time.Minute, init, root)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	sess := res.Sessions[0]
	if sess.HostPID != 0 || sess.HostName != "" {
		t.Fatalf("a reparented root has no known launcher, got pid %d name %q", sess.HostPID, sess.HostName)
	}
	if res.Launch[sess.SessionID].Observed {
		t.Fatal("the launcher must be reported as unobserved")
	}
	if !strings.Contains(res.Launch[sess.SessionID].Describe(), "unknown") {
		t.Fatalf("the description must say unknown, got %q", res.Launch[sess.SessionID].Describe())
	}
}
