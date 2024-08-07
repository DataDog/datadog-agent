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

func (f *FdResources) clone() *FdResources {
	return &FdResources{
		Fd:              maps.Clone(f.Fd),
		FileHandleCache: maps.Clone(f.FileHandleCache),
	}
}

// FSResources defines shared process resources
type FSResources struct {
	Cwd string
}

func (f *FSResources) clone() *FSResources {
	return &FSResources{
		Cwd: f.Cwd,
	}
}

// Process represents a process context
type Process struct {
	Pid   int
	Tgid  int
	Nr    map[int]*ebpfless.SyscallMsg
	FdRes *FdResources
	FsRes *FSResources
}

// NewProcess returns a new process
func NewProcess(pid int) *Process {
	return &Process{
		Pid:  pid,
		Tgid: pid,
		Nr:   make(map[int]*ebpfless.SyscallMsg),
		FdRes: &FdResources{
			Fd:              make(map[int32]string),
			FileHandleCache: make(map[fileHandleKey]*fileHandleVal),
		},
		FsRes: &FSResources{},
	}
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

	// fill the cache for next time
	p.FdRes.Fd[fd] = filename

	return filename, nil
}

// ProcessCache defines a thread cache
type ProcessCache struct {
	pid2Process map[int]*Process
	tgid2Pid    map[int][]int
	tgid2Span   map[int]*SpanTLS
}

// NewProcessCache returns a new thread cache
func NewProcessCache() *ProcessCache {
	return &ProcessCache{
		pid2Process: make(map[int]*Process),
		tgid2Pid:    make(map[int][]int),
		tgid2Span:   make(map[int]*SpanTLS),
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
	}

	if cloneFlags&unix.CLONE_FILES != 0 {
		process.FdRes = parent.FdRes
	} else {
		process.FdRes = parent.FdRes.clone()
	}
	if cloneFlags&unix.CLONE_FS != 0 {
		process.FsRes = parent.FsRes
	} else {
		process.FsRes = parent.FsRes.clone()
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

		for pid := range pids {
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

// GetSpan returns the span TLS entry for the given pid
func (tc *ProcessCache) GetSpan(tgid int) *SpanTLS {
	return tc.tgid2Span[tgid]
}

// SetSpanTLS sets the span TLS entry for the given pid
func (tc *ProcessCache) SetSpanTLS(tgid int, span *SpanTLS) {
	tc.tgid2Span[tgid] = span
}

// UnsetSpan unsets the span TLS entry for the given pid
func (tc *ProcessCache) UnsetSpan(tgid int) {
	delete(tc.tgid2Span, tgid)
}
