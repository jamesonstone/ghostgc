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

func (p *darwinPlatform) SignalProcess(ctx context.Context, key process.Key,
	executable process.ExecutableIdentity, sig Signal) error {
	if sig != SIGTERM {
		return ErrSignalNotAllowed
	}
	return p.c.SignalProcess(ctx, key, executable, sig)
}

func (p *darwinPlatform) InstallService(ctx context.Context, opts ServiceOptions) error {
	return p.c.InstallService(ctx, opts.Label, opts.BinaryPath, opts.Arguments, opts.LogDir)
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
