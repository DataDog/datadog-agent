// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
	"golang.org/x/exp/slices"
)

type fileHandleKey struct {
	handleBytes uint32
	handleType  int32
}

type fileHandleVal struct {
	pathName string
}

// Resources defines shared process resources
type Resources struct {
	Fd              map[int32]string
	Cwd             string
	FileHandleCache map[fileHandleKey]*fileHandleVal
}

// Process represents a process context
type Process struct {
	Pid  int
	Tgid int
	Nr   map[int]*ebpfless.SyscallMsg
	Res  *Resources
}

// NewProcess returns a new process
func NewProcess(pid int) *Process {
	return &Process{
		Pid:  pid,
		Tgid: pid,
		Nr:   make(map[int]*ebpfless.SyscallMsg),
		Res: &Resources{
			Fd:              make(map[int32]string),
			FileHandleCache: make(map[fileHandleKey]*fileHandleVal),
		},
	}
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

// SetAsThreadOf set the process as thread of the given tgid
func (tc *ProcessCache) SetAsThreadOf(process *Process, ppid int) {
	parent := tc.pid2Process[ppid]
	if parent == nil {
		// this shouldn't happen
		return
	}

	// share resources, parent should never be nil
	process.Tgid = parent.Tgid
	process.Res = parent.Res

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
