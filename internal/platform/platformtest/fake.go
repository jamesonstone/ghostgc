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
	mu            sync.Mutex
	snapshots     []*process.Snapshot
	index         int
	last          *process.Snapshot
	activity      map[string][]process.ActivitySample
	activityIndex map[string]int
	activityCalls []process.Key

	// UID is the uid the fake claims to run as.
	UID uint32
	// Err, when set, is returned by the next SnapshotProcesses call.
	Err error
	// SignalAttempts counts how many times anything tried to signal a process.
	// A passing test suite must leave this at whatever the test itself did, and
	// no signal is ever actually delivered.
	SignalAttempts int
	Signals        []SignalAttempt
	SignalErr      error

	installed bool
	running   bool
	service   platform.ServiceOptions
}

// New builds a Fake that replays the given snapshots in order, repeating the
// last one once the script is exhausted.
func New(uid uint32, snapshots ...*process.Snapshot) *Fake {
	return &Fake{
		UID: uid, snapshots: snapshots,
		activity:      make(map[string][]process.ActivitySample),
		activityIndex: make(map[string]int),
	}
}

// SetActivity scripts targeted samples for one exact process key.
func (f *Fake) SetActivity(key process.Key, samples ...process.ActivitySample) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activity[key.UID()] = append([]process.ActivitySample(nil), samples...)
}

// ActivityCalls returns the exact keys selected for expensive inspection.
func (f *Fake) ActivityCalls() []process.Key {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]process.Key(nil), f.activityCalls...)
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
	f.last = s
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
	snap := f.last
	if snap == nil {
		snap = f.snapshots[f.index]
	}
	p, ok := snap.ByPID(pid)
	if !ok {
		return process.Process{}, errors.New("platformtest: no such process")
	}
	return p, nil
}

// SampleActivity implements targeted activity inspection and validates the
// exact process key before returning scripted evidence.
func (f *Fake) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.last == nil {
		return process.ActivitySample{}, errors.New("platformtest: no process snapshot has been observed")
	}
	p, ok := f.last.ByKey(key)
	if !ok {
		return process.ActivitySample{}, errors.New("platformtest: activity target changed or exited")
	}
	f.activityCalls = append(f.activityCalls, key)
	if scripted := f.activity[key.UID()]; len(scripted) > 0 {
		i := f.activityIndex[key.UID()]
		if i >= len(scripted) {
			i = len(scripted) - 1
		}
		f.activityIndex[key.UID()]++
		return scripted[i], nil
	}
	return process.ActivitySample{
		Key: key, Taken: f.last.Taken, CPUTime: p.CPUTime,
		CPUKnown: p.Detailed, RSSBytes: p.RSSBytes,
		Note: "file, socket and I/O activity was not scripted",
	}, nil
}

// SignalAttempt records a test-only exact-key signal request.
type SignalAttempt struct {
	Key process.Key
	Sig syscall.Signal
}

// SignalProcess validates the scripted exact key and accepts SIGTERM only.
func (f *Fake) SignalProcess(ctx context.Context, key process.Key,
	executable process.ExecutableIdentity, sig syscall.Signal) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SignalAttempts++
	if sig != platform.SIGTERM {
		return platform.ErrSignalNotAllowed
	}
	if f.last == nil {
		return errors.New("platformtest: no process snapshot has been observed")
	}
	observed, ok := f.last.ByKey(key)
	if !ok {
		return errors.New("platformtest: signal target changed or exited")
	}
	identity, ok := observed.Executable()
	if !ok || identity != executable {
		return errors.New("platformtest: signal target executable changed or is unavailable")
	}
	f.Signals = append(f.Signals, SignalAttempt{Key: key, Sig: sig})
	return f.SignalErr
}

// InstallService implements platform.Platform.
func (f *Fake) InstallService(ctx context.Context, opts platform.ServiceOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed = true
	f.service = opts
	return nil
}

// UninstallService implements platform.Platform.
func (f *Fake) UninstallService(ctx context.Context, label string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installed = false
	return nil
}

// ServiceStatus implements platform.Platform.
func (f *Fake) ServiceStatus(ctx context.Context, label string) (platform.ServiceState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return platform.ServiceState{Installed: f.installed, Running: f.running, Label: label}, nil
}

// InstalledService returns the most recently registered service options.
func (f *Fake) InstalledService() platform.ServiceOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	opts := f.service
	opts.Arguments = append([]string(nil), opts.Arguments...)
	return opts
}

var _ platform.Platform = (*Fake)(nil)
