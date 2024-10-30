// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// SetAncestor sets the ancestor
func (pc *ProcessCacheEntry) SetAncestor(parent *ProcessCacheEntry) {
	if pc.Ancestor == parent {
		return
	}

	if pc.Ancestor != nil {
		pc.Ancestor.Release()
	}

	pc.hasValidLineage = nil
	pc.Ancestor = parent
	pc.Parent = &parent.Process
	parent.Retain()
}

func hasValidLineage(pc *ProcessCacheEntry) (bool, error) {
	var (
		pid, ppid uint32
		ctrID     containerutils.ContainerID
		err       error
	)

	for pc != nil {
		if pc.hasValidLineage != nil {
			return *pc.hasValidLineage, pc.lineageError
		}

		pid, ppid, ctrID = pc.Pid, pc.PPid, pc.ContainerID

		if pc.IsParentMissing {
			err = &ErrProcessMissingParentNode{PID: pid, PPID: ppid, ContainerID: string(ctrID)}
		}

		if pc.Pid == 1 {
			if pc.Ancestor == nil {
				return err == nil, err
			}
			return false, &ErrProcessWrongParentNode{PID: pid, PPID: pc.Ancestor.Pid, ContainerID: string(ctrID)}
		}
		pc = pc.Ancestor
	}

	return false, &ErrProcessIncompleteLineage{PID: pid, PPID: ppid, ContainerID: string(ctrID)}
}

// HasValidLineage returns false if, from the entry, we cannot ascend the ancestors list to PID 1 or if a new is having a missing parent
func (pc *ProcessCacheEntry) HasValidLineage() (bool, error) {
	res, err := hasValidLineage(pc)
	pc.hasValidLineage, pc.lineageError = &res, err
	return res, err
}

// Exit a process
func (pc *ProcessCacheEntry) Exit(exitTime time.Time) {
	pc.ExitTime = exitTime
}

func copyProcessContext(parent, child *ProcessCacheEntry) {
	// inherit the container ID from the parent if necessary. If a container is already running when system-probe
	// starts, the in-kernel process cache will have out of sync container ID values for the processes of that
	// container (the snapshot doesn't update the in-kernel cache with the container IDs). This can also happen if
	// the proc_cache LRU ejects an entry.
	// WARNING: this is why the user space cache should not be used to detect container breakouts. Dedicated
	// in-kernel probes will need to be added.
	if len(parent.ContainerID) > 0 && len(child.ContainerID) == 0 {
		child.CGroup = parent.CGroup
		child.ContainerID = parent.ContainerID
	}

	// AUIDs should be inherited just like container IDs
	child.Credentials.AUID = parent.Credentials.AUID
}

// ApplyExecTimeOf replace previous entry values by the given one
func (pc *ProcessCacheEntry) ApplyExecTimeOf(entry *ProcessCacheEntry) {
	pc.ExecTime = entry.ExecTime
}

// SetExecParent set the parent of the exec entry
func (pc *ProcessCacheEntry) SetExecParent(parent *ProcessCacheEntry) {
	pc.SetAncestor(parent)
	pc.IsExec = true
	pc.IsExecExec = pc.Parent != nil && pc.Parent.IsExec
}

// Exec replace a process
func (pc *ProcessCacheEntry) Exec(entry *ProcessCacheEntry) {
	entry.SetExecParent(pc)

	// use exec time as exit time
	pc.Exit(entry.ExecTime)

	// keep some context
	copyProcessContext(pc, entry)
}

// GetContainerPIDs return the pids
func (pc *ProcessCacheEntry) GetContainerPIDs() ([]uint32, []string) {
	var (
		pids  []uint32
		paths []string
	)

	for pc != nil {
		if pc.ContainerID == "" {
			break
		}
		pids = append(pids, pc.Pid)
		paths = append(paths, pc.FileEvent.PathnameStr)

		pc = pc.Ancestor
	}

	return pids, paths
}

// SetForkParent set the parent of the fork entry
func (pc *ProcessCacheEntry) SetForkParent(parent *ProcessCacheEntry) {
	pc.SetAncestor(parent)
	if parent != nil {
		pc.ArgsEntry = parent.ArgsEntry
		pc.EnvsEntry = parent.EnvsEntry
	}
}

// Fork returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Fork(childEntry *ProcessCacheEntry) {
	childEntry.PPid = pc.Pid
	childEntry.TTYName = pc.TTYName
	childEntry.Comm = pc.Comm
	childEntry.FileEvent = pc.FileEvent
	childEntry.ContainerID = pc.ContainerID
	childEntry.CGroup = pc.CGroup
	childEntry.ExecTime = pc.ExecTime
	childEntry.Credentials = pc.Credentials
	childEntry.LinuxBinprm = pc.LinuxBinprm
	childEntry.Cookie = pc.Cookie

	childEntry.SetForkParent(pc)
}

// Equals returns whether process cache entries share the same values for file and args/envs
func (pc *ProcessCacheEntry) Equals(entry *ProcessCacheEntry) bool {
	return (pc.FileEvent.Equals(&entry.FileEvent) &&
		pc.Credentials.Equals(&entry.Credentials) &&
		pc.ArgsEntry.Equals(entry.ArgsEntry) &&
		pc.EnvsEntry.Equals(entry.EnvsEntry))
}

func (pc *ProcessCacheEntry) markFileEventAsResolved() {
	// mark file path as resolved
	pc.FileEvent.SetPathnameStr("")
	pc.FileEvent.SetBasenameStr("")

	// mark interpreter as resolved too
	pc.LinuxBinprm.FileEvent.SetPathnameStr("")
	pc.LinuxBinprm.FileEvent.SetBasenameStr("")
}

// NewPlaceholderProcessCacheEntry returns a new empty process cache entry for failed process resolutions
func NewPlaceholderProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	entry := &ProcessCacheEntry{
		ProcessContext: ProcessContext{
			Process: Process{
				PIDContext: PIDContext{Pid: pid, Tid: tid, IsKworker: isKworker},
				Source:     ProcessCacheEntryFromPlaceholder,
			},
		},
	}
	entry.markFileEventAsResolved()
	return entry
}

var processContextZero = ProcessCacheEntry{ProcessContext: ProcessContext{Process: Process{Source: ProcessCacheEntryFromPlaceholder}}}

// GetPlaceholderProcessCacheEntry returns an empty process cache entry for failed process resolutions
func GetPlaceholderProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	processContextZero.Pid = pid
	processContextZero.Tid = tid
	processContextZero.IsKworker = isKworker
	processContextZero.markFileEventAsResolved()
	return &processContextZero
}
