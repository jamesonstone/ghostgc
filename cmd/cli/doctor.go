package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// cmdDoctor runs the checks that do not need a daemon, then merges in the
// daemon's own checks when it is reachable. The command a user runs to find
// out why nothing works must itself work when nothing works.
func cmdDoctor(ctx context.Context, e *env, args []string) error {
	resp := api.DoctorResponse{OK: true, Checks: localChecks(ctx, e)}

	if daemonResp, err := e.api().Doctor(ctx); err == nil {
		resp.Checks = append(resp.Checks, daemonResp.Checks...)
	} else {
		resp.Checks = append(resp.Checks, api.DoctorCheck{
			Name:   "daemon",
			Status: api.CheckWarn,
			Detail: "the daemon is not reachable on " + e.socket,
			Remedy: "run `ghostgc service install`, or start `ghostgcd` in the foreground to see why it exits",
		})
	}

	for _, c := range resp.Checks {
		if c.Status == api.CheckError {
			resp.OK = false
		}
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderDoctor(resp)
	if !resp.OK {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

func localChecks(ctx context.Context, e *env) []api.DoctorCheck {
	var checks []api.DoctorCheck
	add := func(name, status, detail, remedy string) {
		checks = append(checks, api.DoctorCheck{Name: name, Status: status, Detail: detail, Remedy: remedy})
	}

	cfg, err := config.Load(e.paths.Config)
	switch {
	case err != nil:
		add("config-file", api.CheckError, err.Error(), "correct "+e.paths.Config+", or delete it to fall back to the built-in defaults")
	case cfg.Defaulted:
		add("config-file", api.CheckWarn, "no configuration file at "+e.paths.Config+"; built-in defaults are in use",
			"run `ghostgc config init`")
	default:
		add("config-file", api.CheckOK, e.paths.Config, "")
	}

	for _, dir := range []struct{ name, path string }{
		{"state-directory", e.paths.StateDir},
		{"log-directory", e.paths.LogDir},
	} {
		fi, err := os.Stat(dir.path)
		switch {
		case err != nil:
			add(dir.name, api.CheckWarn, dir.path+" does not exist yet", "it is created when the daemon first starts")
		case !fi.IsDir():
			add(dir.name, api.CheckError, dir.path+" is not a directory", "remove the file at that path")
		default:
			add(dir.name, api.CheckOK, dir.path, "")
		}
	}

	if err := e.paths.Validate(); err != nil {
		add("socket-path", api.CheckError, err.Error(), "set paths.socket to something shorter in the configuration")
	} else if _, err := os.Stat(e.paths.Socket); err != nil {
		add("socket", api.CheckWarn, "no socket at "+e.paths.Socket, "the daemon creates it at startup")
	} else {
		add("socket", api.CheckOK, e.paths.Socket, "")
	}

	if _, err := os.Stat(e.paths.Database); err != nil {
		add("database-file", api.CheckWarn, "no database at "+e.paths.Database, "it is created when the daemon first starts")
	} else {
		add("database-file", api.CheckOK, e.paths.Database, "")
	}

	if os.Geteuid() == 0 {
		add("privileges", api.CheckWarn, "running as root",
			"ghostgc is designed to run unprivileged and to observe only the current user's processes")
	} else {
		add("privileges", api.CheckOK, fmt.Sprintf("running unprivileged as uid %d, which is all ghostgc needs", os.Geteuid()), "")
	}

	if plat, err := platform.New(platform.Options{}); err == nil {
		invalid := process.Key{PID: os.Getpid(), StartTimeNs: 1}
		if err := plat.SignalProcess(ctx, invalid, platform.Signal(-1)); err != nil {
			add("signal-safety-gate", api.CheckOK, "non-TERM signals are rejected before any system call", "")
		} else {
			add("signal-safety-gate", api.CheckError, "the platform accepted a non-TERM signal", "do not use this build; rebuild from a trusted source tree")
		}
	}

	return checks
}
