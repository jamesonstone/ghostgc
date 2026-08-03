package daemon

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Processes implements api.Backend.
func (d *Daemon) Processes(ctx context.Context, opts api.ListOptions) (api.ProcessesResponse, error) {
	filter := storage.ProcessFilter{
		SessionID: opts.SessionID,
		AgentID:   opts.AgentID,
		LiveOnly:  !opts.All,
		Limit:     opts.Limit,
	}
	recs, err := d.store.ListProcesses(ctx, filter)
	if err != nil {
		return api.ProcessesResponse{}, err
	}
	classes, err := d.store.ListClassifications(ctx, storage.ClassificationFilter{Latest: true, Limit: 1000})
	if err != nil {
		return api.ProcessesResponse{}, err
	}
	byProcess := make(map[string]storage.ClassificationRecord, len(classes))
	for _, class := range classes {
		byProcess[class.ProcUID] = class
	}
	resp := api.ProcessesResponse{
		Processes: make([]api.ProcessSummary, 0, len(recs)),
		Note:      "Only processes attributed to an agent session are recorded. Everything else on the machine is counted during each scan and then forgotten, because monitoring activity outside coding-agent sessions is a non-goal.",
	}
	for _, rec := range recs {
		summary := d.processSummary(rec)
		if class, ok := byProcess[rec.ProcUID]; ok {
			summary.ActivityState, summary.Detached, summary.ClassificationTsNs = class.State, class.Detached, class.TsNs
		}
		resp.Processes = append(resp.Processes, summary)
	}
	return resp, nil
}

func (d *Daemon) processSummary(rec storage.ProcessRecord) api.ProcessSummary {
	var cmdline []string
	_ = json.Unmarshal([]byte(rec.Cmdline), &cmdline)

	summary := api.ProcessSummary{
		ProcUID:                rec.ProcUID,
		PID:                    rec.PID,
		PPID:                   rec.PPID,
		OriginalPPID:           rec.OriginalPPID,
		OriginalParentObserved: rec.OriginalParentObserved,
		ExecPath:               rec.ExecPath,
		Cmdline:                cmdline,
		CWD:                    rec.CWD,
		TTY:                    rec.TTY,
		RepositoryPath:         rec.RepositoryPath,
		AgentID:                rec.AgentID,
		SessionID:              rec.SessionID,
		ShortID:                shortID(rec.SessionID),
		Relation:               rec.Relation,
		Confidence:             rec.Confidence,
		AgeSeconds:             time.Since(time.Unix(0, rec.StartTimeNs)).Seconds(),
		Live:                   rec.ExitedAtNs == nil,
	}
	summary.Name = rec.Comm
	summary.State = "unknown"

	d.mu.RLock()
	snap := d.snapshot
	d.mu.RUnlock()
	if snap != nil {
		if p, ok := snap.ByKey(process.Key{PID: rec.PID, StartTimeNs: rec.StartTimeNs}); ok {
			summary.Name = p.Name()
			summary.RSSBytes = p.RSSBytes
			summary.CPUSeconds = p.CPUTime.Seconds()
			summary.Threads = p.Threads
			summary.State = string(p.Status)
		}
	}
	if !summary.Live {
		summary.State = "exited"
	}
	return summary
}
