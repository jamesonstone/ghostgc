package sessions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/adapters/codex"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const selfUID = 501

var t0 = time.Date(2026, 8, 3, 11, 0, 0, 0, time.UTC)

func mkProc(pid, ppid int, exec string, offset time.Duration) process.Process {
	return process.Process{
		PID: pid, PPID: ppid, PGID: pid, SID: pid, UID: selfUID,
		StartTime: t0.Add(offset), ExecPath: exec, Comm: "proc",
		Args: []string{exec}, CWD: "/repo", Detailed: true,
	}
}

// codexRoot is recognised with high confidence: the executable basename plus
// the environment variables the official launcher sets.
func codexRoot(pid, ppid int, offset time.Duration) process.Process {
	p := mkProc(pid, ppid, "/opt/homebrew/bin/codex", offset)
	p.Env = map[string]string{
		"CODEX_MANAGED_PACKAGE_ROOT": "/opt/homebrew/lib/node_modules/@openai/codex",
		"CODEX_HOME":                 "/Users/dev/.codex",
	}
	return p
}

func snap(at time.Duration, procs ...process.Process) (*process.Snapshot, *process.Tree) {
	s := process.NewSnapshot(t0.Add(at), procs, len(procs)+400)
	return s, process.BuildTree(s)
}

func newReconciler() *Reconciler {
	reg := adapters.NewRegistry(codex.New(nil))
	return New(reg, 99, selfUID, nil)
}

func TestReconcileDetectsSessionAndAttributesDescendants(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mkProc(200, 100, "/bin/sh", 2*time.Second)
	grandchild := mkProc(300, 200, "/usr/bin/rg", 3*time.Second)
	unrelated := mkProc(400, 1, "/usr/bin/vim", 4*time.Second)

	s, tree := snap(time.Minute, init, root, child, grandchild, unrelated)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1: %+v", len(res.Sessions), res.Sessions)
	}
	sess := res.Sessions[0]
	if sess.State != string(StateActive) {
		t.Fatalf("session state = %q, want active", sess.State)
	}
	if sess.RootPID != 100 {
		t.Fatalf("root pid = %d, want 100", sess.RootPID)
	}
	if sess.Confidence < adapters.ConfidencePolicyEligible {
		t.Fatalf("confidence %.2f, want a high-confidence detection for this fixture", sess.Confidence)
	}

	if res.AttributedCount != 3 {
		t.Fatalf("attributed %d processes, want the root plus two descendants", res.AttributedCount)
	}
	for _, uid := range []string{root.Key().UID(), child.Key().UID(), grandchild.Key().UID()} {
		if _, ok := res.Attributions[uid]; !ok {
			t.Fatalf("process %s was not attributed", uid)
		}
	}
	if _, ok := res.Attributions[unrelated.Key().UID()]; ok {
		t.Fatal("an unrelated process must not be attributed to the session")
	}

	if res.Attributions[root.Key().UID()].Relation != adapters.RelationRoot {
		t.Fatal("the root must be recorded as the root")
	}
	if res.Attributions[grandchild.Key().UID()].Relation != adapters.RelationDescendant {
		t.Fatal("a grandchild must be recorded as a descendant")
	}
}

func TestProcessesOwnedByAnotherUserAreNeverAttributed(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	other := mkProc(200, 100, "/bin/sh", 2*time.Second)
	other.UID = 502

	s, tree := snap(time.Minute, init, root, other)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Attributions[other.Key().UID()]; ok {
		t.Fatal("a process owned by another user must never be attributed")
	}
}

func TestUninspectedProcessesAreNeverAttributed(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	blind := mkProc(200, 100, "", 2*time.Second)
	blind.Detailed = false

	s, tree := snap(time.Minute, init, root, blind)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Attributions[blind.Key().UID()]; ok {
		t.Fatal("a process that was never inspected must not be attributed")
	}
}

func TestCommandLinesCanBeWithheld(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	root.Args = []string{"/opt/homebrew/bin/codex", "--api-key=sk-live-secret"}

	s, tree := snap(time.Minute, init, root)
	res, err := r.Reconcile(context.Background(), s, tree, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Processes {
		if p.Cmdline != `["codex"]` {
			t.Fatalf("cmdline = %s, want only the executable name when storage is disabled", p.Cmdline)
		}
	}
}

func TestStoredCommandLinesAreRedacted(t *testing.T) {
	r := newReconciler()
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	root.Args = []string{"/opt/homebrew/bin/codex", "--api-key=sk-live-secret"}

	s, tree := snap(time.Minute, init, root)
	res, err := r.Reconcile(context.Background(), s, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Processes {
		if strings.Contains(p.Cmdline, "sk-live-secret") {
			t.Fatalf("a credential reached the storage record: %s", p.Cmdline)
		}
	}
}

func countKind(entries []storage.AuditRecord, kind string) int {
	n := 0
	for _, e := range entries {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

func ownershipMap(rows []storage.OwnershipRecord) map[string]storage.OwnershipRecord {
	out := make(map[string]storage.OwnershipRecord, len(rows))
	for _, r := range rows {
		out[r.ProcUID] = r
	}
	return out
}

// ---------------------------------------------------------------------------
// Delivery phase 2: the session graph.
// ---------------------------------------------------------------------------
