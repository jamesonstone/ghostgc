//go:build darwin

package darwin

/*
#include <stdlib.h>
#include <libproc.h>
#include <sys/proc_info.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/jamesonstone/ghostgc/internal/process"
)

const (
	maxUsageProcesses = 4096
	maxUsageFDs       = 131072
	maxUsageEntries   = 100000
)

type vnodeIdentity struct {
	device uint64
	inode  uint64
}

// InspectPathUsage performs two same-user process-table snapshots around a
// bounded CWD/vnode inspection. A changing table is incomplete evidence.
func (c *Collector) InspectPathUsage(ctx context.Context, root string) (process.PathUsage, error) {
	candidateVnodes, err := pathVnodeIdentities(ctx, root)
	if err != nil {
		return process.PathUsage{}, err
	}
	before, err := c.sameUserProcesses()
	if err != nil {
		return process.PathUsage{}, err
	}
	if len(before) > maxUsageProcesses {
		return process.PathUsage{}, fmt.Errorf("darwin: path inspection exceeded %d same-user processes", maxUsageProcesses)
	}
	result := process.PathUsage{InspectedProcesses: len(before)}
	used := make(map[string]process.Key)
	totalFDs := 0
	for _, observed := range before {
		if err := ctx.Err(); err != nil {
			return process.PathUsage{}, err
		}
		key := observed.Key()
		cwd := pidCWD(observed.PID)
		if cwd == "" && observed.Status != process.StatusZombie && c.keyStillPresent(key) {
			return process.PathUsage{}, fmt.Errorf("darwin: working-directory inspection was incomplete for process %s", key)
		}
		if withinRepository(root, cwd) {
			result.CWDReferences++
			used[key.UID()] = key
		}
		if observed.Status == process.StatusZombie {
			continue
		}
		matched, inspected, complete := c.pathVnodes(observed.PID, root, key, candidateVnodes)
		totalFDs += inspected
		if totalFDs > maxUsageFDs {
			return process.PathUsage{}, fmt.Errorf("darwin: path inspection exceeded %d descriptors", maxUsageFDs)
		}
		if !complete {
			return process.PathUsage{}, fmt.Errorf("darwin: vnode inspection was incomplete for process %s", key)
		}
		if matched > 0 {
			result.OpenVnodes += matched
			used[key.UID()] = key
		}
	}
	after, err := c.sameUserProcesses()
	if err != nil {
		return process.PathUsage{}, err
	}
	if !sameKeys(before, after) {
		return process.PathUsage{}, fmt.Errorf("darwin: process table changed during path inspection")
	}
	for _, key := range used {
		result.ProcessKeys = append(result.ProcessKeys, key)
	}
	sort.Slice(result.ProcessKeys, func(i, j int) bool { return result.ProcessKeys[i].UID() < result.ProcessKeys[j].UID() })
	result.Complete = true
	return result, nil
}

func pathVnodeIdentities(ctx context.Context, root string) (map[vnodeIdentity]bool, error) {
	identities := make(map[vnodeIdentity]bool)
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("darwin: path usage traversal was incomplete")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > maxUsageEntries {
			return fmt.Errorf("darwin: path usage exceeded %d filesystem entries", maxUsageEntries)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		var stat unix.Stat_t
		if err := unix.Lstat(path, &stat); err != nil {
			return errors.New("darwin: path usage metadata inspection was incomplete")
		}
		identities[vnodeIdentity{device: uint64(stat.Dev), inode: stat.Ino}] = true
		return nil
	})
	return identities, err
}

func (c *Collector) sameUserProcesses() ([]process.Process, error) {
	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("darwin: listing processes for path inspection: %w", err)
	}
	out := make([]process.Process, 0, len(kprocs))
	for i := range kprocs {
		p := c.fromKinfo(&kprocs[i])
		if p.PID > 0 && p.UID == c.uid {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key().UID() < out[j].Key().UID() })
	return out, nil
}

func sameKeys(a, b []process.Process) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key() != b[i].Key() {
			return false
		}
	}
	return true
}

func (c *Collector) keyStillPresent(key process.Key) bool {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", key.PID)
	return err == nil && c.fromKinfo(kp).Key() == key
}

func (c *Collector) pathVnodes(pid int, root string, key process.Key,
	candidateVnodes map[vnodeIdentity]bool) (matched, inspected int, complete bool) {
	fdSize := int(C.sizeof_struct_proc_fdinfo)
	needed := int(C.proc_pidinfo(C.int(pid), C.PROC_PIDLISTFDS, 0, nil, 0))
	if needed <= 0 {
		return 0, 0, !c.keyStillPresent(key) || key.PID == 0
	}
	if needed/fdSize > maxActivityFDs {
		return 0, 0, false
	}
	capacity := needed + 32*fdSize
	buf := C.malloc(C.size_t(capacity))
	if buf == nil {
		return 0, 0, false
	}
	defer C.free(buf)
	n := int(C.proc_pidinfo(C.int(pid), C.PROC_PIDLISTFDS, 0, buf, C.int(capacity)))
	if n <= 0 || n >= capacity {
		return 0, 0, !c.keyStillPresent(key)
	}
	fds := unsafe.Slice((*C.struct_proc_fdinfo)(buf), n/fdSize)
	for _, fd := range fds {
		inspected++
		if fd.proc_fdtype != C.PROX_FDTYPE_VNODE {
			continue
		}
		var info C.struct_vnode_fdinfowithpath
		got := C.proc_pidfdinfo(C.int(pid), C.int(fd.proc_fd), C.PROC_PIDFDVNODEPATHINFO,
			unsafe.Pointer(&info), C.int(unsafe.Sizeof(info)))
		if int(got) >= int(unsafe.Sizeof(info)) {
			if withinRepository(root, C.GoString(&info.pvip.vip_path[0])) {
				matched++
			}
			continue
		}
		var metadata C.struct_vnode_fdinfo
		got = C.proc_pidfdinfo(C.int(pid), C.int(fd.proc_fd), C.PROC_PIDFDVNODEINFO,
			unsafe.Pointer(&metadata), C.int(unsafe.Sizeof(metadata)))
		if int(got) < int(unsafe.Sizeof(metadata)) {
			if c.keyStillPresent(key) {
				return matched, inspected, false
			}
			continue
		}
		identity := vnodeIdentity{
			device: uint64(metadata.pvi.vi_stat.vst_dev),
			inode:  uint64(metadata.pvi.vi_stat.vst_ino),
		}
		if candidateVnodes[identity] {
			matched++
		}
	}
	return matched, inspected, true
}
