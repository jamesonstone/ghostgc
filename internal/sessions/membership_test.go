package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
)

// The process-compose case from the phase 1 field test, resolved: a process
// that inherited a Codex session identifier is a member of that session's
// lineage, and now says so.
func TestEnvironmentMembershipAttributesToTheOwningSession(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRootWithSession(100, 1, time.Second, "019fae08-329a-7bb1-946c-a90e9908c2ae")

	s1, tree1 := snap(time.Minute, init, root)
	res1, err := r.Reconcile(ctx, s1, tree1, true)
	if err != nil {
		t.Fatal(err)
	}
	r.Commit(res1)
	sessionID := res1.Sessions[0].SessionID
	if res1.Sessions[0].NativeSessionID == "" {
		t.Fatal("the agent's own session identifier must be recorded on the session")
	}

	// The Codex session ends. A service manager it started days ago is still
	// running, reparented to init, carrying the inherited identifier.
	svc := mkProc(900, 1, "/opt/homebrew/bin/process-compose", 2*time.Second)
	svc.Env = map[string]string{"CODEX_THREAD_ID": "019fae08-329a-7bb1-946c-a90e9908c2ae"}

	s2, tree2 := snap(2*time.Minute, init, svc)
	res2, err := r.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}

	attr, ok := res2.Attributions[svc.Key().UID()]
	if !ok {
		t.Fatal("a process carrying the session's own identifier should be attributed to it")
	}
	if attr.SessionID != sessionID {
		t.Fatalf("session = %q, want %q", attr.SessionID, sessionID)
	}
	if attr.Relation != adapters.RelationEnvironment {
		t.Fatalf("relation = %q, want environment", attr.Relation)
	}

	// The cap is the safety property: an inherited variable must never make a
	// process eligible for automated action.
	if attr.Confidence != adapters.ConfidenceEnvironmentMembership {
		t.Fatalf("confidence = %.2f, want the environment-membership ceiling %.2f",
			attr.Confidence, adapters.ConfidenceEnvironmentMembership)
	}
	if attr.Confidence >= adapters.ConfidencePolicyEligible {
		t.Fatalf("confidence %.2f reaches the policy-eligible threshold; an inherited environment variable must never do that",
			attr.Confidence)
	}
	if !hasRelationship(res2.Relationships, string(RelEnvironment)) {
		t.Fatal("an environment edge must be recorded")
	}
}

func TestEnvironmentMembershipRequiresAKnownSession(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	stranger := mkProc(900, 1, "/opt/homebrew/bin/process-compose", time.Second)
	stranger.Env = map[string]string{"CODEX_THREAD_ID": "a-session-ghostgc-never-saw"}

	s, tree := snap(time.Minute, init, stranger)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Attributions[stranger.Key().UID()]; ok {
		t.Fatal("an identifier naming no known session must not attribute anything")
	}
}

// An agent that exposes its session identifier passes it to every helper it
// starts. A helper built from the same executable is detected on its own
// identity evidence too, so two processes end up naming one session. A session
// has exactly one root; the earliest claimant wins and the rest are members.
func TestOnlyTheEarliestClaimantBecomesTheSessionRoot(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRootWithSession(100, 1, time.Second, "shared-session")
	// A detached helper built from the same binary, started later, carrying
	// the same identifier, and not a descendant of the root.
	helper := codexRootWithSession(300, 1, 10*time.Second, "shared-session")

	s, tree := snap(time.Minute, init, root, helper)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Sessions) != 1 {
		t.Fatalf("got %d session records for one session identifier: %+v", len(res.Sessions), res.Sessions)
	}
	if res.Sessions[0].RootPID != 100 {
		t.Fatalf("root pid = %d, want the earliest claimant 100", res.Sessions[0].RootPID)
	}

	rootAttr := res.Attributions[root.Key().UID()]
	if rootAttr.Relation != adapters.RelationRoot {
		t.Fatalf("the earliest claimant should be the root, got %q", rootAttr.Relation)
	}
	helperAttr := res.Attributions[helper.Key().UID()]
	if helperAttr.Relation == adapters.RelationRoot {
		t.Fatal("a second process must not also be recorded as the session root")
	}
	if helperAttr.SessionID != rootAttr.SessionID {
		t.Fatalf("the helper belongs to session %q, want %q", helperAttr.SessionID, rootAttr.SessionID)
	}

	var explained bool
	for _, ev := range helperAttr.Evidence {
		if strings.Contains(ev.Detail, "started earlier and is the session root") {
			explained = true
		}
	}
	if !explained {
		t.Fatalf("the demotion must be explained in the evidence: %+v", helperAttr.Evidence)
	}
}

// Order of discovery must not decide which process is the root; start time must.
func TestRootChoiceIsIndependentOfScanOrder(t *testing.T) {
	init := mkProc(1, 0, "/sbin/launchd", 0)
	early := codexRootWithSession(900, 1, time.Second, "shared")
	late := codexRootWithSession(100, 1, 10*time.Second, "shared")

	// pid 100 sorts first but started later; the root must still be pid 900.
	r := newReconciler()
	s, tree := snap(time.Minute, init, late, early)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sessions[0].RootPID != 900 {
		t.Fatalf("root pid = %d, want the earliest-started 900 regardless of pid order", res.Sessions[0].RootPID)
	}
}
