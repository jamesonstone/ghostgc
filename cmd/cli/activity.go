package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func cmdActivity(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "activity", "[flags]")
	var opts api.ActivityOptions
	fs.StringVar(&opts.SessionID, "session", "", "filter by session id")
	fs.StringVar(&opts.ProcUID, "process", "", "filter by exact pid:start-time process key")
	fs.IntVar(&opts.Limit, "limit", 100, "maximum samples")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Activity(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderActivity(resp)
	return nil
}

func renderActivity(resp api.ActivityResponse) {
	if len(resp.Samples) == 0 {
		fmt.Println("No activity samples have been recorded yet.")
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "TIME\tPID\tSESSION\tCPU\tDISK R/W\tFILES\tSOCKETS\tEVIDENCE")
	for _, sample := range resp.Samples {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			time.Unix(0, sample.TsNs).Format("15:04:05"), activityPID(sample), activityShortID(sample.SessionID),
			activityCPU(sample), activityIO(sample), activityFiles(sample), activitySockets(sample),
			activityEvidence(sample))
	}
	_ = w.Flush()
	if resp.Note != "" {
		fmt.Printf("\n%s\n", resp.Note)
	}
}

func activityShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func activityPID(sample storage.ActivityRecord) int {
	key, err := process.ParseKey(sample.ProcUID)
	if err != nil {
		return 0
	}
	return key.PID
}

func activityCPU(sample storage.ActivityRecord) string {
	if !sample.CPUKnown {
		return "?"
	}
	return fmt.Sprintf("%.1f%%", sample.CPUPercent)
}

func activityIO(sample storage.ActivityRecord) string {
	if !sample.IOKnown {
		return "?"
	}
	return fmt.Sprintf("%s/%s", humanBytes(uint64(sample.DiskReadBytes)), humanBytes(uint64(sample.DiskWrittenBytes)))
}

func activityFiles(sample storage.ActivityRecord) string {
	if !sample.FilesKnown {
		return "?"
	}
	return fmt.Sprintf("%d (%d writable)", sample.OpenFiles, sample.WritableRepositoryFiles)
}

func activitySockets(sample storage.ActivityRecord) string {
	if !sample.SocketsKnown {
		return "?"
	}
	return fmt.Sprintf("%d (%d connected)", sample.Sockets, sample.ConnectedSockets)
}

func activityEvidence(sample storage.ActivityRecord) string {
	if sample.Note != "" {
		return sample.Note
	}
	if !sample.BaselineOK {
		return "baseline pending"
	}
	if !sample.CPUKnown || !sample.IOKnown || !sample.FilesKnown || !sample.SocketsKnown {
		return "partial: unavailable metrics"
	}
	return "complete"
}
