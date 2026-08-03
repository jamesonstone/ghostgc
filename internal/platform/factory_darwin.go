//go:build darwin

package platform

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/platform/darwin"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// New returns the Platform implementation for the host operating system.
func New(opts Options) (Platform, error) {
	c, err := darwin.New(darwin.Options{EnvAllow: opts.EnvAllow})
	if err != nil {
		return nil, err
	}
	return &darwinPlatform{c: c}, nil
}

// darwinPlatform adapts the macOS collector to the Platform interface. The
// collector deliberately does not import this package, which is what lets the
// factory live here without an import cycle.
type darwinPlatform struct{ c *darwin.Collector }

func (p *darwinPlatform) Name() string    { return p.c.Name() }
func (p *darwinPlatform) SelfUID() uint32 { return p.c.SelfUID() }

func (p *darwinPlatform) SnapshotProcesses(ctx context.Context) (*process.Snapshot, error) {
	return p.c.SnapshotProcesses(ctx)
}

func (p *darwinPlatform) InspectProcess(ctx context.Context, pid int) (process.Process, error) {
	return p.c.InspectProcess(ctx, pid)
}

func (p *darwinPlatform) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	return p.c.SampleActivity(ctx, key, repositoryRoot)
}

// SignalProcess is refused by the collector and re-reported with this
// package's sentinel error so callers can match on it.
func (p *darwinPlatform) SignalProcess(ctx context.Context, pid int, sig Signal) error {
	if err := p.c.SignalProcess(ctx, pid, sig); err != nil {
		return ErrSignalingDisabled
	}
	// Unreachable: the collector never returns nil. Fail closed regardless.
	return ErrSignalingDisabled
}

func (p *darwinPlatform) InstallService(ctx context.Context, opts ServiceOptions) error {
	return p.c.InstallService(ctx, opts.Label, opts.BinaryPath, opts.ConfigPath, opts.LogDir)
}

func (p *darwinPlatform) UninstallService(ctx context.Context, label string) error {
	return p.c.UninstallService(ctx, label)
}

func (p *darwinPlatform) ServiceStatus(ctx context.Context, label string) (ServiceState, error) {
	installed, running, unitPath, pid, lastExit, err := p.c.ServiceStatus(ctx, label)
	return ServiceState{
		Installed: installed,
		Running:   running,
		Label:     label,
		UnitPath:  unitPath,
		PID:       pid,
		LastExit:  lastExit,
	}, err
}
