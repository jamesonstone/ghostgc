package daemon_test

import (
	"context"
	"strings"
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

func TestActivityDropsBaselineAcrossAttributionGap(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	unreadable := root
	unreadable.Detailed = false
	h := newHarness(t,
		snapshot(time.Minute, init, root),
		snapshot(2*time.Minute, init, unreadable),
		snapshot(3*time.Minute, init, root),
	)
	h.fake.SetActivity(root.Key(),
		activitySample(root.Key(), time.Minute, time.Second, 10, 20),
		activitySample(root.Key(), 3*time.Minute, 10*time.Second, 50, 60),
	)

	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	resp, err := h.d.Activity(ctx, api.ActivityOptions{ProcUID: root.Key().UID()})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Samples) != 2 {
		t.Fatalf("samples = %d, want the two successfully attributed passes", len(resp.Samples))
	}
	if resp.Samples[0].BaselineOK || resp.Samples[0].CPUKnown || resp.Samples[0].IOKnown {
		t.Fatalf("activity after an attribution gap reused a stale baseline: %+v", resp.Samples[0])
	}
}

func TestActivityPreservesCollectorTimestamp(t *testing.T) {
	ctx := context.Background()
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	h := newHarness(t,
		snapshot(time.Minute, init, root),
		snapshot(2*time.Minute, init, root),
	)
	first := activitySample(root.Key(), time.Minute+2*time.Second, time.Second, 10, 20)
	second := activitySample(root.Key(), 2*time.Minute+3*time.Second, 2*time.Second, 20, 30)
	h.fake.SetActivity(root.Key(), first, second)

	h.d.ScanNow(ctx)
	h.d.ScanNow(ctx)
	resp, err := h.d.Activity(ctx, api.ActivityOptions{ProcUID: root.Key().UID()})
	if err != nil {
		t.Fatal(err)
	}
	latest := resp.Samples[0]
	if latest.TsNs != second.Taken.UnixNano() || latest.IntervalNs != int64(61*time.Second) {
		t.Fatalf("stored time/interval = %d/%s, want %d/%s", latest.TsNs,
			time.Duration(latest.IntervalNs), second.Taken.UnixNano(), 61*time.Second)
	}
}

func TestActivityRejectsInvalidCollectorIdentityAndTime(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*process.ActivitySample)
		want   string
	}{
		{name: "wrong key", mutate: func(s *process.ActivitySample) { s.Key.StartTimeNs++ }, want: "collector returned key"},
		{name: "zero time", mutate: func(s *process.ActivitySample) { s.Taken = time.Time{} }, want: "zero sample time"},
		{name: "old time", mutate: func(s *process.ActivitySample) { s.Taken = t0.Add(59 * time.Second) }, want: "predates"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			init := mk(1, 0, "/sbin/launchd", 0)
			root := codexRoot(100, 1, time.Second)
			h := newHarness(t, snapshot(time.Minute, init, root))
			sample := activitySample(root.Key(), time.Minute, time.Second, 10, 20)
			tt.mutate(&sample)
			h.fake.SetActivity(root.Key(), sample)

			h.d.ScanNow(ctx)
			resp, err := h.d.Activity(ctx, api.ActivityOptions{ProcUID: root.Key().UID()})
			if err != nil {
				t.Fatal(err)
			}
			if len(resp.Samples) != 1 || !strings.Contains(resp.Samples[0].Note, tt.want) {
				t.Fatalf("invalid sample was not rejected as unavailable: %+v", resp.Samples)
			}
			if resp.Samples[0].CPUKnown || resp.Samples[0].IOKnown {
				t.Fatal("an invalid collector sample must not retain activity evidence")
			}
		})
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
