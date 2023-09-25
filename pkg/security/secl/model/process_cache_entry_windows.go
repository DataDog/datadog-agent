// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "time"

// NewPlaceholderProcessCacheEntry returns an empty process cache entry for failed process resolutions
func NewPlaceholderProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	return &ProcessCacheEntry{ProcessContext: ProcessContext{Process: Process{PIDContext: PIDContext{Pid: pid, Tid: tid, IsKworker: isKworker}}}}
}

// GetPlaceholderProcessCacheEntry returns an empty process cache entry for failed process resolutions
func GetPlaceholderProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	processContextZero.Pid = pid
	processContextZero.Tid = tid
	processContextZero.IsKworker = isKworker
	return &processContextZero
}

// SetAncestor sets the ancestor
func (pc *ProcessCacheEntry) SetAncestor(parent *ProcessCacheEntry) {
	if pc.Ancestor == parent {
		return
	}

	if pc.Ancestor != nil {
		pc.Ancestor.Release()
	}

	pc.Ancestor = parent
	pc.Parent = &parent.Process
	parent.Retain()
}

// Exit a process
func (pc *ProcessCacheEntry) Exit(exitTime time.Time) {
	pc.ExitTime = exitTime
}
