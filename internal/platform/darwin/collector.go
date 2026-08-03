//go:build darwin

// Package darwin implements process observation on macOS.
//
// Collection is staged, as required by the specification:
//
//  1. one cheap system-wide sysctl(kern.proc.all) call that returns the whole
//     process table in a single syscall;
//  2. a detail pass, restricted to processes owned by the current user, that
//     resolves the executable path, argument vector, selected environment
//     variables, working directory, session id and task counters.
//
// The detail pass runs on a bounded worker pool. Processes owned by other
// users are counted and then left alone: ghostgc has no business inspecting
// them and could not manage them without elevated privileges anyway.
package darwin

/*
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <libproc.h>
#include <sys/proc_info.h>
*/
import "C"

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// Bounds on what a single process observation may cost in memory. A hostile or
// merely unusual process must not be able to grow the daemon.
// The argument cap is deliberately tight. Detection reads argv[0] and argv[1];
// everything beyond that is for display. Modern Electron applications carry
// multi-kilobyte command lines, and the daemon holds a whole snapshot in
// memory, so an uncapped argv would put the 50 MB residency target at the mercy
// of whatever the user happens to be running.
const (
	maxArgs       = 128
	maxArgBytes   = 4 << 10
	maxDetailJobs = 8
	noDev         = -1
)

// errSignalingDisabled mirrors platform.ErrSignalingDisabled. It is declared
// here rather than imported to keep this package free of any dependency on the
// interface package, which is what allows the factory to live there.
var errSignalingDisabled = fmt.Errorf("platform: process signalling is not implemented in this build; it is introduced in delivery phase 6, behind manual approval and full pre-action revalidation")

// Options configures the collector.
type Options struct {
	// EnvAllow lists environment variable names worth extracting. Everything
	// else is discarded during parsing and never reaches Go memory beyond the
	// scan-local buffer, which keeps the daemon's footprint flat regardless of
	// how large the observed environments are.
	EnvAllow []string
}

// Collector is the macOS implementation of platform.Platform.
type Collector struct {
	uid      uint32
	envAllow map[string]bool

	devMu    sync.Mutex
	devCache map[int32]string
}

// New constructs a macOS collector.
func New(opts Options) (*Collector, error) {
	c := &Collector{
		uid:      uint32(os.Getuid()),
		envAllow: make(map[string]bool, len(opts.EnvAllow)),
		devCache: make(map[int32]string),
	}
	for _, k := range opts.EnvAllow {
		c.envAllow[k] = true
	}
	return c, nil
}

// Name implements platform.Platform.
func (c *Collector) Name() string { return "darwin" }

// SelfUID implements platform.Platform.
func (c *Collector) SelfUID() uint32 { return c.uid }

// SignalProcess implements platform.Platform and always refuses.
//
// Do not implement this before the policy engine, its safety gates and its
// safety tests exist. See docs/safety.md.
func (c *Collector) SignalProcess(ctx context.Context, pid int, sig syscall.Signal) error {
	return errSignalingDisabled
}

// SnapshotProcesses implements platform.Platform.
func (c *Collector) SnapshotProcesses(ctx context.Context) (*process.Snapshot, error) {
	taken := time.Now()
	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("darwin: kern.proc.all: %w", err)
	}

	total := len(kprocs)
	procs := make([]process.Process, 0, total)
	for i := range kprocs {
		kp := &kprocs[i]
		if kp.Proc.P_pid <= 0 {
			continue
		}
		p := c.fromKinfo(kp)
		if p.UID != c.uid {
			// Counted in TotalCount, never inspected.
			continue
		}
		procs = append(procs, p)
	}

	if err := c.detailPass(ctx, procs); err != nil {
		return nil, err
	}

	sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
	return process.NewSnapshot(taken, procs, total), nil
}

// InspectProcess implements platform.Platform.
func (c *Collector) InspectProcess(ctx context.Context, pid int) (process.Process, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return process.Process{}, fmt.Errorf("darwin: kern.proc.pid %d: %w", pid, err)
	}
	if kp.Proc.P_pid != int32(pid) {
		return process.Process{}, fmt.Errorf("darwin: kern.proc.pid %d returned pid %d", pid, kp.Proc.P_pid)
	}
	p := c.fromKinfo(kp)
	if p.UID != c.uid {
		return p, nil
	}
	c.detail(&p)
	return p, nil
}

func (c *Collector) fromKinfo(kp *unix.KinfoProc) process.Process {
	start := time.Unix(kp.Proc.P_starttime.Sec, int64(kp.Proc.P_starttime.Usec)*1000)
	return process.Process{
		PID:       int(kp.Proc.P_pid),
		PPID:      int(kp.Eproc.Ppid),
		PGID:      int(kp.Eproc.Pgid),
		SID:       0, // resolved during the detail pass
		UID:       kp.Eproc.Ucred.Uid,
		StartTime: start,
		Comm:      cString(kp.Proc.P_comm[:]),
		Status:    statusOf(kp.Proc.P_stat),
		TTY:       c.ttyName(kp.Eproc.Tdev),
	}
}

func statusOf(stat int8) process.Status {
	switch stat {
	case 1: // SIDL
		return process.StatusStarting
	case 2: // SRUN
		return process.StatusRunning
	case 3: // SSLEEP
		return process.StatusSleeping
	case 4: // SSTOP
		return process.StatusStopped
	case 5: // SZOMB
		return process.StatusZombie
	default:
		return process.StatusUnknown
	}
}

// cString reads a NUL-terminated string out of a fixed kernel buffer. The
// kernel does not clear the tail of P_comm, so trailing bytes after the first
// NUL are garbage and must be dropped rather than trimmed.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func (c *Collector) ttyName(tdev int32) string {
	if tdev == noDev || tdev == 0 {
		return process.NoTTY
	}
	c.devMu.Lock()
	defer c.devMu.Unlock()
	if name, ok := c.devCache[tdev]; ok {
		return name
	}
	buf := make([]byte, 64)
	r := C.devname_r(C.dev_t(tdev), C.mode_t(C.S_IFCHR), (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	name := ""
	if r != nil {
		name = cString(buf)
	}
	if name == "" {
		name = fmt.Sprintf("dev:%d", tdev)
	} else if !strings.HasPrefix(name, "/") {
		name = "/dev/" + name
	}
	c.devCache[tdev] = name
	return name
}
