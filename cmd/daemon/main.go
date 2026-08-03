// Command ghostgcd is the ghostgc observation daemon.
//
// It runs one persistent process with bounded internal sampling. The sole
// action path is an exact, manually approved and freshly revalidated SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/daemon"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghostgcd:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath  = flag.String("config", "", "path to config.yaml (default: ~/.config/ghostgc/config.yaml)")
		logLevel    = flag.String("log-level", "info", "log level: debug, info, warn, error")
		showVersion = flag.Bool("version", false, "print version and exit")
		once        = flag.Bool("once", false, "run a single observation cycle and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghostgcd %s (delivery phase %s)\n", version.String(), version.Phase)
		return nil
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	if *configPath != "" {
		paths.Config = *configPath
	}

	cfg, err := config.Load(paths.Config)
	if err != nil {
		return err
	}
	paths = cfg.ResolvePaths(paths)
	if err := paths.Validate(); err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	log := newLogger(*logLevel)

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
			TsNs:    nowNs(),
			Kind:    daemon.AuditStorageRecovery,
			Subject: "daemon",
			Summary: recovered.Error(),
		})
	}

	plat, err := platform.New(platform.Options{EnvAllow: daemon.AdapterEnvKeys(cfg)})
	if err != nil {
		return err
	}

	d, err := daemon.New(daemon.Options{
		Config:   cfg,
		Paths:    paths,
		Store:    store,
		Platform: plat,
		Logger:   log,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	// Structured JSON internally, as required; the CLI renders human-readable
	// output from the API rather than from these logs.
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

func nowNs() int64 { return time.Now().UnixNano() }
