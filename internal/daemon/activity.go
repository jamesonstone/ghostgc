package daemon

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type activityBatch struct {
	due       bool
	at        time.Time
	records   []storage.ActivityRecord
	baselines map[string]process.ActivitySample
}

func (d *Daemon) collectActivity(ctx context.Context, snap *process.Snapshot, res *sessions.Result) activityBatch {
	interval := d.cfg.Sampling.ActivitySample.D()
	if !d.lastActivityAt.IsZero() && snap.Taken.Sub(d.lastActivityAt) < interval {
		return activityBatch{}
	}
	batch := activityBatch{due: true, at: snap.Taken, baselines: make(map[string]process.ActivitySample)}
	for _, p := range snap.Processes {
		if err := ctx.Err(); err != nil {
			break
		}
		uid := p.Key().UID()
		attr, ok := res.Attributions[uid]
		if !ok || !attr.Attributed() {
			continue
		}
		sample, err := d.plat.SampleActivity(ctx, p.Key(), attr.RepositoryPath)
		if err == nil {
			switch {
			case sample.Key != p.Key():
				err = fmt.Errorf("collector returned key %s for requested process %s", sample.Key, p.Key())
			case sample.Taken.IsZero():
				err = fmt.Errorf("collector returned a zero sample time for process %s", p.Key())
			case sample.Taken.Before(snap.Taken):
				err = fmt.Errorf("collector sample time for %s predates its selecting snapshot", p.Key())
			}
		}
		if err != nil {
			batch.records = append(batch.records, storage.ActivityRecord{
				ProcUID: uid, SessionID: attr.SessionID, TsNs: snap.Taken.UnixNano(),
				Note: "activity unavailable: " + err.Error(),
			})
			continue
		}
		delta := process.DeriveActivity(d.activityBaseline[uid], sample)
		batch.baselines[uid] = sample
		batch.records = append(batch.records, activityRecord(attr.SessionID, sample, delta))
		if sample.SocketsKnown && sample.Sockets > 0 {
			res.Relationships = append(res.Relationships, storage.RelationshipRecord{
				SessionID: attr.SessionID, Kind: string(sessions.RelSocket), FromProcUID: uid,
				Detail: fmt.Sprintf("%d open socket descriptors (%d connected); endpoints are intentionally not stored",
					sample.Sockets, sample.ConnectedSockets),
				FirstSeenNs: snap.Taken.UnixNano(), LastSeenNs: snap.Taken.UnixNano(),
			})
		}
	}
	appendRepositoryLockRelationships(res, snap.Taken)
	return batch
}

func activityRecord(sessionID string, sample process.ActivitySample, delta process.ActivityDelta) storage.ActivityRecord {
	return storage.ActivityRecord{
		ProcUID: sample.Key.UID(), SessionID: sessionID, TsNs: delta.Taken.UnixNano(),
		IntervalNs: int64(delta.Interval), BaselineOK: delta.BaselineOK,
		CPUPercent: delta.CPUPercent, CPUDeltaNs: int64(delta.CPUDelta), CPUKnown: delta.CPUKnown,
		DiskReadBytes:    boundedInt64(delta.DiskReadBytes),
		DiskWrittenBytes: boundedInt64(delta.DiskWrittenBytes), IOKnown: delta.IOKnown,
		RSSBytes:  boundedInt64(sample.RSSBytes),
		OpenFiles: delta.OpenFiles, WritableRepositoryFiles: delta.WritableRepositoryFiles,
		FilesKnown: delta.FilesKnown, Sockets: delta.Sockets,
		ConnectedSockets:  delta.ConnectedSockets,
		ReceiveQueueBytes: boundedInt64(delta.ReceiveQueueBytes),
		SendQueueBytes:    boundedInt64(delta.SendQueueBytes),
		NetworkChanged:    delta.NetworkChanged, SocketsKnown: delta.SocketsKnown, Note: delta.Note,
	}
}

func boundedInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func appendRepositoryLockRelationships(res *sessions.Result, at time.Time) {
	for _, session := range res.Sessions {
		if !session.RepositoryBusy || session.RootProcUID == "" {
			continue
		}
		res.Relationships = append(res.Relationships, storage.RelationshipRecord{
			SessionID: session.SessionID, Kind: string(sessions.RelFileLock),
			FromProcUID: session.RootProcUID,
			Detail:      "repository metadata reports an in-flight git lock; lock ownership is not inferred",
			FirstSeenNs: at.UnixNano(), LastSeenNs: at.UnixNano(),
		})
	}
}

func (d *Daemon) commitActivity(batch activityBatch) {
	if !batch.due {
		return
	}
	// Replace rather than extend: exited, unreadable and de-attributed process
	// keys must not leak memory or regain a stale baseline after an evidence gap.
	d.activityBaseline = batch.baselines
	d.lastActivityAt = batch.at
}
