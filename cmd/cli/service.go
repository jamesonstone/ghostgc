package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
)

var executablePath = os.Executable

func cmdService(ctx context.Context, e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ghostgc service install|uninstall|status")
	}
	plat, err := platform.New(platform.Options{})
	if err != nil {
		return err
	}

	switch args[0] {
	case "install":
		return serviceInstall(ctx, e, plat, args[1:])
	case "uninstall":
		if err := plat.UninstallService(ctx, config.ServiceLabel); err != nil {
			return err
		}
		fmt.Printf("Removed %s\n", config.ServiceLabel)
		return nil
	case "status":
		state, err := plat.ServiceStatus(ctx, config.ServiceLabel)
		if err != nil {
			return err
		}
		if e.jsonOut {
			return emitJSON(state)
		}
		fmt.Printf("Label: %s\n", state.Label)
		fmt.Printf("Installed: %t\n", state.Installed)
		fmt.Printf("Running: %t\n", state.Running)
		if state.UnitPath != "" {
			fmt.Printf("Unit: %s\n", state.UnitPath)
		}
		if state.PID > 0 {
			fmt.Printf("PID: %d\n", state.PID)
		}
		if state.LastExit != 0 {
			fmt.Printf("Last exit status: %d\n", state.LastExit)
		}
		if state.Description != "" {
			fmt.Printf("Note: %s\n", state.Description)
		}
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q; expected install, uninstall or status", args[0])
	}
}

func serviceInstall(ctx context.Context, e *env, plat platform.Platform, args []string) error {
	fs := newFlagSet(e, "service install", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected service install argument %q", fs.Arg(0))
	}
	return installBackground(ctx, e, plat, nil)
}

func installBackground(ctx context.Context, e *env, plat platform.Platform, startupMode *config.StartupMode) error {
	path, err := resolveSelfBinary()
	if err != nil {
		return err
	}
	var cfg config.Config
	arguments := []string{"daemon", "--config", e.paths.Config}
	if startupMode == nil {
		cfg, err = config.Load(e.paths.Config)
	} else {
		cfg, err = config.LoadForStartup(e.paths.Config, *startupMode)
		arguments = []string{"daemon", "--mode", string(*startupMode), "--config", e.paths.Config}
	}
	if err != nil {
		return err
	}
	paths := cfg.ResolvePaths(e.paths)
	if err := paths.Validate(); err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	arguments[len(arguments)-1] = paths.Config

	if err := plat.InstallService(ctx, platform.ServiceOptions{
		Label:      config.ServiceLabel,
		BinaryPath: path,
		Arguments:  arguments,
		LogDir:     paths.LogDir,
		StateDir:   paths.StateDir,
	}); err != nil {
		return err
	}
	legacy, err := retireLegacyDaemonSibling()
	if err != nil {
		return fmt.Errorf("service migrated but legacy executable remains: %w", err)
	}
	if startupMode != nil && string(cfg.GlobalMode) != string(*startupMode) {
		fmt.Printf("Started ghostgc with the %s ceiling; effective global mode is %s.\n", *startupMode, cfg.GlobalMode)
	} else {
		fmt.Printf("Started ghostgc in %s mode.\n", cfg.GlobalMode)
	}
	fmt.Printf("  binary: %s\n  command: %s %s\n  logs:   %s\n",
		path, path, strings.Join(arguments, " "), paths.LogDir)
	if cfg.Defaulted {
		fmt.Printf("  config: built-in Codex defaults; optional overrides: %s\n", paths.Config)
	} else {
		fmt.Printf("  config: %s\n", paths.Config)
	}
	if legacy != "" {
		fmt.Printf("  removed legacy executable: %s\n", legacy)
	}
	fmt.Println("\nThe background service starts now and at login. Run `ghostgc status` to inspect it.")
	return nil
}

func retireLegacyDaemonSibling() (string, error) {
	self, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve ghostgc executable for migration: %w", err)
	}
	abs, err := filepath.Abs(self)
	if err != nil {
		return "", fmt.Errorf("resolve ghostgc executable %s for migration: %w", self, err)
	}
	if filepath.Base(abs) != "ghostgc" {
		return "", nil
	}
	legacy := filepath.Join(filepath.Dir(abs), "ghostgcd")
	info, err := os.Lstat(legacy)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect legacy executable %s: %w", legacy, err)
	}
	if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("refusing to remove non-file legacy path %s", legacy)
	}
	if err := os.Remove(legacy); err != nil {
		return "", fmt.Errorf("remove legacy executable %s: %w", legacy, err)
	}
	return legacy, nil
}

// resolveSelfBinary returns the canonical path to the running ghostgc artifact.
func resolveSelfBinary() (string, error) {
	self, err := executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve ghostgc executable: %w", err)
	}
	abs, err := filepath.Abs(self)
	if err != nil {
		return "", fmt.Errorf("resolve ghostgc executable %s: %w", self, err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve ghostgc executable %s: %w", abs, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect ghostgc executable %s: %w", canonical, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("ghostgc executable %s is not an executable regular file", canonical)
	}
	return canonical, nil
}
