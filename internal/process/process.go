// Package process defines the platform-neutral process model used by ghostgc.
//
// Nothing in this package sends signals or mutates system state. It describes
// what was observed and how to identify it again safely.
package process

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Status is the coarse kernel-reported run state of a process.
type Status string

const (
	StatusUnknown  Status = "unknown"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusSleeping Status = "sleeping"
	StatusStopped  Status = "stopped"
	StatusZombie   Status = "zombie"
)

// NoTTY is the TTY value for a process with no controlling terminal.
const NoTTY = ""

// Key identifies a process in a way that survives PID reuse.
//
// A PID on its own is never sufficient: the kernel recycles PIDs, so a stored
// PID may refer to an unrelated process by the time it is used. Every part of
// ghostgc that names a process names it with a Key.
type Key struct {
	PID         int
	StartTimeNs int64
}

// NewKey builds a Key from a PID and process start time.
func NewKey(pid int, start time.Time) Key {
	return Key{PID: pid, StartTimeNs: start.UnixNano()}
}

// UID is the stable string form of a Key, used as the primary key in storage.
// It is deliberately human-readable so that stored rows can be reasoned about
// without a lookup table.
func (k Key) UID() string {
	return strconv.Itoa(k.PID) + ":" + strconv.FormatInt(k.StartTimeNs, 10)
}

// String implements fmt.Stringer.
func (k Key) String() string { return k.UID() }

// StartTime returns the process start time.
func (k Key) StartTime() time.Time { return time.Unix(0, k.StartTimeNs) }

// Zero reports whether the key is unset.
func (k Key) Zero() bool { return k.PID == 0 && k.StartTimeNs == 0 }

// ParseKey parses the output of Key.UID.
func ParseKey(s string) (Key, error) {
	pidStr, startStr, ok := strings.Cut(s, ":")
	if !ok {
		return Key{}, fmt.Errorf("process: malformed key %q", s)
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return Key{}, fmt.Errorf("process: malformed key %q: %w", s, err)
	}
	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		return Key{}, fmt.Errorf("process: malformed key %q: %w", s, err)
	}
	return Key{PID: pid, StartTimeNs: start}, nil
}

// Process is a single observation of one operating-system process.
//
// Fields below the Detailed marker are only populated when the collector
// performed a detail pass for this process. The daemon performs detail passes
// only for processes owned by the current user.
type Process struct {
	PID       int
	PPID      int
	PGID      int
	SID       int
	UID       uint32
	StartTime time.Time
	Comm      string // kernel-truncated name, always available
	Status    Status

	// TTY is the controlling terminal device path, or NoTTY when the process
	// has none. Never infer "safe to terminate" from this field alone.
	TTY string

	// Detailed reports whether the expensive per-process inspection ran.
	Detailed bool

	ExecPath string
	Args     []string
	CWD      string

	// Env holds the allowlisted environment variables. It is never persisted
	// verbatim; see RedactEnv. It is retained in memory only for the duration
	// of a scan because agent adapters use it for attribution evidence.
	Env map[string]string

	// EnvReadable reports whether the operating system let ghostgc see this
	// process's environment at all.
	//
	// macOS redacts the environment portion of kern.procargs2 for
	// SIP-protected system binaries, so /bin/sh, /bin/sleep and everything
	// else shipped with the OS return their arguments and nothing more. An
	// empty Env therefore means one of two very different things, and
	// conflating them would let "the agent set no variables" stand in for
	// "the daemon was not permitted to look".
	EnvReadable bool

	CPUTime  time.Duration
	RSSBytes uint64
	VSZBytes uint64
	Threads  int
}

// Key returns the PID-reuse-safe identity of the process.
func (p Process) Key() Key { return NewKey(p.PID, p.StartTime) }

// HasTTY reports whether the process has a controlling terminal.
func (p Process) HasTTY() bool { return p.TTY != NoTTY }

// Name returns the best available short name for the process: the executable
// basename when known, otherwise the kernel-reported comm.
func (p Process) Name() string {
	if p.ExecPath != "" {
		if i := strings.LastIndexByte(p.ExecPath, '/'); i >= 0 && i+1 < len(p.ExecPath) {
			return p.ExecPath[i+1:]
		}
		return p.ExecPath
	}
	return p.Comm
}

// Snapshot is an immutable point-in-time view of the process table.
type Snapshot struct {
	Taken     time.Time
	Processes []Process

	// TotalCount is the number of processes visible on the system, including
	// those owned by other users that were deliberately not inspected.
	TotalCount int

	byPID map[int]int
}

// NewSnapshot indexes a set of observations. The slice is retained, not copied.
func NewSnapshot(taken time.Time, procs []Process, total int) *Snapshot {
	s := &Snapshot{Taken: taken, Processes: procs, TotalCount: total, byPID: make(map[int]int, len(procs))}
	for i, p := range procs {
		// On the vanishingly rare chance the kernel hands back a duplicate PID
		// within one scan, the later entry wins; both are still individually
		// distinguishable by start time downstream.
		s.byPID[p.PID] = i
	}
	return s
}

// ByPID looks up a process by PID within this snapshot.
func (s *Snapshot) ByPID(pid int) (Process, bool) {
	if s == nil {
		return Process{}, false
	}
	i, ok := s.byPID[pid]
	if !ok {
		return Process{}, false
	}
	return s.Processes[i], true
}

// ByKey looks up a process by its PID-reuse-safe key.
func (s *Snapshot) ByKey(k Key) (Process, bool) {
	p, ok := s.ByPID(k.PID)
	if !ok || p.Key() != k {
		return Process{}, false
	}
	return p, true
}

// Len returns the number of inspected processes in the snapshot.
func (s *Snapshot) Len() int {
	if s == nil {
		return 0
	}
	return len(s.Processes)
}

// DetailedCount returns how many processes received a detail pass.
func (s *Snapshot) DetailedCount() int {
	n := 0
	for _, p := range s.Processes {
		if p.Detailed {
			n++
		}
	}
	return n
}
