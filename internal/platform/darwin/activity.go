//go:build darwin

package darwin

/*
#include <stdlib.h>
#include <libproc.h>
#include <sys/fcntl.h>
#include <sys/proc_info.h>
#include <sys/resource.h>

static int ghostgc_pid_rusage(int pid, struct rusage_info_v4 *info) {
	return proc_pid_rusage(pid, RUSAGE_INFO_V4, (rusage_info_t *)info);
}
*/
import "C"

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jamesonstone/ghostgc/internal/process"
)

const maxActivityFDs = 4096

// SampleActivity performs the expensive pass only when the daemon has already
// selected an attributed process. Paths and socket endpoints never leave this
// function; only bounded counts and queue sizes are returned.
func (c *Collector) SampleActivity(ctx context.Context, key process.Key, repositoryRoot string) (process.ActivitySample, error) {
	if err := ctx.Err(); err != nil {
		return process.ActivitySample{}, err
	}
	if err := c.validateActivityKey(key); err != nil {
		return process.ActivitySample{}, err
	}

	sample := process.ActivitySample{Key: key, Taken: time.Now()}
	if cpu, rss, _, _, ok := pidTaskInfo(key.PID); ok {
		sample.CPUTime, sample.RSSBytes, sample.CPUKnown = cpu, rss, true
	}
	if read, written, ok := pidIOCounters(key.PID); ok {
		sample.DiskReadBytes, sample.DiskWrittenBytes, sample.IOKnown = read, written, true
	}
	c.descriptorActivity(key.PID, repositoryRoot, &sample)

	if err := c.validateActivityKey(key); err != nil {
		return process.ActivitySample{}, err
	}
	return sample, nil
}

func (c *Collector) validateActivityKey(key process.Key) error {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", key.PID)
	if err != nil {
		return fmt.Errorf("darwin: activity target %s unavailable: %w", key, err)
	}
	observed := c.fromKinfo(kp)
	if observed.UID != c.uid {
		return fmt.Errorf("darwin: activity target %s is owned by uid %d", key, observed.UID)
	}
	if observed.Key() != key {
		return fmt.Errorf("darwin: activity target changed from %s to %s", key, observed.Key())
	}
	return nil
}

func pidIOCounters(pid int) (read, written uint64, ok bool) {
	var info C.struct_rusage_info_v4
	if C.ghostgc_pid_rusage(C.int(pid), &info) != 0 {
		return 0, 0, false
	}
	return uint64(info.ri_diskio_bytesread), uint64(info.ri_diskio_byteswritten), true
}

func (c *Collector) descriptorActivity(pid int, repositoryRoot string, sample *process.ActivitySample) {
	fdSize := int(C.sizeof_struct_proc_fdinfo)
	needed := int(C.proc_pidinfo(C.int(pid), C.PROC_PIDLISTFDS, 0, nil, 0))
	if needed <= 0 {
		sample.Note = "file and socket descriptors were unavailable"
		return
	}
	if needed/fdSize > maxActivityFDs {
		sample.Note = fmt.Sprintf("descriptor count exceeded the %d-entry inspection bound", maxActivityFDs)
		return
	}
	capacity := needed + 32*fdSize
	buf := C.malloc(C.size_t(capacity))
	if buf == nil {
		sample.Note = "descriptor inspection allocation failed"
		return
	}
	defer C.free(buf)

	n := int(C.proc_pidinfo(C.int(pid), C.PROC_PIDLISTFDS, 0, buf, C.int(capacity)))
	if n <= 0 || n >= capacity {
		sample.Note = "descriptor list changed or became unavailable during inspection"
		return
	}
	fds := unsafe.Slice((*C.struct_proc_fdinfo)(buf), n/fdSize)
	filesComplete, socketsComplete := true, true
	for _, fd := range fds {
		switch fd.proc_fdtype {
		case C.PROX_FDTYPE_VNODE:
			sample.OpenFiles++
			if !c.inspectVnode(pid, int(fd.proc_fd), repositoryRoot, sample) {
				filesComplete = false
			}
		case C.PROX_FDTYPE_SOCKET:
			sample.Sockets++
			if !inspectSocket(pid, int(fd.proc_fd), sample) {
				socketsComplete = false
			}
		}
	}
	sample.FilesKnown = filesComplete
	sample.SocketsKnown = socketsComplete
}

func (c *Collector) inspectVnode(pid, fd int, repositoryRoot string, sample *process.ActivitySample) bool {
	if repositoryRoot == "" {
		return true
	}
	var info C.struct_vnode_fdinfowithpath
	n := C.proc_pidfdinfo(C.int(pid), C.int(fd), C.PROC_PIDFDVNODEPATHINFO,
		unsafe.Pointer(&info), C.int(unsafe.Sizeof(info)))
	if int(n) < int(unsafe.Sizeof(info)) {
		return false
	}
	if uint32(info.pfi.fi_openflags)&uint32(C.FWRITE) == 0 {
		return true
	}
	path := C.GoString(&info.pvip.vip_path[0])
	if withinRepository(repositoryRoot, path) {
		sample.WritableRepositoryFiles++
	}
	return true
}

func inspectSocket(pid, fd int, sample *process.ActivitySample) bool {
	var info C.struct_socket_fdinfo
	n := C.proc_pidfdinfo(C.int(pid), C.int(fd), C.PROC_PIDFDSOCKETINFO,
		unsafe.Pointer(&info), C.int(unsafe.Sizeof(info)))
	if int(n) < int(unsafe.Sizeof(info)) {
		return false
	}
	state := uint16(info.psi.soi_state)
	connected := uint16(C.SOI_S_ISCONNECTED | C.SOI_S_ISCONNECTING | C.SOI_S_COMP)
	if state&connected != 0 {
		sample.ConnectedSockets++
	}
	sample.ReceiveQueueBytes += uint64(info.psi.soi_rcv.sbi_cc)
	sample.SendQueueBytes += uint64(info.psi.soi_snd.sbi_cc)
	return true
}

func withinRepository(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
