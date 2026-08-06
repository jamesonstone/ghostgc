package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

var logPollInterval = time.Second

type logFetcher func(context.Context, api.LogOptions) (api.LogsResponse, error)

func cmdLogs(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "logs", "[--follow=false] [--verbose] [--limit <n>] [--kind <kind>] [--subject <subject>]")
	var opts api.LogOptions
	follow := true
	verbose := false
	fs.BoolVar(&follow, "follow", true, "continue printing new entries")
	fs.BoolVar(&follow, "f", true, "shorthand for --follow")
	fs.BoolVar(&verbose, "verbose", false, "include process attribution entries while following")
	fs.BoolVar(&verbose, "v", false, "shorthand for --verbose")
	fs.IntVar(&opts.Limit, "limit", 50, "initial entries and follow batch size")
	fs.StringVar(&opts.Kind, "kind", "", "filter by entry kind")
	fs.StringVar(&opts.Subject, "subject", "", "filter by subject")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected logs argument %q", fs.Arg(0))
	}
	if follow && !verbose && opts.Kind == "" {
		opts.ExcludeKind = "process.attributed"
	}

	resp, err := e.logs(ctx, opts)
	if err != nil {
		return err
	}
	if err := emitInitialLogs(e, resp, follow); err != nil || !follow {
		return err
	}
	return followLogs(ctx, e, opts, highestAuditID(resp))
}

func (e *env) logs(ctx context.Context, opts api.LogOptions) (api.LogsResponse, error) {
	if e.fetchLogs != nil {
		return e.fetchLogs(ctx, opts)
	}
	return e.api().Logs(ctx, opts)
}

func followLogs(ctx context.Context, e *env, base api.LogOptions, cursor int64) error {
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		for {
			next := cursor
			base.AfterID = &next
			resp, err := e.logs(ctx, base)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if len(resp.Entries) == 0 {
				break
			}
			if err := emitFollowedLogs(e, resp); err != nil {
				return err
			}
			advanced := highestAuditID(resp)
			if advanced <= cursor {
				return fmt.Errorf("audit cursor did not advance beyond %d", cursor)
			}
			cursor = advanced
			if len(resp.Entries) < base.Limit {
				break
			}
		}
	}
}

func emitInitialLogs(e *env, resp api.LogsResponse, following bool) error {
	if e.jsonOut {
		if following {
			return emitLogJSONLine(resp)
		}
		return emitJSON(resp)
	}
	renderLogs(resp)
	return nil
}

func emitFollowedLogs(e *env, resp api.LogsResponse) error {
	if e.jsonOut {
		return emitLogJSONLine(resp)
	}
	renderFollowedLogs(resp)
	return nil
}

func emitLogJSONLine(resp api.LogsResponse) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}

func highestAuditID(resp api.LogsResponse) int64 {
	var highest int64
	for _, entry := range resp.Entries {
		if entry.ID > highest {
			highest = entry.ID
		}
	}
	return highest
}
