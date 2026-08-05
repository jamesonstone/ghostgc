package main

import (
	"context"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
)

func cmdStart(ctx context.Context, e *env, args []string) error {
	mode, err := startMode(e, args)
	if err != nil {
		return err
	}
	plat, err := platform.New(platform.Options{})
	if err != nil {
		return err
	}
	return installBackground(ctx, e, plat, &mode)
}

func startMode(e *env, args []string) (config.StartupMode, error) {
	fs := newFlagSet(e, "start", "[--mode audit|reconcile]")
	value := fs.String("mode", string(config.StartupAudit), "startup mode: audit or reconcile")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", fmt.Errorf("unexpected start argument %q", fs.Arg(0))
	}
	return config.ParseStartupMode(*value)
}
