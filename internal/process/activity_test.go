package process

import (
	"testing"
	"time"
)

func TestDeriveActivityRequiresExactIdentityAndBaseline(t *testing.T) {
	key := Key{PID: 42, StartTimeNs: 100}
	first := ActivitySample{
		Key: key, Taken: time.Unix(10, 0), CPUTime: time.Second,
		DiskReadBytes: 10, DiskWrittenBytes: 20, CPUKnown: true, IOKnown: true,
	}
	if got := DeriveActivity(ActivitySample{}, first); got.CPUKnown || got.IOKnown || got.BaselineOK {
		t.Fatalf("first sample must be unknown, got %+v", got)
	}

	reused := first
	reused.Key.StartTimeNs++
	reused.Taken = reused.Taken.Add(time.Second)
	if got := DeriveActivity(first, reused); got.CPUKnown || got.IOKnown {
		t.Fatal("a reused pid must not inherit another process's activity")
	}
}

func TestDeriveActivityComputesMonotonicDeltas(t *testing.T) {
	key := Key{PID: 42, StartTimeNs: 100}
	first := ActivitySample{
		Key: key, Taken: time.Unix(10, 0), CPUTime: time.Second,
		DiskReadBytes: 10, DiskWrittenBytes: 20, CPUKnown: true, IOKnown: true,
		Sockets: 1, ConnectedSockets: 1, SocketsKnown: true,
	}
	second := first
	second.Taken = first.Taken.Add(2 * time.Second)
	second.CPUTime += 500 * time.Millisecond
	second.DiskReadBytes += 30
	second.DiskWrittenBytes += 40
	second.ReceiveQueueBytes = 5

	got := DeriveActivity(first, second)
	if !got.CPUKnown || !got.IOKnown || !got.BaselineOK {
		t.Fatalf("expected a valid baseline, got %+v", got)
	}
	if got.CPUPercent != 25 || got.DiskReadBytes != 30 || got.DiskWrittenBytes != 40 {
		t.Fatalf("unexpected deltas: %+v", got)
	}
	if !got.NetworkChanged {
		t.Fatal("socket queue movement must be visible as network activity")
	}
}

func TestDeriveActivityRejectsCounterReset(t *testing.T) {
	key := Key{PID: 42, StartTimeNs: 100}
	first := ActivitySample{
		Key: key, Taken: time.Unix(10, 0), CPUTime: time.Second,
		DiskReadBytes: 10, CPUKnown: true, IOKnown: true,
	}
	second := first
	second.Taken = first.Taken.Add(time.Second)
	second.DiskReadBytes = 1
	if got := DeriveActivity(first, second); got.IOKnown {
		t.Fatal("a reset counter must be unknown, not negative or zero activity")
	}
}
