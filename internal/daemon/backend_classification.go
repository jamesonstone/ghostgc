package daemon

import (
	"context"
	"encoding/json"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Classifications implements api.Backend.
func (d *Daemon) Classifications(ctx context.Context, opts api.ClassificationOptions) (api.ClassificationsResponse, error) {
	recs, err := d.store.ListClassifications(ctx, storage.ClassificationFilter{
		ProcUID: opts.ProcUID, SessionID: opts.SessionID, State: opts.State,
		SinceNs: opts.SinceNs, Limit: opts.Limit, Latest: opts.Latest,
	})
	if err != nil {
		return api.ClassificationsResponse{}, err
	}
	out := api.ClassificationsResponse{Classifications: make([]api.ClassificationView, 0, len(recs))}
	for _, rec := range recs {
		view := api.ClassificationView{
			ID: rec.ID, ProcUID: rec.ProcUID, SessionID: rec.SessionID, TsNs: rec.TsNs,
			ActivityTsNs: rec.ActivityTsNs, State: rec.State, BasisState: rec.BasisState,
			Detached: rec.Detached, SessionEnded: rec.SessionEnded, StableSinceNs: rec.StableSinceNs,
		}
		_ = json.Unmarshal([]byte(rec.EvidenceJSON), &view.Evidence)
		out.Classifications = append(out.Classifications, view)
	}
	return out, nil
}
