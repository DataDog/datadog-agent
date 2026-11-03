// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"golang.org/x/sys/unix"
)

type fileHandleKey struct {
	handleBytes uint32
	handleType  int32
}

type fileHandleVal struct {
	pathName string
}

// FdResources defines shared process resources
type FdResources struct {
	Fd              map[int32]string
	FileHandleCache map[fileHandleKey]*fileHandleVal
}

// clone retains file-handle metadata, but not paths from the creator's live FD table.
func (f *FdResources) clone() *FdResources {
	return &FdResources{
		Fd:              make(map[int32]string),
		FileHandleCache: maps.Clone(f.FileHandleCache),
	}
}

// FSResources defines shared process resources
type FSResources struct {
	Cwd string
}

// SocketInfo represents the status of an open socket
type SocketInfo struct {
	AddressFamily uint16
	Protocol      uint16
	BoundToPort   uint16
	SocketType    uint16
}

// Process represents a process context
type Process struct {
	Pid                      int
	Tgid                     int
	startTime                uint64 // /proc/<tgid>/stat starttime in clock ticks; zero if unavailable
	forkPPID                 uint32 // known process parent, not necessarily the creating TID
	creationHandled          bool   // creation hooks completed, or an initial tracee with no creation event
	creationWaitTimedOut     bool   // resumed without creation; inherited resources must not be reapplied
	CGroupID                 containerutils.CGroupID
	ContainerID              containerutils.ContainerID
	containerContextResolved bool
	Nr                       map[int]*ebpfless.SyscallMsg
	FdRes                    *FdResources
	FsRes                    *FSResources
	FdToSocket               map[int32]SocketInfo
}

// NewProcess returns a new process
func NewProcess(pid int) *Process {
	tgid := getPidTGID(pid)
	_, startTime, _ := readProcessStat(tgid)
	return &Process{
		Pid:        pid,
		Tgid:       tgid,
		startTime:  startTime,
		Nr:         make(map[int]*ebpfless.SyscallMsg),
		FdToSocket: make(map[int32]SocketInfo),

		FdRes: &FdResources{
			Fd:              make(map[int32]string),
			FileHandleCache: make(map[fileHandleKey]*fileHandleVal),
		},
		FsRes: &FSResources{},
	}
}

func (p *Process) identity() processIdentity {
	// A nonleader exec changes its TID to the TGID, not the process generation.
	return processIdentity{tgid: uint32(p.Tgid), startTime: p.startTime}
}

var errPipeFd = errors.New("pipe fd")
var errNegativeFd = errors.New("negative fd")

// GetFilenameFromFd returns the filename for the given fd
func (p *Process) GetFilenameFromFd(fd int32) (string, error) {
	if fd < 0 {
		return "", errNegativeFd
	}

	raw, err := p.getFilenameFromFdRaw(fd)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(raw, "pipe:") {
		return "", errPipeFd
	}

	return raw, nil
}

func (p *Process) getFilenameFromFdRaw(fd int32) (string, error) {
	filename, ok := p.FdRes.Fd[fd]
	if ok {
		return filename, nil
	}

	procPath := fmt.Sprintf("/proc/%d/fd/%d", p.Pid, fd)
	filename, err := os.Readlink(procPath)
	if err != nil {
		return "", err
	}
	// Inherited memfds must keep the memfd_create hook's naming convention.
	filename = procExecutableName(filename)

	// fill the cache for next time
	p.FdRes.Fd[fd] = filename

	return filename, nil
}

// ProcessCache defines a thread cache
type ProcessCache struct {
	pid2Process map[int]*Process
	tgid2Pid    map[int][]int
}

// NewProcessCache returns a new thread cache
func NewProcessCache() *ProcessCache {
	return &ProcessCache{
		pid2Process: make(map[int]*Process),
		tgid2Pid:    make(map[int][]int),
	}
}

// Add a process
func (tc *ProcessCache) Add(pid int, process *Process) {
	tc.pid2Process[pid] = process

	if process.Pid != process.Tgid {
		tc.tgid2Pid[process.Tgid] = append(tc.tgid2Pid[process.Tgid], process.Pid)
	}
}

func (tc *ProcessCache) shareResources(process *Process, ppid int, cloneFlags uint64) {
	parent := tc.pid2Process[ppid]
	if parent == nil {
		// this shouldn't happen
		return
	}

	// share resources, parent should never be nil
	if cloneFlags&unix.CLONE_THREAD != 0 {
		process.Tgid = parent.Tgid
		process.startTime = parent.startTime
	}
	switch {
	case cloneFlags&unix.CLONE_THREAD != 0:
		// A thread belongs to the same process, with the same known parent.
		process.forkPPID = parent.forkPPID
	case cloneFlags&unix.CLONE_PARENT != 0:
		// The creator is not the child's parent. Observe the actual parent
		// once at creation instead of resolving it anew on every exec.
		process.forkPPID = getPidPPID(process.Pid)
	default:
		process.forkPPID = uint32(parent.Tgid)
	}
	process.CGroupID = parent.CGroupID
	process.ContainerID = parent.ContainerID

	// A sibling can change the creator's FD/CWD caches after the kernel copies
	// private resources but before this creation event. Leave private paths
	// unresolved so they are read from the child's procfs entries instead.
	if cloneFlags&unix.CLONE_FILES != 0 {
		process.FdRes = parent.FdRes
	} else {
		process.FdRes = parent.FdRes.clone()
	}
	if cloneFlags&unix.CLONE_FS != 0 {
		process.FsRes = parent.FsRes
	} else {
		process.FsRes = &FSResources{}
	}

	// re-add to update the caches
	tc.Add(process.Pid, process)
}

// Remove a pid
func (tc *ProcessCache) Remove(process *Process) {
	delete(tc.pid2Process, process.Pid)

	if process.Pid == process.Tgid {
		pids, ok := tc.tgid2Pid[process.Pid]
		if !ok {
			return
		}
		delete(tc.tgid2Pid, process.Pid)

		for _, pid := range pids {
			delete(tc.pid2Process, pid)
		}
	} else {
		tc.tgid2Pid[process.Tgid] = slices.DeleteFunc(tc.tgid2Pid[process.Tgid], func(pid int) bool {
			return pid == process.Pid
		})
		if len(tc.tgid2Pid[process.Tgid]) == 0 {
			delete(tc.tgid2Pid, process.Tgid)
		}
	}
}

// Get return the process entry for the given pid
func (tc *ProcessCache) Get(pid int) *Process {
	return tc.pid2Process[pid]
}
