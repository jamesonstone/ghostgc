package process

import (
	"testing"
	"time"
)

var base = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

func proc(pid, ppid int, offset time.Duration) Process {
	return Process{PID: pid, PPID: ppid, StartTime: base.Add(offset), Detailed: true}
}

func snapshot(procs ...Process) *Snapshot {
	return NewSnapshot(base.Add(time.Hour), procs, len(procs))
}

func TestBuildTreeLinksChildToPlausibleParent(t *testing.T) {
	s := snapshot(
		proc(1, 0, 0),
		proc(100, 1, time.Second),
		proc(200, 100, 2*time.Second),
		proc(300, 200, 3*time.Second),
	)
	tree := BuildTree(s)

	if got := tree.Link(200); got != LinkIntact {
		t.Fatalf("link(200) = %q, want %q", got, LinkIntact)
	}
	if !tree.IsDescendantOf(300, 100) {
		t.Fatal("300 should be a descendant of 100")
	}
	if got := tree.Descendants(100); len(got) != 2 {
		t.Fatalf("descendants(100) = %v, want two entries", got)
	}
}

// A recycled PID must never be believed as a parent. This is the primary
// PID-reuse defence: the "parent" started after the child, which is impossible.
func TestBuildTreeRefusesParentYoungerThanChild(t *testing.T) {
	child := proc(200, 100, time.Second)
	// pid 100 exited and the PID was reused by a process started later.
	recycled := proc(100, 1, 10*time.Second)
	tree := BuildTree(snapshot(proc(1, 0, 0), recycled, child))

	if got := tree.Link(200); got != LinkImpossible {
		t.Fatalf("link(200) = %q, want %q", got, LinkImpossible)
	}
	if tree.IsDescendantOf(200, 100) {
		t.Fatal("a process must not be treated as the child of a younger process")
	}
	if got := tree.Descendants(100); len(got) != 0 {
		t.Fatalf("descendants(100) = %v, want none", got)
	}
}

func TestBuildTreeMarksReparentedProcesses(t *testing.T) {
	tree := BuildTree(snapshot(proc(1, 0, 0), proc(500, 1, time.Second)))

	if got := tree.Link(500); got != LinkReparented {
		t.Fatalf("link(500) = %q, want %q", got, LinkReparented)
	}
	// Reparented is not orphaned, but it does mean the live tree can no longer
	// attribute the process; ancestry must not silently walk through init.
	if got := tree.Ancestors(500); len(got) != 0 {
		t.Fatalf("ancestors(500) = %v, want none", got)
	}
}

func TestBuildTreeMissingParent(t *testing.T) {
	tree := BuildTree(snapshot(proc(1, 0, 0), proc(700, 650, time.Second)))
	if got := tree.Link(700); got != LinkMissing {
		t.Fatalf("link(700) = %q, want %q", got, LinkMissing)
	}
}

func TestBuildTreeRefusesSelfParent(t *testing.T) {
	tree := BuildTree(snapshot(proc(1, 0, 0), proc(800, 800, time.Second)))
	if got := tree.Link(800); got != LinkImpossible {
		t.Fatalf("link(800) = %q, want %q", got, LinkImpossible)
	}
	if got := tree.Descendants(800); len(got) != 0 {
		t.Fatalf("descendants(800) = %v, want none", got)
	}
}

// A cycle can only appear through inconsistent data, but the walk must
// terminate if it does.
func TestTreeWalksTerminateOnCycle(t *testing.T) {
	a := Process{PID: 10, PPID: 11, StartTime: base}
	b := Process{PID: 11, PPID: 10, StartTime: base}
	tree := BuildTree(snapshot(a, b))

	done := make(chan struct{})
	go func() {
		tree.Descendants(10)
		tree.Ancestors(10)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tree walk did not terminate on a cyclic parent relationship")
	}
}

func TestDeepChainIsBounded(t *testing.T) {
	var procs []Process
	procs = append(procs, proc(1, 0, 0))
	for i := 2; i < maxTreeDepth+50; i++ {
		procs = append(procs, proc(i, i-1, time.Duration(i)*time.Millisecond))
	}
	tree := BuildTree(snapshot(procs...))

	if got := len(tree.Ancestors(maxTreeDepth + 49)); got > maxTreeDepth {
		t.Fatalf("ancestors walked %d levels, want at most %d", got, maxTreeDepth)
	}
	if got := len(tree.Descendants(2)); got > maxTreeDepth*maxTreeDepth {
		t.Fatalf("descendant walk was not bounded: %d", got)
	}
}

func TestSnapshotByKeyRejectsPIDReuse(t *testing.T) {
	p := proc(4242, 1, time.Second)
	s := snapshot(p)

	if _, ok := s.ByKey(p.Key()); !ok {
		t.Fatal("lookup by the exact key should succeed")
	}
	stale := Key{PID: 4242, StartTimeNs: base.Add(-time.Hour).UnixNano()}
	if _, ok := s.ByKey(stale); ok {
		t.Fatal("a key with a different start time must not match; that is a different process")
	}
}

func TestKeyRoundTrip(t *testing.T) {
	k := NewKey(1234, base)
	parsed, err := ParseKey(k.UID())
	if err != nil {
		t.Fatalf("ParseKey: %v", err)
	}
	if parsed != k {
		t.Fatalf("round trip: got %v, want %v", parsed, k)
	}
	if _, err := ParseKey("not-a-key"); err == nil {
		t.Fatal("ParseKey should reject malformed input")
	}
}

func TestNameFallsBackToComm(t *testing.T) {
	p := Process{Comm: "trunc", ExecPath: ""}
	if got := p.Name(); got != "trunc" {
		t.Fatalf("Name() = %q, want the kernel comm", got)
	}
	p.ExecPath = "/usr/local/bin/codex"
	if got := p.Name(); got != "codex" {
		t.Fatalf("Name() = %q, want the executable basename", got)
	}
}
