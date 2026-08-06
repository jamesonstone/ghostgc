package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
)

var (
	startLogRetryInterval = 100 * time.Millisecond
	startLogReadyTimeout  = 10 * time.Second
)

type startOptions struct {
	mode config.StartupMode
	logs bool
}

func cmdStart(ctx context.Context, e *env, args []string) error {
	opts, err := parseStartOptions(e, args)
	if err != nil {
		return err
	}
	plat, err := platform.New(platform.Options{})
	if err != nil {
		return err
	}
	return startWithPlatform(ctx, e, plat, opts)
}

func parseStartOptions(e *env, args []string) (startOptions, error) {
	fs := newFlagSet(e, "start", "[--mode audit|reconcile] [--logs]")
	value := fs.String("mode", string(config.StartupAudit), "startup mode: audit or reconcile")
	follow := fs.Bool("logs", false, "follow the audit log after startup")
	if err := fs.Parse(args); err != nil {
		return startOptions{}, err
	}
	if fs.NArg() != 0 {
		return startOptions{}, fmt.Errorf("unexpected start argument %q", fs.Arg(0))
	}
	mode, err := config.ParseStartupMode(*value)
	return startOptions{mode: mode, logs: *follow}, err
}

func startWithPlatform(ctx context.Context, e *env, plat platform.Platform, opts startOptions) error {
	if err := installBackground(ctx, e, plat, &opts.mode); err != nil {
		return err
	}
	if !opts.logs {
		return nil
	}
	return followStartedLogs(ctx, e)
}

func followStartedLogs(ctx context.Context, e *env) error {
	started := *e
	fetch := e.logs
	ready := false
	started.fetchLogs = func(ctx context.Context, opts api.LogOptions) (api.LogsResponse, error) {
		if ready {
			return fetch(ctx, opts)
		}
		resp, err := waitForStartedLogs(ctx, fetch, opts)
		if err == nil {
			ready = true
		}
		return resp, err
	}
	if err := cmdLogs(ctx, &started, nil); err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func waitForStartedLogs(ctx context.Context, fetch logFetcher, opts api.LogOptions) (api.LogsResponse, error) {
	deadline := time.Now().Add(startLogReadyTimeout)
	for {
		resp, err := fetch(ctx, opts)
		switch {
		case err == nil:
			return resp, nil
		case ctx.Err() != nil:
			return api.LogsResponse{}, ctx.Err()
		case !errors.Is(err, api.ErrDaemonUnreachable):
			return api.LogsResponse{}, err
		case !time.Now().Before(deadline):
			return api.LogsResponse{}, fmt.Errorf("ghostgc started but logs did not become available within %s: %w", startLogReadyTimeout, err)
		}

		delay := startLogRetryInterval
		if remaining := time.Until(deadline); delay > remaining {
			delay = remaining
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return api.LogsResponse{}, ctx.Err()
		case <-timer.C:
		}
	}
}
