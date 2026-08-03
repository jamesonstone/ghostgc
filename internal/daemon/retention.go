package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/storage"
)

func (d *Daemon) runRetention(ctx context.Context) {
	policy := storage.RetentionPolicy{
		RawObservations:  d.cfg.Retention.RawObservations.D(),
		Scans:            d.cfg.Retention.RawObservations.D(),
		Audit:            d.cfg.Retention.Actions.D(),
		PolicyDecisions:  d.cfg.Retention.Actions.D(),
		Actions:          d.cfg.Retention.Actions.D(),
		ExitedProcesses:  d.cfg.Retention.AggregatedObservations.D(),
		EndedSessions:    d.cfg.Retention.AggregatedObservations.D(),
		MaxDatabaseBytes: d.cfg.Retention.MaxDatabaseBytes,
	}
	res, err := d.store.Compact(ctx, policy, time.Now())
	if err != nil {
		d.log.Error("retention failed", "error", err)
		return
	}

	d.mu.Lock()
	d.metrics.retentionRuns++
	d.metrics.lastRetentionRows = res.Total()
	d.mu.Unlock()

	d.log.Info("retention complete",
		"deleted", res.Total(),
		"aggressive", res.Aggressive,
		"size_before", res.SizeBeforeBytes,
		"size_after", res.SizeAfterBytes,
	)
	if res.Total() > 0 || res.Aggressive {
		d.audit(ctx, AuditRetention, "daemon", fmt.Sprintf(
			"retention removed %d rows; database went from %d to %d bytes%s",
			res.Total(), res.SizeBeforeBytes, res.SizeAfterBytes,
			map[bool]string{true: " (aggressive pass: the size budget was exceeded)", false: ""}[res.Aggressive]))
	}
}

func (d *Daemon) audit(ctx context.Context, kind, subject, summary string) {
	if err := d.store.AppendAudit(ctx, storage.AuditRecord{
		TsNs:    time.Now().UnixNano(),
		Kind:    kind,
		Subject: subject,
		Summary: summary,
	}); err != nil {
		d.log.Error("appending audit entry failed", "kind", kind, "error", err)
	}
}

// RunRetentionNow exposes a compaction pass for tests and the CLI.
func (d *Daemon) RunRetentionNow(ctx context.Context) { d.runRetention(ctx) }
