//go:build linux

package platform

import (
	"context"

	"github.com/jamesonstone/ghostgc/internal/platform/linux"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// New returns the Platform implementation for the host operating system.
func New(opts Options) (Platform, error) {
	c, err := linux.New(linux.Options{EnvAllow: opts.EnvAllow})
	if err != nil {
		return nil, err
	}
	return &linuxPlatform{c: c}, nil
}

type linuxPlatform struct{ c *linux.Collector }

func (p *linuxPlatform) Name() string    { return p.c.Name() }
func (p *linuxPlatform) SelfUID() uint32 { return p.c.SelfUID() }

func (p *linuxPlatform) SnapshotProcesses(ctx context.Context) (*process.Snapshot, error) {
	return p.c.SnapshotProcesses(ctx)
}

func (p *linuxPlatform) InspectProcess(ctx context.Context, pid int) (process.Process, error) {
	return p.c.InspectProcess(ctx, pid)
}

func (p *linuxPlatform) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	return p.c.SampleActivity(ctx, key, repositoryRoot)
}

func (p *linuxPlatform) SignalProcess(ctx context.Context, pid int, sig Signal) error {
	return ErrSignalingDisabled
}

func (p *linuxPlatform) InstallService(ctx context.Context, opts ServiceOptions) error {
	return ErrNotSupported
}

func (p *linuxPlatform) UninstallService(ctx context.Context, label string) error {
	return ErrNotSupported
}

func (p *linuxPlatform) ServiceStatus(ctx context.Context, label string) (ServiceState, error) {
	return ServiceState{Label: label, Description: "linux service management arrives in delivery phase 9"}, nil
}
