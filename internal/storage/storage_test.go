package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ghostgc.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func ns(offset time.Duration) int64 {
	return time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC).Add(offset).UnixNano()
}

func proc(uid string, pid int, startNs, seenNs int64) ProcessRecord {
	return ProcessRecord{
		ProcUID: uid, PID: pid, StartTimeNs: startNs, PPID: 1, OriginalPPID: 900,
		PGID: pid, SID: pid, UID: 501, Comm: "helper", ExecPath: "/opt/helper",
		Cmdline: `["helper"]`, CWD: "/tmp", TTY: "",
		AgentID: "codex", SessionID: "sess-1", Relation: "descendant", Confidence: 0.97,
		FirstSeenNs: seenNs, LastSeenNs: seenNs,
	}
}

func session(id string, startNs, seenNs int64) SessionRecord {
	return SessionRecord{
		SessionID: id, AgentID: "codex", RootProcUID: "100:" + itoa(startNs), RootPID: 100,
		State: "active", Confidence: 0.9, WorkingDir: "/repo", RepositoryPath: "/repo",
		StartedNs: startNs, LastSeenNs: seenNs,
	}
}

func itoa(n int64) string {
	return time.Unix(0, n).Format("20060102150405.000000000")
}

func TestProcessUpsertPreservesOriginalParentAndFirstSeen(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	first := proc("4242:1000", 4242, 1000, ns(0))
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.UpsertProcess(first) }); err != nil {
		t.Fatal(err)
	}

	// The process is later reparented to init. The live ppid changes; the
	// original parent must not.
	second := first
	second.PPID = 1
	second.OriginalPPID = 1
	second.LastSeenNs = ns(time.Minute)
	second.FirstSeenNs = ns(time.Minute)
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.UpsertProcess(second) }); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProcess(ctx, "4242:1000")
	if err != nil {
		t.Fatal(err)
	}
	if got.OriginalPPID != 900 {
		t.Fatalf("original_ppid = %d, want the value recorded at first observation (900)", got.OriginalPPID)
	}
	if got.FirstSeenNs != ns(0) {
		t.Fatalf("first_seen_ns = %d, want the original observation time", got.FirstSeenNs)
	}
	if got.LastSeenNs != ns(time.Minute) {
		t.Fatalf("last_seen_ns = %d, want the newest observation time", got.LastSeenNs)
	}
}

// A recycled PID must be a different row. If it shared a row it would inherit
// the previous process's session, evidence and history.
func TestRecycledPIDIsADifferentRow(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	original := proc("4242:1000", 4242, 1000, ns(0))
	recycled := proc("4242:9999", 4242, 9999, ns(time.Hour))
	recycled.SessionID = "sess-2"

	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertProcess(original); err != nil {
			return err
		}
		return tx.UpsertProcess(recycled)
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := s.ListProcesses(ctx, ProcessFilter{PID: 4242})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows for pid 4242, want 2 distinct processes", len(rows))
	}
	a, err := s.GetProcess(ctx, "4242:1000")
	if err != nil {
		t.Fatal(err)
	}
	if a.SessionID != "sess-1" {
		t.Fatalf("the original process's session changed to %q", a.SessionID)
	}
}

func TestMarkExitedBeforeOnlyTouchesUnseenProcesses(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	stale := proc("1:100", 1, 100, ns(0))
	fresh := proc("2:200", 2, 200, ns(time.Minute))
	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertProcess(stale); err != nil {
			return err
		}
		return tx.UpsertProcess(fresh)
	}); err != nil {
		t.Fatal(err)
	}

	var affected int64
	if err := s.WithTx(ctx, func(tx *Tx) error {
		var err error
		affected, err = tx.MarkExitedBefore(ns(time.Minute), ns(time.Minute))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("marked %d processes exited, want 1", affected)
	}

	gone, _ := s.GetProcess(ctx, "1:100")
	if gone.ExitedAtNs == nil {
		t.Fatal("the unseen process should be marked exited")
	}
	alive, _ := s.GetProcess(ctx, "2:200")
	if alive.ExitedAtNs != nil {
		t.Fatal("a process seen in this scan must not be marked exited")
	}
}

func TestOwnershipIsDurableAndNeverDowngraded(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	root := OwnershipRecord{
		SessionID: "sess-1", ProcUID: "100:1", AgentID: "codex", Relation: "root",
		Confidence: 0.97, OriginalPPID: 42, FirstSeenNs: ns(0), LastSeenNs: ns(0),
	}
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.UpsertOwnership(root) }); err != nil {
		t.Fatal(err)
	}

	// A later, weaker observation must not overwrite what was established.
	weaker := root
	weaker.Relation = "descendant"
	weaker.Confidence = 0.4
	weaker.OriginalPPID = 1
	weaker.LastSeenNs = ns(time.Minute)
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.UpsertOwnership(weaker) }); err != nil {
		t.Fatal(err)
	}

	rows, err := s.SessionProcesses(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d ownership rows, want 1", len(rows))
	}
	got := rows[0]
	if got.Relation != "root" {
		t.Fatalf("relation = %q, want it to stay root", got.Relation)
	}
	if got.Confidence != 0.97 {
		t.Fatalf("confidence = %.2f, want the higher recorded value 0.97", got.Confidence)
	}
	if got.OriginalPPID != 42 {
		t.Fatalf("original_ppid = %d, want the first recorded value 42", got.OriginalPPID)
	}
	if got.LastSeenNs != ns(time.Minute) {
		t.Fatal("last_seen_ns should still advance")
	}
}

func TestLiveOwnershipExcludesExitedProcesses(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	live := proc("2:200", 2, 200, ns(0))
	dead := proc("1:100", 1, 100, ns(0))
	err := s.WithTx(ctx, func(tx *Tx) error {
		for _, p := range []ProcessRecord{live, dead} {
			if err := tx.UpsertProcess(p); err != nil {
				return err
			}
			if err := tx.UpsertOwnership(OwnershipRecord{
				SessionID: "sess-1", ProcUID: p.ProcUID, AgentID: "codex",
				Relation: "descendant", Confidence: 0.9, FirstSeenNs: ns(0), LastSeenNs: ns(0),
			}); err != nil {
				return err
			}
		}
		_, err := tx.MarkExitedBefore(ns(time.Minute), ns(time.Minute))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Both were marked exited by the sweep above; re-record only the live one.
	if err := s.WithTx(ctx, func(tx *Tx) error {
		p := live
		p.LastSeenNs = ns(2 * time.Minute)
		return tx.UpsertProcess(p)
	}); err != nil {
		t.Fatal(err)
	}

	own, err := s.LiveOwnership(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := own["2:200"]; !ok {
		t.Fatal("a live process's ownership must be loaded")
	}
	if _, ok := own["1:100"]; ok {
		t.Fatal("an exited process's ownership must not be loaded as live")
	}
}

func TestSessionLookupByPrefixAndAmbiguity(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertSession(session("8f2a1111", ns(0), ns(0))); err != nil {
			return err
		}
		return tx.UpsertSession(session("8f2a2222", ns(0), ns(0)))
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := s.GetSession(ctx, "8f2a"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("an ambiguous prefix must be reported, got %v", err)
	}
	got, err := s.GetSession(ctx, "8f2a11")
	if err != nil {
		t.Fatalf("an unambiguous prefix must resolve: %v", err)
	}
	if got.SessionID != "8f2a1111" {
		t.Fatalf("resolved to %q", got.SessionID)
	}
	if _, err := s.GetSession(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown id must be ErrNotFound, got %v", err)
	}
}

func TestEndSessionIsIdempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.WithTx(ctx, func(tx *Tx) error {
		if err := tx.UpsertSession(session("sess-1", ns(0), ns(0))); err != nil {
			return err
		}
		if err := tx.EndSession("sess-1", "active", "completed", ns(time.Minute)); err != nil {
			return err
		}
		return tx.EndSession("sess-1", "active", "completed", ns(time.Hour))
	}); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.EndedNs == nil || *got.EndedNs != ns(time.Minute) {
		t.Fatalf("ended_ns = %v, want the first end time to stand", got.EndedNs)
	}
}
