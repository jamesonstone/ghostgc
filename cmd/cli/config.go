package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jamesonstone/ghostgc/internal/config"
)

func cmdConfig(ctx context.Context, e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ghostgc config init|path|show")
	}
	switch args[0] {
	case "path":
		fmt.Println(e.paths.Config)
		return nil
	case "init":
		return configInit(e, args[1:])
	case "show":
		return configShow(e)
	default:
		return fmt.Errorf("unknown subcommand %q; expected init, path or show", args[0])
	}
}

func configInit(e *env, args []string) error {
	fs := newFlagSet(e, "config init", "[--force]")
	force := fs.Bool("force", false, "overwrite an existing configuration file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(e.paths.Config); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass --force to overwrite it", e.paths.Config)
	}
	if err := os.MkdirAll(filepath.Dir(e.paths.Config), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(e.paths.Config, []byte(config.Example()), 0o600); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", e.paths.Config)
	fmt.Println("The generated configuration is in audit mode, which is the only mode this build accepts.")
	return nil
}

func configShow(e *env) error {
	cfg, err := config.Load(e.paths.Config)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(cfg)
	}
	source := cfg.SourcePath
	if cfg.Defaulted {
		source += " (not present; built-in defaults in use)"
	}
	fmt.Printf("Source: %s\n", source)
	fmt.Printf("Global mode: %s\n", cfg.GlobalMode)
	fmt.Printf("Agents: %v\n", cfg.EnabledAgents())
	fmt.Printf("Process scan: %s\n", cfg.Sampling.ProcessScan.D())
	fmt.Printf("Retention: raw %s, aggregated %s, actions %s, ceiling %s\n",
		cfg.Retention.RawObservations.D(), cfg.Retention.AggregatedObservations.D(),
		cfg.Retention.Actions.D(), humanBytes(uint64(cfg.Retention.MaxDatabaseBytes)))
	fmt.Printf("Privacy: storeCommandLines=%t redactEnvironmentValues=%t storeSourceContents=%t networkTelemetry=%t\n",
		cfg.Privacy.StoreCommandLines, cfg.Privacy.RedactEnvironmentValues,
		cfg.Privacy.StoreSourceContents, cfg.Privacy.NetworkTelemetry)
	fmt.Printf("\nState directory: %s\nDatabase: %s\nSocket: %s\nLogs: %s\n",
		e.paths.StateDir, e.paths.Database, e.paths.Socket, e.paths.LogDir)
	return nil
}
