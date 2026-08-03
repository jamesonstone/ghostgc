package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
)

const daemonBinaryName = "ghostgcd"

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
	fs := newFlagSet(e, "service install", "[--binary <path>]")
	binary := fs.String("binary", "", "path to the ghostgcd binary")
	if err := fs.Parse(args); err != nil {
		return err
	}

	path, err := resolveDaemonBinary(*binary)
	if err != nil {
		return err
	}
	if err := e.paths.EnsureDirs(); err != nil {
		return err
	}
	if _, statErr := os.Stat(e.paths.Config); statErr != nil {
		if err := os.WriteFile(e.paths.Config, []byte(config.Example()), 0o600); err != nil {
			return err
		}
		fmt.Printf("Wrote a default audit-mode configuration to %s\n", e.paths.Config)
	}

	if err := plat.InstallService(ctx, platform.ServiceOptions{
		Label:      config.ServiceLabel,
		BinaryPath: path,
		ConfigPath: e.paths.Config,
		LogDir:     e.paths.LogDir,
		StateDir:   e.paths.StateDir,
	}); err != nil {
		return err
	}
	fmt.Printf("Installed %s\n", config.ServiceLabel)
	fmt.Printf("  binary: %s\n  config: %s\n  logs:   %s\n", path, e.paths.Config, e.paths.LogDir)
	fmt.Println("\nThe daemon starts at login and restarts only after an unsuccessful exit, with a 30 second throttle.")
	return nil
}

// resolveDaemonBinary finds ghostgcd next to the CLI, then on PATH.
func resolveDaemonBinary(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("daemon binary %s: %w", abs, err)
		}
		return abs, nil
	}
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), daemonBinaryName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if found, err := exec.LookPath(daemonBinaryName); err == nil {
		return filepath.Abs(found)
	}
	return "", fmt.Errorf("could not find the %s binary next to %s or on PATH; pass --binary", daemonBinaryName, "ghostgc")
}
