package storage

import (
	"context"
	"testing"
	"time"
)

func TestActivityRoundTripsAvailability(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	want := ActivityRecord{
		ProcUID: "42:100", SessionID: "session-1", TsNs: 200, IntervalNs: 100,
		BaselineOK: true, CPUPercent: 12.5, CPUDeltaNs: 25, CPUKnown: true,
		DiskReadBytes: 30, DiskWrittenBytes: 40, IOKnown: true, RSSBytes: 50,
		OpenFiles: 4, WritableRepositoryFiles: 1, FilesKnown: true,
		Sockets: 2, ConnectedSockets: 1, ReceiveQueueBytes: 6, SendQueueBytes: 7,
		NetworkChanged: true, SocketsKnown: true,
	}
	if err := s.WithTx(ctx, func(tx *Tx) error { return tx.InsertActivity(want) }); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListActivity(ctx, ActivityFilter{ProcUID: want.ProcUID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].CPUKnown || !got[0].IOKnown || !got[0].FilesKnown || !got[0].SocketsKnown {
		t.Fatalf("activity availability did not round-trip: %+v", got)
	}
	if got[0].CPUPercent != want.CPUPercent || got[0].WritableRepositoryFiles != 1 || !got[0].NetworkChanged {
		t.Fatalf("activity values did not round-trip: %+v", got[0])
	}
}

func TestRetentionBoundsActivity(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()
	if err := s.WithTx(ctx, func(tx *Tx) error {
		return tx.InsertActivity(ActivityRecord{ProcUID: "42:100", SessionID: "session-1", TsNs: now.Add(-2 * time.Hour).UnixNano()})
	}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Compact(ctx, RetentionPolicy{
		RawObservations: time.Hour, Scans: time.Hour, Audit: time.Hour,
		ExitedProcesses: time.Hour, EndedSessions: time.Hour,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.ActivitySamples != 1 {
		t.Fatalf("deleted %d activity samples, want 1", res.ActivitySamples)
	}
}
