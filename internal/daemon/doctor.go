package daemon

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters/codex"
	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// knownAdapters is the set of adapter identifiers this build can construct.
var knownAdapters = map[string]bool{codex.ID: true}

// Doctor implements api.Backend.
func (d *Daemon) Doctor(ctx context.Context) (api.DoctorResponse, error) {
	var checks []api.DoctorCheck
	add := func(name, status, detail, remedy string) {
		checks = append(checks, api.DoctorCheck{Name: name, Status: status, Detail: detail, Remedy: remedy})
	}

	invalid := process.Key{PID: d.selfPI, StartTimeNs: 1}
	invalidExecutable := process.ExecutableIdentity{ExecPath: "/invalid", Comm: "invalid"}
	if err := d.plat.SignalProcess(ctx, invalid, invalidExecutable, platform.Signal(-1)); err == nil {
		add("signal-safety-gate", api.CheckError, "the platform accepted a non-TERM signal", "stop the daemon and rebuild from a trusted source tree")
	} else if err := d.plat.SignalProcess(ctx, invalid, invalidExecutable, platform.SIGTERM); err == nil {
		add("signal-safety-gate", api.CheckError, "the platform accepted a changed exact process identity", "stop the daemon and rebuild from a trusted source tree")
	} else {
		add("signal-safety-gate", api.CheckOK, "non-TERM signals and changed exact process identities are rejected", "")
	}

	if d.automaticCleanupEnabled() {
		add("global-mode", api.CheckOK, "global enforce and one narrow automatic policy are enabled", "")
	} else {
		add("global-mode", api.CheckOK, fmt.Sprintf("global mode is %q; automatic cleanup is disabled", d.cfg.GlobalMode), "")
	}

	if d.cfg.Defaulted {
		add("configuration", api.CheckWarn,
			"no configuration file was found; built-in defaults are in use",
			"run `ghostgc config init` to write "+d.cfg.SourcePath)
	} else {
		add("configuration", api.CheckOK, "loaded from "+d.cfg.SourcePath, "")
	}

	if d.cfg.Privacy.StoreSourceContents {
		add("privacy", api.CheckError, "privacy.storeSourceContents is enabled", "set it to false; ghostgc never reads source contents")
	} else {
		add("privacy", api.CheckOK, "source contents are never read; command lines and environment values are redacted before storage", "")
	}

	for id, agent := range d.cfg.Agents {
		if !agent.Enabled {
			continue
		}
		if !knownAdapters[id] {
			add("adapter:"+id, api.CheckWarn,
				fmt.Sprintf("configuration enables agent %q, for which this build has no adapter", id),
				"remove the entry, or wait for delivery phase 8 which adds Claude Code, Cursor and OpenCode adapters")
		}
	}
	if len(d.reg.All()) == 0 {
		add("adapters", api.CheckError, "no agent adapters are enabled, so no session can ever be detected",
			"enable at least one agent in "+d.cfg.SourcePath)
	} else {
		add("adapters", api.CheckOK, fmt.Sprintf("enabled: %v", d.agentIDs()), "")
	}

	if err := d.paths.Validate(); err != nil {
		add("socket-path", api.CheckError, err.Error(), "set paths.socket in the configuration to a shorter path")
	} else {
		add("socket-path", api.CheckOK, d.paths.Socket, "")
	}
	if fi, err := os.Stat(d.paths.Socket); err == nil {
		if perm := fi.Mode().Perm(); perm != 0o600 {
			add("socket-permissions", api.CheckWarn, fmt.Sprintf("socket mode is %o, expected 600", perm),
				"remove the socket and restart the daemon")
		} else {
			add("socket-permissions", api.CheckOK, "owner-only (600)", "")
		}
	}

	size := d.store.SizeBytes()
	switch {
	case d.cfg.Retention.MaxDatabaseBytes > 0 && size > d.cfg.Retention.MaxDatabaseBytes:
		add("database-size", api.CheckWarn,
			fmt.Sprintf("database is %.1f MiB, above the %.1f MiB budget", mib(size), mib(d.cfg.Retention.MaxDatabaseBytes)),
			"the next retention pass will compact aggressively; lower retention windows if it recurs")
	default:
		add("database-size", api.CheckOK, fmt.Sprintf("%.1f MiB at %s", mib(size), d.store.Path()), "")
	}

	if counts, err := d.store.Counts(ctx); err == nil {
		add("database", api.CheckOK, fmt.Sprintf(
			"%d sessions, %d processes, %d observations, %d relationships, %d audit entries",
			counts.Sessions, counts.Processes, counts.Observations, counts.Relationships, counts.AuditEntries), "")

		// A session with no edges means attribution is happening but the graph
		// that explains it is not being written, which would leave `explain`
		// unable to say why anything belongs.
		switch {
		case counts.Sessions == 0:
			add("session-graph", api.CheckOK, "no sessions observed yet, so no relationships to record", "")
		case counts.Relationships == 0:
			add("session-graph", api.CheckWarn,
				fmt.Sprintf("%d session(s) recorded but no relationships", counts.Sessions),
				"attribution is being stored without the graph that explains it; check the daemon log for persist errors")
		default:
			add("session-graph", api.CheckOK,
				fmt.Sprintf("%d relationship(s) across %d session(s)", counts.Relationships, counts.Sessions), "")
		}
	} else {
		add("database", api.CheckError, err.Error(), "check that the state directory is writable")
	}

	if v, err := d.store.GetMeta(ctx, "schema_version"); err == nil {
		add("schema", api.CheckOK, "database is at schema version "+v, "")
	} else {
		add("schema", api.CheckWarn, "could not read the schema version: "+err.Error(), "")
	}

	d.mu.RLock()
	snap := d.snapshot
	m := d.metrics
	degraded := append([]string(nil), d.degraded...)
	d.mu.RUnlock()

	if snap == nil {
		add("observation", api.CheckWarn, "no snapshot has completed yet", "wait for one scan interval")
	} else {
		age := time.Since(snap.Taken)
		status := api.CheckOK
		remedy := ""
		if age > 3*d.cfg.Sampling.ProcessScan.D() {
			status = api.CheckWarn
			remedy = "check the daemon log for scan failures"
		}
		add("observation", status, fmt.Sprintf(
			"last snapshot %s ago: %d processes visible, %d inspected, %d attributed",
			age.Truncate(time.Second), snap.TotalCount, snap.Len(), m.attributed), remedy)

		if _, ok := snap.ByPID(d.selfPI); ok {
			add("self-visibility", api.CheckOK, "the daemon can see its own process, so process inspection is working", "")
		} else {
			add("self-visibility", api.CheckError, "the daemon cannot see its own process in the snapshot",
				"process inspection is not returning usable data; check the daemon log")
		}
	}

	target := 250 * time.Millisecond
	if m.scanCount > 0 {
		status := api.CheckOK
		remedy := ""
		if m.maxScanDuration > target {
			status = api.CheckWarn
			remedy = "a slow scan is usually transient system load; investigate if it persists"
		}
		add("scan-duration", status, fmt.Sprintf("last %s, max %s, target under %s",
			m.lastScanDuration.Truncate(time.Millisecond), m.maxScanDuration.Truncate(time.Millisecond), target), remedy)
	}
	if m.scanFailures > 0 {
		add("scan-failures", api.CheckWarn, fmt.Sprintf("%d scan(s) failed and were skipped", m.scanFailures),
			"failures are recorded in the audit log; run `ghostgc logs --kind scan.failed`")
	}
	for _, reason := range degraded {
		add("degraded", api.CheckWarn, reason, "")
	}

	if state, err := d.plat.ServiceStatus(ctx, config.ServiceLabel); err == nil {
		switch {
		case state.Running:
			add("service", api.CheckOK, fmt.Sprintf("%s is loaded and running as pid %d", state.Label, state.PID), "")
		case state.Installed:
			add("service", api.CheckWarn, fmt.Sprintf("%s is installed at %s but not running", state.Label, state.UnitPath),
				"run `ghostgc service install` to reload it")
		default:
			add("service", api.CheckWarn, "the daemon is not registered with the platform service manager",
				"run `ghostgc service install` so the daemon starts at login")
		}
	}

	ok := true
	for _, c := range checks {
		if c.Status == api.CheckError {
			ok = false
		}
	}
	return api.DoctorResponse{Checks: checks, OK: ok}, nil
}

func mib(b int64) float64 { return float64(b) / (1 << 20) }
