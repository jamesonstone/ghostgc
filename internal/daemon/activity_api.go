package daemon

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Activity implements api.Backend.
func (d *Daemon) Activity(ctx context.Context, opts api.ActivityOptions) (api.ActivityResponse, error) {
	records, err := d.store.ListActivity(ctx, storage.ActivityFilter{
		ProcUID: opts.ProcUID, SessionID: opts.SessionID, SinceNs: opts.SinceNs, Limit: opts.Limit,
	})
	if err != nil {
		return api.ActivityResponse{}, err
	}
	return api.ActivityResponse{
		Samples: records,
		Note:    "Unknown metrics are unavailable evidence, not observed inactivity.",
	}, nil
}
