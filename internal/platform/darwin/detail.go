//go:build darwin

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
	"encoding/binary"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jamesonstone/ghostgc/internal/process"
)

// detailPass fills in the expensive fields for every process in procs using a
// bounded worker pool.
func (c *Collector) detailPass(ctx context.Context, procs []process.Process) error {
	if len(procs) == 0 {
		return nil
	}
	workers := runtime.NumCPU()
	if workers > maxDetailJobs {
		workers = maxDetailJobs
	}
	if workers < 1 {
		workers = 1
	}

	var (
		wg   sync.WaitGroup
		next = make(chan int)
	)
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := range next {
				c.detail(&procs[i])
			}
		}()
	}

	err := func() error {
		defer close(next)
		for i := range procs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case next <- i:
			}
		}
		return nil
	}()
	wg.Wait()
	return err
}

// detail performs the per-process inspection. Every step is allowed to fail:
// a process can exit at any point during a scan, and a partially observed
// process is recorded as partially observed rather than discarded or guessed at.
func (c *Collector) detail(p *process.Process) {
	pid := p.PID
	p.ExecPath = pidPath(pid)
	execFromArgs, args, env, envReadable := c.procArgs(pid, c.envAllow)
	if p.ExecPath == "" {
		p.ExecPath = execFromArgs
	}
	p.Args = args
	p.Env = env
	p.EnvReadable = envReadable
	if sid, err := unix.Getsid(pid); err == nil {
		p.SID = sid
	}
	p.CWD = pidCWD(pid)
	if cpu, rss, vsz, threads, ok := pidTaskInfo(pid); ok {
		p.CPUTime = cpu
		p.RSSBytes = rss
		p.VSZBytes = vsz
		p.Threads = threads
	}
	p.Detailed = true
}

func pidPath(pid int) string {
	buf := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)
	n := C.proc_pidpath(C.int(pid), unsafe.Pointer(&buf[0]), C.uint32_t(len(buf)))
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

func pidCWD(pid int) string {
	var vi C.struct_proc_vnodepathinfo
	n := C.proc_pidinfo(C.int(pid), C.PROC_PIDVNODEPATHINFO, 0, unsafe.Pointer(&vi), C.int(unsafe.Sizeof(vi)))
	if int(n) < int(unsafe.Sizeof(vi)) {
		return ""
	}
	return C.GoString(&vi.pvi_cdir.vip_path[0])
}

func pidTaskInfo(pid int) (cpu time.Duration, rss, vsz uint64, threads int, ok bool) {
	var ti C.struct_proc_taskinfo
	n := C.proc_pidinfo(C.int(pid), C.PROC_PIDTASKINFO, 0, unsafe.Pointer(&ti), C.int(unsafe.Sizeof(ti)))
	if int(n) < int(unsafe.Sizeof(ti)) {
		return 0, 0, 0, 0, false
	}
	// pti_total_user and pti_total_system are in nanoseconds.
	cpu = time.Duration(uint64(ti.pti_total_user) + uint64(ti.pti_total_system))
	return cpu, uint64(ti.pti_resident_size), uint64(ti.pti_virtual_size), int(ti.pti_threadnum), true
}

// procArgs reads the argument vector and environment of a process from
// kern.procargs2.
//
// The buffer layout is: a 32-bit argument count, the executable path, NUL
// padding, argc NUL-terminated argument strings, then NUL-terminated
// environment strings. Only allowlisted environment variables are retained;
// the rest are skipped in place so that large environments never become large
// allocations.
//
// envReadable reports whether the kernel returned an environment section at
// all. For SIP-protected system binaries macOS returns the arguments and stops,
// so an unreadable environment and an empty one are indistinguishable from the
// contents alone — the caller is told which it was.
func (c *Collector) procArgs(pid int, allow map[string]bool) (execPath string, args []string, env map[string]string, envReadable bool) {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) < 4 {
		return "", nil, nil, false
	}
	argc := int(int32(binary.LittleEndian.Uint32(raw[:4])))
	rest := raw[4:]

	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", nil, nil, false
	}
	execPath = string(rest[:i])
	rest = skipNULs(rest[i:])

	argBytes := 0
	for n := 0; n < argc && len(rest) > 0; n++ {
		tok, remainder, ok := nextCString(rest)
		rest = remainder
		if !ok {
			break
		}
		if len(args) < maxArgs && argBytes < maxArgBytes {
			args = append(args, string(tok))
			argBytes += len(tok)
		}
	}
	rest = skipNULs(rest)
	envReadable = len(rest) > 0

	if len(allow) > 0 {
		for len(rest) > 0 {
			tok, remainder, ok := nextCString(rest)
			rest = remainder
			if !ok {
				break
			}
			eq := bytes.IndexByte(tok, '=')
			if eq <= 0 {
				continue
			}
			name := string(tok[:eq])
			if !allow[name] {
				continue
			}
			if env == nil {
				env = make(map[string]string, len(allow))
			}
			env[name] = string(tok[eq+1:])
		}
	}
	return execPath, args, env, envReadable
}

func skipNULs(b []byte) []byte {
	for len(b) > 0 && b[0] == 0 {
		b = b[1:]
	}
	return b
}

func nextCString(b []byte) (tok, rest []byte, ok bool) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return b, nil, len(b) > 0
	}
	return b[:i], b[i+1:], true
}
