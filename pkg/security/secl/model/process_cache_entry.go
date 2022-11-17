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
	pc.IsThread = false
	parent.Retain()
}

// GetNextAncestorNoFork returns the first ancestor that is not a fork entry
func (pc *ProcessCacheEntry) GetNextAncestorNoFork() *ProcessCacheEntry {
	if pc.Ancestor == nil {
		return nil
	}

	ancestor := pc.Ancestor
	for ancestor.Ancestor != nil {
		if (ancestor.Ancestor.ExitTime == ancestor.ExecTime || ancestor.Ancestor.ExitTime.IsZero()) && ancestor.Tid == ancestor.Ancestor.Tid {
			// this is a fork entry, move on to the next ancestor
			ancestor = ancestor.Ancestor
		} else {
			break
		}
	}

	return ancestor
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

// SetParent set the parent of a fork child
func (pc *ProcessCacheEntry) SetParent(parent *ProcessCacheEntry) {
	pc.SetAncestor(parent)
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

	childEntry.SetParent(pc)
}

// Equals returns whether process cache entries share the same values for comm and args/envs
func (pc *ProcessCacheEntry) Equals(entry *ProcessCacheEntry) bool {
	return pc.Comm == entry.Comm && pc.ArgsEntry.Equals(entry.ArgsEntry) && pc.EnvsEntry.Equals(entry.EnvsEntry)
}

/*func (pc *ProcessCacheEntry) String() string {
	s := fmt.Sprintf("filename: %s[%s] pid:%d ppid:%d args:%v\n", pc.PathnameStr, pc.Comm, pc.Pid, pc.PPid, pc.ArgsArray)
	ancestor := pc.Ancestor
	for i := 0; ancestor != nil; i++ {
		for j := 0; j <= i; j++ {
			s += "\t"
		}
		s += fmt.Sprintf("filename: %s[%s] pid:%d ppid:%d args:%v\n", ancestor.PathnameStr, ancestor.Comm, ancestor.Pid, ancestor.PPid, ancestor.ArgsArray)
		ancestor = ancestor.Ancestor
	}
	return s
}*/

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

// ToArray returns args as array
func (p *ArgsEntry) ToArray() ([]string, bool) {
	return p.Values, p.Truncated
}

// Equals compares two ArgsEntry
func (p *ArgsEntry) Equals(o *ArgsEntry) bool {
	if p == o {
		return true
	} else if p == nil || o == nil {
		return false
	}

	pa, _ := p.ToArray()
	oa, _ := o.ToArray()

	return slices.Equal(pa, oa)
}

// EnvsEntry defines a args cache entry
type EnvsEntry struct {
	Values    []string
	Truncated bool

	filteredEnvs []string
	kv           map[string]string
}

// ToArray returns envs as an array
func (p *EnvsEntry) ToArray() ([]string, bool) {
	return p.Values, p.Truncated
}

// FilterEnvs returns an array of envs, only the name of each variable is returned unless the variable name is part of the provided filter
func (p *EnvsEntry) FilterEnvs(envsWithValue map[string]bool) ([]string, bool) {
	if p.filteredEnvs != nil {
		return p.filteredEnvs, p.Truncated
	}

	values, _ := p.ToArray()
	if len(values) == 0 {
		return nil, p.Truncated
	}

	p.filteredEnvs = make([]string, len(values))

	var i int
	for _, value := range values {
		k, _, found := strings.Cut(value, "=")
		if !found {
			continue
		}

		if envsWithValue[k] {
			p.filteredEnvs[i] = value
		} else {
			p.filteredEnvs[i] = k
		}
		i++
	}

	return p.filteredEnvs, p.Truncated
}

func (p *EnvsEntry) toMap() {
	if p.kv != nil {
		return
	}

	values, _ := p.ToArray()
	p.kv = make(map[string]string, len(values))

	for _, value := range values {
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

	pa, _ := p.ToArray()
	oa, _ := o.ToArray()

	return slices.Equal(pa, oa)
}
