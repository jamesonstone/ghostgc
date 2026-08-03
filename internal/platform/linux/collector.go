//go:build linux

// Package linux is the placeholder for the /proc-based collector.
//
// Linux support is delivery phase 9. The package exists now so that the
// platform abstraction is exercised by the compiler on both targets and so
// that no darwin-specific type leaks into the daemon core. It must never
// pretend to have observed something it did not.
package linux

import (
	"context"
	"errors"
	"os"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// ErrNotImplemented is returned by every collection method.
var ErrNotImplemented = errors.New("linux: process collection arrives in delivery phase 9")

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
// support before delivery phase 9.
func (c *Collector) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	return process.ActivitySample{}, ErrNotImplemented
}

// SignalProcess implements the platform contract and always refuses.
func (c *Collector) SignalProcess(ctx context.Context, pid int, sig syscall.Signal) error {
	return errors.New("platform: process signalling is not implemented in this build; it is introduced in delivery phase 6, behind manual approval and full pre-action revalidation")
}
