// Package platformtest provides a scripted Platform implementation for tests.
//
// It lives in a non-test package so that the daemon, session and storage tests
// can share one fake rather than each inventing its own process table.
package platformtest

import (
	"context"
	"errors"
	"sync"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/platform"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// Fake replays a scripted sequence of snapshots.
type Fake struct {
	mu        sync.Mutex
	snapshots []*process.Snapshot
	index     int

	// UID is the uid the fake claims to run as.
	UID uint32
	// Err, when set, is returned by the next SnapshotProcesses call.
	Err error
	// SignalAttempts counts how many times anything tried to signal a process.
	// A passing test suite must leave this at whatever the test itself did, and
	// no signal is ever actually delivered.
	SignalAttempts int

	installed bool
	running   bool
}

// New builds a Fake that replays the given snapshots in order, repeating the
// last one once the script is exhausted.
func New(uid uint32, snapshots ...*process.Snapshot) *Fake {
	return &Fake{UID: uid, snapshots: snapshots}
}

// Push appends a snapshot to the script.
func (f *Fake) Push(s *process.Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = append(f.snapshots, s)
}

// Name implements platform.Platform.
func (f *Fake) Name() string { return "fake" }

// SelfUID implements platform.Platform.
func (f *Fake) SelfUID() uint32 { return f.UID }

// SnapshotProcesses implements platform.Platform.
func (f *Fake) SnapshotProcesses(ctx context.Context) (*process.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		err := f.Err
		f.Err = nil
		return nil, err
	}
	if len(f.snapshots) == 0 {
		return nil, errors.New("platformtest: no snapshots were scripted")
	}
	s := f.snapshots[f.index]
	if f.index < len(f.snapshots)-1 {
		f.index++
	}
	return s, nil
}

// InspectProcess implements platform.Platform.
func (f *Fake) InspectProcess(ctx context.Context, pid int) (process.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snapshots) == 0 {
		return process.Process{}, errors.New("platformtest: no snapshots were scripted")
	}
	p, ok := f.snapshots[f.index].ByPID(pid)
	if !ok {
		return process.Process{}, errors.New("platformtest: no such process")
	}
	return p, nil
}

// SignalProcess implements platform.Platform and always refuses, exactly as
// the real implementations do in this delivery phase.
func (f *Fake) SignalProcess(ctx context.Context, pid int, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignalAttempts++
	return platform.ErrSignalingDisabled
}

// InstallService implements platform.Platform.
func (f *Fake) InstallService(ctx context.Context, opts platform.ServiceOptions) error {
	f.installed = true
	return nil
}

// UninstallService implements platform.Platform.
func (f *Fake) UninstallService(ctx context.Context, label string) error {
	f.installed = false
	return nil
}

// ServiceStatus implements platform.Platform.
func (f *Fake) ServiceStatus(ctx context.Context, label string) (platform.ServiceState, error) {
	return platform.ServiceState{Installed: f.installed, Running: f.running, Label: label}, nil
}

var _ platform.Platform = (*Fake)(nil)
