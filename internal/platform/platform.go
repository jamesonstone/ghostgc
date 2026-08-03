// Package platform isolates every operating-system-specific operation behind
// one interface so the daemon core stays portable.
package platform

import (
	"context"
	"errors"
	"syscall"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// Signal is an operating-system signal number.
type Signal = syscall.Signal

// SIGTERM is the only signal ghostgc can send.
const SIGTERM = syscall.SIGTERM

// ErrSignalingDisabled reports that the current platform has no signal support.
var ErrSignalingDisabled = errors.New("platform: process signalling is not available on this platform")

// ErrSignalNotAllowed rejects every signal except SIGTERM.
var ErrSignalNotAllowed = errors.New("platform: only SIGTERM is allowed")

// ErrNotSupported is returned for operations the current platform cannot
// perform.
var ErrNotSupported = errors.New("platform: operation not supported on this platform")

// Options configures the platform implementation.
type Options struct {
	// EnvAllow is the union of environment variable names that agent adapters
	// need for attribution. Variables outside this list are never copied out
	// of the kernel buffer, which keeps observation cost independent of how
	// large the observed environments are.
	EnvAllow []string
}

// ServiceOptions describes how the daemon should be registered with the
// platform service manager.
type ServiceOptions struct {
	Label      string
	BinaryPath string
	ConfigPath string
	LogDir     string
	StateDir   string
}

// ServiceState reports whether the daemon is registered and running.
type ServiceState struct {
	Installed   bool
	Running     bool
	Label       string
	UnitPath    string
	PID         int
	LastExit    int
	Description string
}

// Platform is the complete set of operating-system operations ghostgc needs.
//
// Implementations must be safe for concurrent use.
type Platform interface {
	// Name identifies the implementation, e.g. "darwin".
	Name() string

	// SnapshotProcesses performs the cheap system-wide scan and the detail
	// pass for processes owned by the current user. Processes owned by other
	// users are counted but never inspected.
	SnapshotProcesses(ctx context.Context) (*process.Snapshot, error)

	// InspectProcess performs a detail pass for a single PID. Callers must
	// validate the returned start time against any previously recorded start
	// time before acting on the result.
	InspectProcess(ctx context.Context, pid int) (process.Process, error)

	// SampleActivity performs the expensive, targeted activity pass for one
	// already-attributed process. Implementations must validate the exact key
	// before and after inspection and return partial availability explicitly.
	SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error)

	// SignalProcess validates the exact key and bound executable image
	// immediately before sending SIGTERM. The daemon must independently
	// establish action authority first.
	SignalProcess(ctx context.Context, key process.Key, executable process.ExecutableIdentity, sig Signal) error

	// InstallService registers the daemon with the platform service manager.
	InstallService(ctx context.Context, opts ServiceOptions) error

	// UninstallService removes the registration.
	UninstallService(ctx context.Context, label string) error

	// ServiceStatus reports the registration state.
	ServiceStatus(ctx context.Context, label string) (ServiceState, error)

	// SelfUID is the effective user id the daemon runs as. Only processes
	// with this uid are eligible for inspection or attribution.
	SelfUID() uint32
}
