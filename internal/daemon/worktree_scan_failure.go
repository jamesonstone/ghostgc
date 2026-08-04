package daemon

import (
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func (d *Daemon) unknownWorktreeBatch(existing []storage.WorktreeRecord, now time.Time, cause error) worktreeBatch {
	batch := worktreeBatch{due: true, at: now}
	for _, record := range existing {
		if record.State == string(worktree.StateRemoved) {
			continue
		}
		record.State = string(worktree.StateUnknown)
		record.LastSeenNs = now.UnixNano()
		record.LastActivityNs = now.UnixNano()
		record.InactiveSinceNs = 0
		record.DaemonStartedNs = d.startedAt.UnixNano()
		record.Complete = false
		record.ProtectionJSON = `["worktree_scan_incomplete"]`
		batch.records = append(batch.records, record)
	}
	batch.audit = append(batch.audit, storage.AuditRecord{
		TsNs: now.UnixNano(), Kind: "worktree.scan.incomplete", Subject: "worktrees",
		Summary:      "worktree observation was incomplete; every inactivity window was reset",
		EvidenceJSON: marshalJSON([]map[string]string{{"rule": "complete-worktree-observation-v1", "detail": fmt.Sprintf("inventory failed closed: %v", cause)}}, "[]"),
	})
	return batch
}

func missingWorktreeRecord(record storage.WorktreeRecord, now, daemonStarted time.Time) storage.WorktreeRecord {
	record.State = string(worktree.StateMissing)
	record.LastSeenNs = now.UnixNano()
	record.LastActivityNs = now.UnixNano()
	record.InactiveSinceNs = 0
	record.DaemonStartedNs = daemonStarted.UnixNano()
	record.Complete = false
	record.ProtectionJSON = `["worktree_missing"]`
	return record
}

func (d *Daemon) incompleteProcessWorktreeBatch(batch worktreeBatch, cause error) worktreeBatch {
	for index := range batch.records {
		record := &batch.records[index]
		if record.State == string(worktree.StateRemoved) {
			continue
		}
		record.State = string(worktree.StateUnknown)
		record.LastActivityNs = batch.at.UnixNano()
		record.InactiveSinceNs = 0
		record.DaemonStartedNs = d.startedAt.UnixNano()
		record.Complete = false
		record.ProtectionJSON = `["process_scan_incomplete"]`
	}
	batch.audit = append(batch.audit, storage.AuditRecord{
		TsNs: batch.at.UnixNano(), Kind: "worktree.scan.incomplete", Subject: "worktrees",
		Summary:      "worktree inventory was refreshed without complete process evidence; inactivity was reset",
		EvidenceJSON: marshalJSON([]map[string]string{{"rule": "complete-process-evidence-v1", "detail": cause.Error()}}, "[]"),
	})
	return batch
}
