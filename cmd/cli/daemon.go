package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/daemon"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/version"
)

// cmdDaemon runs the persistent observation process from the same executable
// as the short-lived control commands.
func cmdDaemon(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "daemon", "[--config <path>] [--log-level <level>] [--once] [--version]")
	configPath := fs.String("config", "", "path to config.yaml")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	showVersion := fs.Bool("version", false, "print version and exit")
	once := fs.Bool("once", false, "run a single observation cycle and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected daemon argument %q", fs.Arg(0))
	}
	if *showVersion {
		fmt.Printf("ghostgc %s (delivery phase %s)\n", version.String(), version.Phase)
		return nil
	}

	paths, err := daemonPaths(e, *configPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return err
	}
	if *configPath != "" {
		paths = cfg.ResolvePaths(paths)
	}
	if err := paths.Validate(); err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	log := daemonLogger(*logLevel)
	store, err := storage.Open(paths.Database)
	var recovered *storage.ErrRecovered
	switch {
	case errors.As(err, &recovered):
		log.Warn("recovered from an unusable database",
			"moved_to", recovered.MovedTo, "cause", recovered.Cause)
	case err != nil:
		return err
	}
	defer func() { _ = store.Close() }()

	if recovered != nil {
		_ = store.AppendAudit(context.Background(), storage.AuditRecord{
			TsNs: time.Now().UnixNano(), Kind: daemon.AuditStorageRecovery,
			Subject: "daemon", Summary: recovered.Error(),
		})
	}

	plat, err := platform.New(platform.Options{EnvAllow: daemon.AdapterEnvKeys(cfg)})
	if err != nil {
		return err
	}
	d, err := daemon.New(daemon.Options{
		Config: cfg, Paths: paths, Store: store, Platform: plat, Logger: log,
	})
	if err != nil {
		return err
	}
	if *once {
		d.ScanNow(ctx)
		log.Info("single observation cycle complete")
		return nil
	}
	if err := d.Run(ctx); err != nil {
		if errors.Is(err, api.ErrAlreadyRunning) {
			return fmt.Errorf("%w at %s; stop it before starting another", err, paths.Socket)
		}
		return err
	}
	log.Info("daemon stopped")
	return nil
}

func daemonPaths(e *env, override string) (config.Paths, error) {
	if override == "" {
		return e.paths, nil
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return config.Paths{}, err
	}
	paths.Config = override
	return paths, nil
}

func daemonLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
