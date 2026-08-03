package sessions

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// codexRootWithSession is a root that exposes the agent's own session
// identifier, which is what makes environment-derived membership resolvable.
func codexRootWithSession(pid, ppid int, offset time.Duration, native string) process.Process {
	p := codexRoot(pid, ppid, offset)
	p.Env["CODEX_SESSION_ID"] = native
	return p
}

// A POSIX session leader is usually the user's shell, so everything that shell
// ever ran shares the identifier. Sharing a terminal is context, never
// ownership.
func TestTerminalAndProcessGroupEdgesAreContextNotOwnership(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	root.SID = 50
	root.PGID = 50
	child := mkProc(200, 100, "/bin/sh", 2*time.Second)
	child.SID = 50
	child.PGID = 50

	s, tree := snap(time.Minute, init, root, child)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}

	if !hasRelationship(res.Relationships, string(RelTerminal)) {
		t.Fatalf("a shared POSIX session must be recorded as an edge: %+v", res.Relationships)
	}
	if !hasRelationship(res.Relationships, string(RelProcessGroup)) {
		t.Fatal("a shared process group must be recorded as an edge")
	}
	if AttributingKinds[RelTerminal] {
		t.Fatal("a terminal edge must never establish ownership")
	}
	if AttributingKinds[RelProcessGroup] {
		t.Fatal("a process-group edge must never establish ownership")
	}
	if AttributingKinds[RelRepository] {
		t.Fatal("a repository edge must never establish ownership: a popular directory is not a session")
	}

	// The child here is attributed by ancestry, so prove the terminal edge is
	// not what did it: a process sharing only the terminal stays unattributed.
	sibling := mkProc(300, 1, "/usr/bin/vim", 3*time.Second)
	sibling.SID = 50
	sibling.PGID = 50
	s2, tree2 := snap(2*time.Minute, init, root, sibling)
	res2, err := r.Reconcile(context.Background(), s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res2.Attributions[sibling.Key().UID()]; ok {
		t.Fatal("a process that only shares the terminal must not be attributed to the session")
	}
}

func TestReparentingIsRecordedAsAnEdge(t *testing.T) {
	r := newReconciler()
	ctx := context.Background()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/usr/local/bin/chrome-headless-shell", 2*time.Second)

	s1, tree1 := snap(time.Minute, init, root, child)
	res1, _ := r.Reconcile(ctx, s1, tree1, true)
	r.Commit(res1)

	orphan := child
	orphan.PPID = 1
	s2, tree2 := snap(2*time.Minute, init, orphan)
	res2, err := r.Reconcile(ctx, s2, tree2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRelationshipFrom(res2.Relationships, string(RelReparented), child.Key().UID()) {
		t.Fatalf("the reparenting must be recorded as an edge: %+v", res2.Relationships)
	}
	// The original-parent edge recorded before the reparenting must survive it.
	if !hasRelationshipFrom(res2.Relationships, string(RelOriginalParent), child.Key().UID()) {
		t.Fatal("the original-parent edge must survive reparenting")
	}
}

func TestRelationshipsAreScopedToTheirSession(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/bin/sh", 2*time.Second)

	s, tree := snap(time.Minute, init, root, child)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := res.Sessions[0].SessionID
	for _, rel := range res.Relationships {
		if rel.SessionID != sessionID {
			t.Fatalf("edge %q belongs to session %q, want %q", rel.Kind, rel.SessionID, sessionID)
		}
		if rel.FromProcUID == "" {
			t.Fatalf("edge %q has no source process", rel.Kind)
		}
		if rel.Detail == "" {
			t.Fatalf("edge %q has no detail; an edge without a reason is not evidence", rel.Kind)
		}
	}
}

func hasRelationship(rels []storage.RelationshipRecord, kind string) bool {
	for _, rel := range rels {
		if rel.Kind == kind {
			return true
		}
	}
	return false
}

func hasRelationshipFrom(rels []storage.RelationshipRecord, kind, from string) bool {
	for _, rel := range rels {
		if rel.Kind == kind && rel.FromProcUID == from {
			return true
		}
	}
	return false
}
