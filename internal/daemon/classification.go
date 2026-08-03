package daemon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jamesonstone/ghostgc/internal/classification"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type classificationBatch struct {
	due      bool
	at       time.Time
	records  []storage.ClassificationRecord
	previous map[string]classification.Previous
}

func (d *Daemon) classifyActivity(ctx context.Context, snap *process.Snapshot, tree *process.Tree,
	res *sessions.Result, activity activityBatch) (classificationBatch, error) {
	if !activity.due {
		return classificationBatch{}, nil
	}
	if !d.lastClassificationAt.IsZero() &&
		activity.at.Sub(d.lastClassificationAt) < d.cfg.Sampling.Classification.D() {
		return classificationBatch{}, nil
	}
	sessionsByID, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return classificationBatch{}, err
	}
	ended := make(map[string]bool, len(sessionsByID)+len(res.Ended))
	for _, rec := range sessionsByID {
		ended[rec.SessionID] = rec.EndedNs != nil
	}
	for _, rec := range res.Ended {
		ended[rec.SessionID] = true
	}
	batch := classificationBatch{due: true, at: activity.at, previous: make(map[string]classification.Previous)}
	for _, rec := range activity.records {
		key, err := process.ParseKey(rec.ProcUID)
		if err != nil {
			continue
		}
		proc, ok := snap.ByPID(key.PID)
		if !ok || proc.Key() != key {
			continue
		}
		attr := res.Attributions[rec.ProcUID]
		detached := tree.Link(proc.PID) == process.LinkReparented ||
			(attr.OriginalParentObserved && proc.PPID != attr.OriginalPPID)
		result := classification.Classify(classification.Input{
			Key: key, Status: proc.Status, Detached: detached,
			SessionEnded: ended[rec.SessionID], Previous: d.classificationPrevious[rec.ProcUID],
			Activity: classification.Activity{
				Taken: time.Unix(0, rec.TsNs), BaselineOK: rec.BaselineOK,
				CPUPercent: rec.CPUPercent, CPUKnown: rec.CPUKnown,
				DiskReadBytes: rec.DiskReadBytes, DiskWrittenBytes: rec.DiskWrittenBytes, IOKnown: rec.IOKnown,
				WritableRepositoryFiles: rec.WritableRepositoryFiles, FilesKnown: rec.FilesKnown,
				ConnectedSockets: rec.ConnectedSockets, NetworkChanged: rec.NetworkChanged, SocketsKnown: rec.SocketsKnown,
			},
		})
		evidence, _ := json.Marshal(result.Evidence)
		batch.records = append(batch.records, storage.ClassificationRecord{
			ProcUID: rec.ProcUID, SessionID: rec.SessionID, TsNs: snap.Taken.UnixNano(),
			ActivityTsNs: rec.TsNs, State: string(result.State), BasisState: string(result.Basis),
			Detached: result.Detached, SessionEnded: result.SessionEnded,
			StableSinceNs: result.StableSince.UnixNano(), EvidenceJSON: string(evidence),
		})
		batch.previous[rec.ProcUID] = classification.Previous{
			Key: key, Basis: result.Basis, Detached: result.Detached, SessionEnded: result.SessionEnded,
			ProcessStatus: proc.Status, StableSince: result.StableSince,
		}
	}
	return batch, nil
}

func (d *Daemon) commitClassifications(batch classificationBatch) {
	if !batch.due {
		return
	}
	d.classificationPrevious = batch.previous
	d.lastClassificationAt = batch.at
}
