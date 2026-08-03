// Package daemon runs the observation loop.
//
// Each cycle observes, reconciles, classifies, evaluates audit-only policies
// and persists one transaction. It cannot act: no signal path exists.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/adapters/codex"
	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/classification"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/repository"
	"github.com/jamesonstone/ghostgc/internal/sessions"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/version"
)

// Audit kinds emitted by the daemon itself.
const (
	AuditDaemonStarted   = "daemon.started"
	AuditDaemonStopped   = "daemon.stopped"
	AuditScanFailed      = "scan.failed"
	AuditRetention       = "retention.compacted"
	AuditStorageRecovery = "storage.recovered"
)

// Options configures a Daemon.
type Options struct {
	Config   config.Config
	Paths    config.Paths
	Store    *storage.Store
	Platform platform.Platform
	Logger   *slog.Logger
	// Registry may be nil, in which case adapters are built from the config.
	Registry *adapters.Registry
}

// Daemon is the long-running observer.
type Daemon struct {
	cfg    config.Config
	paths  config.Paths
	store  *storage.Store
	plat   platform.Platform
	log    *slog.Logger
	reg    *adapters.Registry
	recon  *sessions.Reconciler
	repos  *repository.Finder
	selfPI int

	startedAt time.Time

	mu                     sync.RWMutex
	snapshot               *process.Snapshot
	tree                   *process.Tree
	last                   *sessions.Result
	degraded               []string
	metrics                metrics
	lastActivityAt         time.Time
	activityBaseline       map[string]process.ActivitySample
	classificationPrevious map[string]classification.Previous
	lastClassificationAt   time.Time
	lastPolicyAt           time.Time
}

type metrics struct {
	scanCount          int64
	scanFailures       int64
	lastScanDuration   time.Duration
	totalScanDuration  time.Duration
	maxScanDuration    time.Duration
	lastReconcile      time.Duration
	lastPersist        time.Duration
	lastActivity       time.Duration
	activitySamples    int64
	classifications    int64
	policyDecisions    int64
	retentionRuns      int64
	lastRetentionRows  int64
	visibleProcesses   int
	inspectedProcesses int
	attributed         int
}

// BuildRegistry constructs the adapter registry from configuration.
func BuildRegistry(cfg config.Config, repos *repository.Finder) *adapters.Registry {
	var list []adapters.AgentAdapter
	for id, agent := range cfg.Agents {
		if !agent.Enabled {
			continue
		}
		switch id {
		case codex.ID:
			list = append(list, codex.New(repos))
		default:
			// An unknown adapter id is a configuration statement the daemon
			// cannot honour. It is reported by `ghostgc doctor` rather than
			// silently ignored, but it must not stop observation.
		}
	}
	return adapters.NewRegistry(list...)
}

// New constructs a Daemon.
func New(opts Options) (*Daemon, error) {
	if opts.Store == nil {
		return nil, errors.New("daemon: a store is required")
	}
	if opts.Platform == nil {
		return nil, errors.New("daemon: a platform is required")
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	repos := repository.NewFinder()
	reg := opts.Registry
	if reg == nil {
		reg = BuildRegistry(opts.Config, repos)
	}

	d := &Daemon{
		cfg:                    opts.Config,
		paths:                  opts.Paths,
		store:                  opts.Store,
		plat:                   opts.Platform,
		log:                    log,
		reg:                    reg,
		repos:                  repos,
		selfPI:                 os.Getpid(),
		startedAt:              time.Now(),
		activityBaseline:       make(map[string]process.ActivitySample),
		classificationPrevious: make(map[string]classification.Previous),
	}
	d.recon = sessions.New(reg, d.selfPI, opts.Platform.SelfUID(), repos)
	return d, nil
}

// AdapterEnvKeys returns the environment variables the enabled adapters need.
// The daemon binary calls this before constructing the platform so that the
// collector extracts nothing else.
func AdapterEnvKeys(cfg config.Config) []string {
	return BuildRegistry(cfg, repository.NewFinder()).EnvKeys()
}

// Run starts the API server and the observation loop and blocks until ctx is
// cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.seed(ctx); err != nil {
		return err
	}

	server := &api.Server{Backend: d, SocketPath: d.paths.Socket, Logger: d.log}
	if err := server.Listen(); err != nil {
		return err
	}

	d.audit(ctx, AuditDaemonStarted, "daemon",
		fmt.Sprintf("ghostgc %s started in %s mode on %s (pid %d); delivery phase %s",
			version.String(), d.cfg.GlobalMode, d.plat.Name(), d.selfPI, version.Phase))
	d.log.Info("daemon started",
		"version", version.String(),
		"mode", string(d.cfg.GlobalMode),
		"socket", d.paths.Socket,
		"database", d.store.Path(),
		"agents", d.agentIDs(),
		"signalling_enabled", false,
	)

	var wg sync.WaitGroup
	wg.Add(1)
	serveErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		serveErr <- server.Serve(ctx)
	}()

	// The first scan runs immediately so that `ghostgc status` is useful the
	// moment the daemon is up.
	d.runScan(ctx)

	scanTicker := time.NewTicker(d.cfg.Sampling.ProcessScan.D())
	defer scanTicker.Stop()
	retentionTicker := time.NewTicker(d.cfg.Sampling.Retention.D())
	defer retentionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.audit(context.WithoutCancel(ctx), AuditDaemonStopped, "daemon", "daemon stopping")
			wg.Wait()
			return <-serveErr
		case <-scanTicker.C:
			d.runScan(ctx)
		case <-retentionTicker.C:
			d.runRetention(ctx)
		case err := <-serveErr:
			return err
		}
	}
}

func (d *Daemon) seed(ctx context.Context) error {
	sessionRecs, err := d.store.ListSessions(ctx, storage.SessionFilter{})
	if err != nil {
		return err
	}
	ownership, err := d.store.LiveOwnership(ctx)
	if err != nil {
		return err
	}
	d.recon.Seed(sessionRecs, ownership)
	d.log.Info("restored state",
		"sessions", len(sessionRecs),
		"recorded_ownership", len(ownership),
	)
	return nil
}

func (d *Daemon) agentIDs() []string {
	out := make([]string, 0, len(d.reg.All()))
	for _, a := range d.reg.All() {
		out = append(out, a.ID())
	}
	return out
}
