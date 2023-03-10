// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

const (
	// MaxArgEnvSize maximum size of one argument or environment variable
	MaxArgEnvSize = 256
)

// SetSpan sets the span
func (pc *ProcessCacheEntry) SetSpan(spanID uint64, traceID uint64) {
	pc.SpanID = spanID
	pc.TraceID = traceID
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
	pc.IsThread = false
	parent.Retain()
}

// GetNextAncestorBinary returns the first ancestor with a different binary
func (pc *ProcessCacheEntry) GetNextAncestorBinary() *ProcessCacheEntry {
	current := pc
	ancestor := pc.Ancestor
	for ancestor != nil {
		if current.Inode != ancestor.Inode {
			return ancestor
		}
		current = ancestor
		ancestor = ancestor.Ancestor
	}
	return nil
}

// HasCompleteLineage returns false if, from the entry, we cannot ascend the ancestors list to PID 1
func (pc *ProcessCacheEntry) HasCompleteLineage() bool {
	for pc != nil {
		if pc.Pid == 1 {
			return true
		}
		pc = pc.Ancestor
	}
	return false
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
		child.ContainerID = parent.ContainerID
	}
}

// Exec replace a process
func (pc *ProcessCacheEntry) Exec(entry *ProcessCacheEntry) {
	entry.SetAncestor(pc)

	// use exec time a exit time
	pc.Exit(entry.ExecTime)

	// keep some context
	copyProcessContext(pc, entry)
}

// SetParentOfForkChild set the parent of a fork child
func (pc *ProcessCacheEntry) SetParentOfForkChild(parent *ProcessCacheEntry) {
	pc.SetAncestor(parent)
	if parent != nil {
		pc.ArgsEntry = parent.ArgsEntry
		pc.EnvsEntry = parent.EnvsEntry
	}
	pc.IsThread = true
}

// Fork returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Fork(childEntry *ProcessCacheEntry) {
	childEntry.PPid = pc.Pid
	childEntry.TTYName = pc.TTYName
	childEntry.Comm = pc.Comm
	childEntry.FileEvent = pc.FileEvent
	childEntry.ContainerID = pc.ContainerID
	childEntry.ExecTime = pc.ExecTime
	childEntry.Credentials = pc.Credentials
	childEntry.LinuxBinprm = pc.LinuxBinprm
	childEntry.Cookie = pc.Cookie

	childEntry.SetParentOfForkChild(pc)
}

// Equals returns whether process cache entries share the same values for comm and args/envs
func (pc *ProcessCacheEntry) Equals(entry *ProcessCacheEntry) bool {
	return pc.Comm == entry.Comm && pc.ArgsEntry.Equals(entry.ArgsEntry) && pc.EnvsEntry.Equals(entry.EnvsEntry)
}

// NewEmptyProcessCacheEntry returns an empty process cache entry for kworker events or failed process resolutions
func NewEmptyProcessCacheEntry(pid uint32, tid uint32, isKworker bool) *ProcessCacheEntry {
	return &ProcessCacheEntry{ProcessContext: ProcessContext{Process: Process{PIDContext: PIDContext{Pid: pid, Tid: tid, IsKworker: isKworker}}}}
}

// ArgsEnvs raw value for args and envs
type ArgsEnvs struct {
	ID        uint32
	Size      uint32
	ValuesRaw [MaxArgEnvSize]byte
}

// ArgsEntry defines a args cache entry
type ArgsEntry struct {
	Values    []string
	Truncated bool
}

// Equals compares two ArgsEntry
func (p *ArgsEntry) Equals(o *ArgsEntry) bool {
	if p == o {
		return true
	} else if p == nil || o == nil {
		return false
	}

	return slices.Equal(p.Values, o.Values)
}

// EnvsEntry defines a args cache entry
type EnvsEntry struct {
	Values    []string
	Truncated bool

	filteredEnvs []string
	kv           map[string]string
}

// FilterEnvs returns an array of envs, only the name of each variable is returned unless the variable name is part of the provided filter
func (p *EnvsEntry) FilterEnvs(envsWithValue map[string]bool) ([]string, bool) {
	if p.filteredEnvs != nil {
		return p.filteredEnvs, p.Truncated
	}

	if len(p.Values) == 0 {
		return nil, p.Truncated
	}

	p.filteredEnvs = make([]string, 0, len(p.Values))

	for _, value := range p.Values {
		k, _, found := strings.Cut(value, "=")
		if found {
			if envsWithValue[k] {
				p.filteredEnvs = append(p.filteredEnvs, value)
			} else {
				p.filteredEnvs = append(p.filteredEnvs, k)
			}
		} else {
			p.filteredEnvs = append(p.filteredEnvs, value)
		}
	}

	return p.filteredEnvs, p.Truncated
}

func (p *EnvsEntry) toMap() {
	if p.kv != nil {
		return
	}

	p.kv = make(map[string]string, len(p.Values))

	for _, value := range p.Values {
		k, v, found := strings.Cut(value, "=")
		if found {
			p.kv[k] = v
		}
	}
}

// Get returns the value for the given key
func (p *EnvsEntry) Get(key string) string {
	p.toMap()
	return p.kv[key]
}

// Equals compares two EnvsEntry
func (p *EnvsEntry) Equals(o *EnvsEntry) bool {
	if p == o {
		return true
	} else if o == nil {
		return false
	}

	return slices.Equal(p.Values, o.Values)
}
