//go:build linux

// Package linux is the placeholder for the /proc-based collector. The package
// keeps the platform abstraction compiling on both targets without allowing a
// darwin-specific type to leak into the daemon core. It must never pretend to
// have observed something it did not.
package linux

import (
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// ErrNotImplemented is returned by every collection method.
var ErrNotImplemented = errors.New("linux: process collection is not implemented")

// Options mirrors the darwin collector options.
type Options struct {
	EnvAllow []string
}

// Collector is the /proc collector.
type Collector struct{ uid uint32 }

// New constructs the Linux collector.
func New(opts Options) (*Collector, error) {
	return &Collector{uid: uint32(os.Getuid())}, nil
}

// Name implements the platform contract.
func (c *Collector) Name() string { return "linux" }

// SelfUID implements the platform contract.
func (c *Collector) SelfUID() uint32 { return c.uid }

// SnapshotProcesses implements the platform contract.
func (c *Collector) SnapshotProcesses(ctx context.Context) (*process.Snapshot, error) {
	return nil, ErrNotImplemented
}

// InspectProcess implements the platform contract.
func (c *Collector) InspectProcess(ctx context.Context, pid int) (process.Process, error) {
	return process.Process{}, ErrNotImplemented
}

// SampleActivity implements the platform contract without fabricating Linux
// support.
func (c *Collector) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	return process.ActivitySample{}, ErrNotImplemented
}

// InspectPathUsage refuses removal authority on Linux.
func (c *Collector) InspectPathUsage(ctx context.Context, canonicalPath string) (process.PathUsage, error) {
	return process.PathUsage{}, ErrNotImplemented
}

// SignalProcess implements the platform contract and always refuses.
func (c *Collector) SignalProcess(ctx context.Context, key process.Key,
	executable process.ExecutableIdentity, sig syscall.Signal) error {
	return errors.New("platform: process signalling is not available on linux")
}
