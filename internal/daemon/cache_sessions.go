package daemon

import (
	"context"
	"encoding/json"

	"github.com/jamesonstone/ghostgc/internal/cacheprovider"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func (d *Daemon) cacheSessionFacts(ctx context.Context) ([]cacheprovider.Session, error) {
	sessions, err := d.store.ListSessions(ctx, storage.SessionFilter{Limit: d.cfg.Cache.MaxEntriesPerScan + 1})
	if err != nil {
		return nil, err
	}
	out := make([]cacheprovider.Session, 0, len(sessions))
	for _, session := range sessions {
		var metadata struct {
			Extra map[string]string `json:"extra"`
		}
		_ = json.Unmarshal([]byte(session.MetadataJSON), &metadata)
		processes, err := d.store.ListProcesses(ctx, storage.ProcessFilter{
			SessionID: session.SessionID, LiveOnly: true,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, cacheprovider.Session{
			ID: session.SessionID, NativeID: session.NativeSessionID,
			Agent: session.AgentID, State: session.State,
			CodexHome: metadata.Extra["CODEX_HOME"], LiveProcesses: len(processes),
			Confidence: session.Confidence,
		})
	}
	return out, nil
}
