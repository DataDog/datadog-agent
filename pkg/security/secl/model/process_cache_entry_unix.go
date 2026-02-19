// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package model holds model related files
package model

import (
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

func (pc *ProcessCacheEntry) setAncestor(parent *ProcessCacheEntry) {
	if pc.Ancestor == parent {
		return
	}

	// remove from old parent's children list
	if pc.Ancestor != nil {
		pc.Ancestor.RemoveChild(pc)
	}

	pc.Ancestor = parent

	if parent != nil {
		pc.Parent = &parent.Process
		parent.Children = append(parent.Children, pc)
		pc.copyProcessContextFrom(parent)
	} else {
		pc.Parent = nil
	}
}

// RemoveChild removes a child from this entry's Children list.
func (pc *ProcessCacheEntry) RemoveChild(child *ProcessCacheEntry) {
	pc.Children = slices.DeleteFunc(pc.Children, func(c *ProcessCacheEntry) bool {
		return c == child
	})
}

// HasValidLineage returns false if, from the entry, we cannot ascend the ancestors list to PID 1 or if a node has a missing parent
func (pc *ProcessCacheEntry) HasValidLineage() (bool, error) {
	var (
		pid, ppid uint32
		ctrID     containerutils.ContainerID
	)

	for pc != nil {
		pid, ppid, ctrID = pc.Pid, pc.PPid, pc.ContainerContext.ContainerID

		if pc.IsParentMissing {
			return false, &ErrProcessMissingParentNode{PID: pid, PPID: ppid, ContainerID: string(ctrID)}
		}

		if pc.Pid == 1 {
			if pc.Ancestor == nil {
				return true, nil
			}
			return false, &ErrProcessWrongParentNode{PID: pid, PPID: pc.Ancestor.Pid, ContainerID: string(ctrID)}
		}
		pc = pc.Ancestor
	}

	return false, &ErrProcessIncompleteLineage{PID: pid, PPID: ppid, ContainerID: string(ctrID)}
}

// Exit a process
func (pc *ProcessCacheEntry) Exit(exitTime time.Time) {
	pc.ExitTime = exitTime
}

func (pc *ProcessCacheEntry) copyProcessContextFrom(parent *ProcessCacheEntry) {
	pc.copySSHUserSessionFrom(parent)
	pc.copyAUIDFrom(parent)
	pc.copyCGroupFrom(parent)
	pc.copyContainerContextFrom(parent)
	pc.copyNSFrom(parent)
}

func (pc *ProcessCacheEntry) copyAUIDFrom(parent *ProcessCacheEntry) {
	if pc.Credentials.AUID == 0 {
		pc.Credentials.AUID = parent.Credentials.AUID
	}
}

func (pc *ProcessCacheEntry) copySSHUserSessionFrom(parent *ProcessCacheEntry) {
	if parent.ProcessContext.UserSession.SSHSessionID != 0 {
		pc.UserSession.SSHSessionContext = parent.UserSession.SSHSessionContext
	}
}

// ApplyExecTimeOf replace previous entry values by the given one
func (pc *ProcessCacheEntry) ApplyExecTimeOf(entry *ProcessCacheEntry) {
	pc.ExecTime = entry.ExecTime
}

// SetExecParent set the parent of the exec entry
func (pc *ProcessCacheEntry) SetExecParent(parent *ProcessCacheEntry) {
	pc.setAncestor(parent)
	pc.IsExec = true
	pc.IsExecExec = pc.Parent != nil && pc.Parent.IsExec
}

// SetAsExec set the entry as an Exec
func (pc *ProcessCacheEntry) SetAsExec() {
	pc.IsExec = true
}

// Exec replace a process
func (pc *ProcessCacheEntry) Exec(entry *ProcessCacheEntry) {
	entry.SetExecParent(pc)

	// use exec time as exit time
	pc.Exit(entry.ExecTime)
}

// GetContainerPIDs return the pids
func (pc *ProcessCacheEntry) GetContainerPIDs() ([]uint32, []string) {
	var (
		pids  []uint32
		paths []string
	)

	for pc != nil {
		if pc.ContainerContext.IsNull() {
			break
		}
		if !slices.Contains(pids, pc.Pid) {
			// add only LAST exec of an unique pid
			pids = append(pids, pc.Pid)
			paths = append(paths, pc.FileEvent.PathnameStr)
		}
		pc = pc.Ancestor
	}

	return pids, paths
}

func (pc *ProcessCacheEntry) copyCGroupFrom(parent *ProcessCacheEntry) {
	if !parent.CGroup.CGroupPathKey.IsNull() && pc.CGroup.IsNull() {
		pc.CGroup = parent.CGroup
	}
}

func (pc *ProcessCacheEntry) copyContainerContextFrom(parent *ProcessCacheEntry) {
	if !parent.ContainerContext.IsNull() && pc.ContainerContext.IsNull() {
		pc.ContainerContext = parent.ContainerContext
	}
}

func (pc *ProcessCacheEntry) copyNSFrom(parent *ProcessCacheEntry) {
	if pc.NetNS == 0 {
		pc.NetNS = parent.NetNS
	}

	if pc.MntNS == 0 {
		pc.MntNS = parent.MntNS
	}
}

// GetAncestorsPIDs return the ancestors list PIDs
func (pc *ProcessCacheEntry) GetAncestorsPIDs() []uint32 {
	var pids []uint32

	for pc != nil {
		if !slices.Contains(pids, pc.Pid) {
			pids = append(pids, pc.Pid)
		}
		pc = pc.Ancestor
	}
	return pids
}

// SetForkParent set the parent of the fork entry
func (pc *ProcessCacheEntry) SetForkParent(parent *ProcessCacheEntry) {
	pc.setAncestor(parent)
	pc.ArgsEntry = parent.ArgsEntry
	pc.EnvsEntry = parent.EnvsEntry
}

// Fork returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Fork(child *ProcessCacheEntry) {
	child.PPid = pc.Pid
	child.TTYName = pc.TTYName
	child.Comm = pc.Comm
	child.FileEvent = pc.FileEvent
	child.ExecTime = pc.ExecTime
	child.Credentials = pc.Credentials
	child.LinuxBinprm = pc.LinuxBinprm
	child.Cookie = pc.Cookie
	child.TracerTags = pc.TracerTags

	child.SetForkParent(pc)
}

// Reparent updates the parent of the process cache entry to reflect reparenting by the kernel.
// This handles the subreaper mechanism where children are reparented when their parent exits.
func (pc *ProcessCacheEntry) Reparent(newParent *ProcessCacheEntry) {
	pc.PPid = newParent.Pid
	pc.IsParentMissing = false
	pc.setAncestor(newParent)
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
