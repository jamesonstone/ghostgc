package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func cmdClassifications(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "classifications", "[flags]")
	var opts api.ClassificationOptions
	var history bool
	fs.StringVar(&opts.SessionID, "session", "", "session id")
	fs.StringVar(&opts.ProcUID, "process", "", "exact process uid")
	fs.StringVar(&opts.State, "state", "", "classification state")
	fs.IntVar(&opts.Limit, "limit", 100, "maximum records")
	fs.BoolVar(&history, "history", false, "include historical results")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts.Latest = !history
	resp, err := e.api().Classifications(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	if len(resp.Classifications) == 0 {
		fmt.Println("No classifications have been recorded yet.")
		return nil
	}
	fmt.Println("TIME      PID     SESSION   STATE       BASIS     DETACHED  STABLE SINCE")
	for _, rec := range resp.Classifications {
		key, _ := process.ParseKey(rec.ProcUID)
		fmt.Printf("%-9s %-7d %-9s %-11s %-9s %-9t %s\n",
			time.Unix(0, rec.TsNs).Format("15:04:05"), key.PID, activityShortID(rec.SessionID),
			rec.State, rec.BasisState, rec.Detached, time.Unix(0, rec.StableSinceNs).Format("15:04:05"))
	}
	return nil
}
