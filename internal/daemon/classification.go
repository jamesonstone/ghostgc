package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/classification"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

type classificationBatch struct {
	due      bool
	emit     bool
	at       time.Time
	records  []storage.ClassificationRecord
	current  []storage.ClassificationRecord
	previous map[string]classification.Previous
}

func (d *Daemon) classifyActivity(ctx context.Context, snap *process.Snapshot,
	res *sessions.Result, activity activityBatch) (classificationBatch, error) {
	if !activity.due {
		return classificationBatch{}, nil
	}
	emit := d.lastClassificationAt.IsZero() ||
		activity.at.Sub(d.lastClassificationAt) >= d.cfg.Sampling.Classification.D()
	sessionsByID, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return classificationBatch{}, err
	}
	ended := make(map[string]bool, len(sessionsByID)+len(res.Ended))
	for _, rec := range sessionsByID {
		ended[rec.SessionID] = rec.EndedNs != nil
	}
	for _, rec := range res.Sessions {
		ended[rec.SessionID] = rec.EndedNs != nil
	}
	for _, rec := range res.Ended {
		ended[rec.SessionID] = true
	}
	batch := classificationBatch{due: true, emit: emit, at: activity.at, previous: make(map[string]classification.Previous)}
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
		detached := attr.OriginalParentObserved && proc.PPID != attr.OriginalPPID
		detachmentDetail := ""
		if detached {
			detachmentDetail = fmt.Sprintf("observed original parent pid %d was replaced by current parent pid %d", attr.OriginalPPID, proc.PPID)
		}
		result := classification.Classify(classification.Input{
			Key: key, Status: proc.Status, Detached: detached,
			SessionEnded: ended[rec.SessionID], Previous: d.classificationPrevious[rec.ProcUID],
			EvidenceCadence: d.cfg.Sampling.ActivitySample.D(), DetachmentDetail: detachmentDetail,
			Activity: classification.Activity{
				Taken: time.Unix(0, rec.TsNs), BaselineOK: rec.BaselineOK,
				CPUPercent: rec.CPUPercent, CPUKnown: rec.CPUKnown,
				DiskReadBytes: rec.DiskReadBytes, DiskWrittenBytes: rec.DiskWrittenBytes, IOKnown: rec.IOKnown,
				WritableRepositoryFiles: rec.WritableRepositoryFiles, FilesKnown: rec.FilesKnown,
				ConnectedSockets: rec.ConnectedSockets, NetworkChanged: rec.NetworkChanged, SocketsKnown: rec.SocketsKnown,
			},
		})
		evidence, _ := json.Marshal(result.Evidence)
		current := storage.ClassificationRecord{
			ProcUID: rec.ProcUID, SessionID: rec.SessionID, TsNs: rec.TsNs,
			ActivityTsNs: rec.TsNs, State: string(result.State), BasisState: string(result.Basis),
			Detached: result.Detached, SessionEnded: result.SessionEnded,
			StableSinceNs: result.StableSince.UnixNano(), EvidenceJSON: string(evidence),
		}
		batch.current = append(batch.current, current)
		if emit {
			batch.records = append(batch.records, current)
		}
		batch.previous[rec.ProcUID] = classification.Previous{
			Key: key, Basis: result.Basis, Detached: result.Detached, SessionEnded: result.SessionEnded,
			ProcessStatus: proc.Status, StableSince: result.StableSince, SampledAt: time.Unix(0, rec.TsNs),
		}
	}
	return batch, nil
}

func (d *Daemon) resetClassificationEvidence() {
	d.classificationPrevious = make(map[string]classification.Previous)
}

func (d *Daemon) commitClassifications(batch classificationBatch) {
	if !batch.due {
		return
	}
	d.classificationPrevious = batch.previous
	if batch.emit {
		d.lastClassificationAt = batch.at
	}
}
