package daemon_test

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func TestActivityTargetsOnlyAttributedProcessesAndDerivesDeltas(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mk(200, 100, "/usr/local/bin/worker", 2*time.Second)
	unrelated := mk(300, 1, "/usr/local/bin/unrelated", 3*time.Second)
	h := newHarness(t,
		snapshot(time.Minute, init, root, child, unrelated),
		snapshot(2*time.Minute, init, root, child, unrelated),
	)

	rootKey, childKey := root.Key(), child.Key()
	h.fake.SetActivity(rootKey,
		activitySample(rootKey, time.Minute, time.Second, 100, 200),
		activitySample(rootKey, 2*time.Minute, 7*time.Second, 160, 240),
	)
	h.fake.SetActivity(childKey,
		activitySample(childKey, time.Minute, 2*time.Second, 300, 400),
		activitySample(childKey, 2*time.Minute, 5*time.Second, 320, 480),
	)

	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)

	calls := h.fake.ActivityCalls()
	if len(calls) != 4 {
		t.Fatalf("targeted activity calls = %d, want two attributed processes over two samples", len(calls))
	}
	for _, key := range calls {
		if key != rootKey && key != childKey {
			t.Fatalf("expensive inspection reached unattributed process %s", key)
		}
	}

	resp, err := h.d.Activity(ctx, api.ActivityOptions{ProcUID: rootKey.UID()})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Samples) != 2 {
		t.Fatalf("root samples = %d, want 2", len(resp.Samples))
	}
	latest := resp.Samples[0]
	if !latest.BaselineOK || !latest.CPUKnown || latest.CPUDeltaNs != int64(6*time.Second) {
		t.Fatalf("CPU delta was not derived from the exact-key baseline: %+v", latest)
	}
	if !latest.IOKnown || latest.DiskReadBytes != 60 || latest.DiskWrittenBytes != 40 {
		t.Fatalf("I/O delta = %d/%d known=%t", latest.DiskReadBytes, latest.DiskWrittenBytes, latest.IOKnown)
	}
	if !latest.FilesKnown || latest.WritableRepositoryFiles != 1 || !latest.SocketsKnown || !latest.NetworkChanged {
		t.Fatalf("descriptor evidence was not preserved: %+v", latest)
	}
}

func TestActivityCadenceSkipsIntermediateScans(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	h := newHarness(t,
		snapshot(time.Minute, init, root),
		snapshot(75*time.Second, init, root),
		snapshot(2*time.Minute, init, root),
	)
	h.fake.SetActivity(root.Key(),
		activitySample(root.Key(), time.Minute, time.Second, 0, 0),
		activitySample(root.Key(), 2*time.Minute, 2*time.Second, 0, 0),
	)

	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	if got := len(h.fake.ActivityCalls()); got != 2 {
		t.Fatalf("activity calls = %d, want one per 60-second cadence", got)
	}
}

func activitySample(key process.Key, at time.Duration, cpu time.Duration, read, written uint64) process.ActivitySample {
	return process.ActivitySample{
		Key: key, Taken: t0.Add(at), CPUTime: cpu, CPUKnown: true,
		DiskReadBytes: read, DiskWrittenBytes: written, IOKnown: true,
		RSSBytes:  8 << 20,
		OpenFiles: 3, WritableRepositoryFiles: 1, FilesKnown: true,
		Sockets: 1, ConnectedSockets: 1,
		ReceiveQueueBytes: read, SendQueueBytes: written, SocketsKnown: true,
	}
}
