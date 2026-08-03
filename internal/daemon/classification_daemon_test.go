package daemon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func TestClassificationCadenceStillConsumesIntermediateEvidence(t *testing.T) {
	cfg := config.Default()
	cfg.Sampling.ActivitySample = config.Duration(time.Minute)
	cfg.Sampling.Classification = config.Duration(3 * time.Minute)
	root, child, snaps := detachedSequence(7)
	h := newHarnessConfig(t, cfg, snaps...)
	h.fake.SetActivity(root.Key(), completeSample(root.Key(), time.Minute, time.Second))
	h.fake.SetActivity(child.Key(),
		completeSample(child.Key(), time.Minute, time.Second),
		completeSample(child.Key(), 2*time.Minute, time.Second),
		completeSample(child.Key(), 3*time.Minute, time.Second),
		completeSample(child.Key(), 4*time.Minute, time.Second),
		completeSample(child.Key(), 5*time.Minute, 2*time.Second),
		completeSample(child.Key(), 6*time.Minute, 2*time.Second),
		completeSample(child.Key(), 7*time.Minute, 2*time.Second),
	)
	for range snaps {
		h.d.ScanNow(context.Background())
	}
	assertLatestChildIdle(t, h, child.Key())
}

func TestFailedScanResetsStrongClassificationWindow(t *testing.T) {
	cfg := config.Default()
	cfg.Sampling.ActivitySample = config.Duration(time.Minute)
	cfg.Sampling.Classification = config.Duration(time.Minute)
	root, child, snaps := detachedSequence(7)
	h := newHarnessConfig(t, cfg, snaps...)
	h.fake.SetActivity(root.Key(), completeSample(root.Key(), time.Minute, time.Second))
	var samples []process.ActivitySample
	for minute := 1; minute <= 7; minute++ {
		samples = append(samples, completeSample(child.Key(), time.Duration(minute)*time.Minute, time.Second))
	}
	h.fake.SetActivity(child.Key(), samples...)
	for range 6 {
		h.d.ScanNow(context.Background())
	}
	h.fake.Err = errors.New("scripted evidence gap")
	h.d.ScanNow(context.Background())
	h.d.ScanNow(context.Background())
	assertLatestChildIdle(t, h, child.Key())
}

func detachedSequence(count int) (process.Process, process.Process, []*process.Snapshot) {
	init := mk(1, 0, "/sbin/launchd", 0)
	root := codexRoot(100, 1, time.Second)
	child := mk(200, 100, "/usr/local/bin/helper", 2*time.Second)
	snaps := []*process.Snapshot{snapshot(time.Minute, init, root, child)}
	for minute := 2; minute <= count; minute++ {
		snaps = append(snaps, snapshot(time.Duration(minute)*time.Minute, init, withParent(child, 1)))
	}
	return root, child, snaps
}

func completeSample(key process.Key, at, cpu time.Duration) process.ActivitySample {
	return process.ActivitySample{Key: key, Taken: t0.Add(at), CPUTime: cpu,
		CPUKnown: true, IOKnown: true, FilesKnown: true, SocketsKnown: true}
}

func assertLatestChildIdle(t *testing.T, h *harness, key process.Key) {
	t.Helper()
	resp, err := h.d.Classifications(context.Background(), api.ClassificationOptions{ProcUID: key.UID(), Latest: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Classifications) != 1 || resp.Classifications[0].State != "idle" {
		t.Fatalf("latest classification = %+v, want reset idle window", resp.Classifications)
	}
}
