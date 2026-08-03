package daemon

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// runScan performs one observation cycle. It never returns an error: a failed
// scan is recorded and observation continues. Stopping on a transient
// inspection failure would turn a recoverable condition into an outage.
func (d *Daemon) runScan(ctx context.Context) {
	start := time.Now()

	snap, err := d.plat.SnapshotProcesses(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		d.recordScanFailure(ctx, start, fmt.Errorf("snapshot: %w", err))
		return
	}
	scanDuration := time.Since(start)
	tree := process.BuildTree(snap)

	reconcileStart := time.Now()
	result, err := d.recon.Reconcile(ctx, snap, tree, d.cfg.Privacy.StoreCommandLines)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		d.recordScanFailure(ctx, start, fmt.Errorf("reconcile: %w", err))
		return
	}
	reconcileDuration := time.Since(reconcileStart)
	activityStart := time.Now()
	activity := d.collectActivity(ctx, snap, result)
	activityDuration := time.Since(activityStart)

	persistStart := time.Now()
	if err := d.persist(ctx, snap, result, activity.records, start, scanDuration); err != nil {
		if ctx.Err() != nil {
			return
		}
		d.recordScanFailure(ctx, start, fmt.Errorf("persist: %w", err))
		return
	}
	persistDuration := time.Since(persistStart)

	// The in-memory view only advances once the write has landed.
	d.recon.Commit(result)
	d.commitActivity(activity)

	d.mu.Lock()
	d.snapshot = snap
	d.tree = tree
	d.last = result
	d.degraded = nil
	d.metrics.scanCount++
	d.metrics.lastScanDuration = scanDuration
	d.metrics.totalScanDuration += scanDuration
	if scanDuration > d.metrics.maxScanDuration {
		d.metrics.maxScanDuration = scanDuration
	}
	d.metrics.lastReconcile = reconcileDuration
	d.metrics.lastPersist = persistDuration
	d.metrics.lastActivity = activityDuration
	d.metrics.activitySamples += int64(len(activity.records))
	d.metrics.visibleProcesses = snap.TotalCount
	d.metrics.inspectedProcesses = snap.Len()
	d.metrics.attributed = result.AttributedCount
	d.mu.Unlock()

	d.log.Debug("scan complete",
		"visible", snap.TotalCount,
		"inspected", snap.Len(),
		"attributed", result.AttributedCount,
		"sessions", len(result.Sessions),
		"scan_ms", scanDuration.Milliseconds(),
		"reconcile_ms", reconcileDuration.Milliseconds(),
		"activity_ms", activityDuration.Milliseconds(),
		"activity_samples", len(activity.records),
		"persist_ms", persistDuration.Milliseconds(),
	)
}

func (d *Daemon) persist(ctx context.Context, snap *process.Snapshot, res *sessions.Result, activity []storage.ActivityRecord, start time.Time, scanDuration time.Duration) error {
	nowNs := snap.Taken.UnixNano()
	return d.store.WithTx(ctx, func(tx *storage.Tx) error {
		scanID, err := tx.InsertScan(storage.ScanRecord{
			StartedNs:           start.UnixNano(),
			DurationUs:          scanDuration.Microseconds(),
			VisibleProcesses:    snap.TotalCount,
			InspectedProcesses:  snap.Len(),
			AttributedProcesses: res.AttributedCount,
			Sessions:            len(res.Sessions),
		})
		if err != nil {
			return err
		}
		for _, s := range res.Sessions {
			if err := tx.UpsertSession(s); err != nil {
				return err
			}
		}
		for _, p := range res.Processes {
			if err := tx.UpsertProcess(p); err != nil {
				return err
			}
		}
		for _, o := range res.Ownership {
			if err := tx.UpsertOwnership(o); err != nil {
				return err
			}
		}
		for _, rel := range res.Relationships {
			if err := tx.UpsertRelationship(rel); err != nil {
				return err
			}
		}
		for _, obs := range res.Observations {
			obs.ScanID = scanID
			if err := tx.InsertObservation(obs); err != nil {
				return err
			}
		}
		for _, sample := range activity {
			if err := tx.InsertActivity(sample); err != nil {
				return err
			}
		}
		if _, err := tx.MarkExitedBefore(nowNs, nowNs); err != nil {
			return err
		}
		for _, e := range res.Ended {
			if err := tx.EndSession(e.SessionID, string(e.From), string(e.State), e.EndedNs); err != nil {
				return err
			}
		}
		for _, a := range res.Audit {
			if err := tx.AppendAudit(a); err != nil {
				return err
			}
		}
		return tx.SetMeta("last_scan_ns", fmt.Sprint(nowNs))
	})
}

func (d *Daemon) recordScanFailure(ctx context.Context, start time.Time, cause error) {
	d.mu.Lock()
	d.metrics.scanFailures++
	d.degraded = append(d.degraded[:0], cause.Error())
	d.mu.Unlock()

	d.log.Error("scan failed", "error", cause)
	err := d.store.WithTx(ctx, func(tx *storage.Tx) error {
		if _, err := tx.InsertScan(storage.ScanRecord{
			StartedNs:  start.UnixNano(),
			DurationUs: time.Since(start).Microseconds(),
			Error:      cause.Error(),
		}); err != nil {
			return err
		}
		return tx.AppendAudit(storage.AuditRecord{
			TsNs:    time.Now().UnixNano(),
			Kind:    AuditScanFailed,
			Subject: "daemon",
			Summary: "observation cycle failed and was skipped; no conclusion was drawn: " + cause.Error(),
		})
	})
	if err != nil {
		d.log.Error("recording scan failure failed", "error", err)
	}
}

// ScanNow performs one observation cycle synchronously. It exists for tests
// and for the daemon's own startup path.
func (d *Daemon) ScanNow(ctx context.Context) { d.runScan(ctx) }

func rssBytes() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys - m.HeapReleased
}
