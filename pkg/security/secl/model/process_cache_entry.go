// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"container/list"
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

// ShareArgsEnvs share args and envs between the current entry and the given child entry
func (pc *ProcessCacheEntry) ShareArgsEnvs(childEntry *ProcessCacheEntry) {
	childEntry.ArgsEntry = pc.ArgsEntry
	if childEntry.ArgsEntry != nil {
		childEntry.ArgsEntry.Retain()
	}
	childEntry.EnvsEntry = pc.EnvsEntry
	if childEntry.EnvsEntry != nil {
		childEntry.EnvsEntry.Retain()
	}
}

// SetParentOfForkChild set the parent of a fork child
func (pc *ProcessCacheEntry) SetParentOfForkChild(parent *ProcessCacheEntry) {
	pc.SetAncestor(parent)
	parent.ShareArgsEnvs(pc)
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

// ArgsEnvs raw value for args and envs
type ArgsEnvs struct {
	ID        uint32
	Size      uint32
	ValuesRaw [MaxArgEnvSize]byte
}

// ArgsEnvsCacheEntry defines a args/envs base entry
type ArgsEnvsCacheEntry struct {
	Size      uint32
	ValuesRaw []byte

	TotalSize uint64

	Container *list.Element

	next *ArgsEnvsCacheEntry
	last *ArgsEnvsCacheEntry

	refCount  uint64
	onRelease func(_ *ArgsEnvsCacheEntry)
}

// Reset the entry
func (p *ArgsEnvsCacheEntry) forceReleaseAll() {
	entry := p
	for entry != nil {
		next := entry.next

		entry.Size = 0
		entry.ValuesRaw = nil
		entry.next = nil
		entry.last = nil
		entry.refCount = 0

		// all the element of the list need to return to the
		// pool
		if p.onRelease != nil {
			p.onRelease(entry)
		}

		entry = next
	}
}

// Append an entry to the list
func (p *ArgsEnvsCacheEntry) Append(entry *ArgsEnvsCacheEntry) {
	p.TotalSize += uint64(entry.Size)

	// this shouldn't happen, but is here to protect against infinite loops
	entry.next = nil
	entry.last = nil

	if p.last != nil {
		p.last.next = entry
	} else {
		p.next = entry
	}
	p.last = entry
}

// Retain increment ref counter
func (p *ArgsEnvsCacheEntry) retain() {
	p.refCount++
}

// Release decrement and eventually release the entry
func (p *ArgsEnvsCacheEntry) release() bool {
	p.refCount--
	if p.refCount > 0 {
		return false
	}

	p.forceReleaseAll()

	return true
}

// NewArgsEnvsCacheEntry returns a new args/env cache entry
func NewArgsEnvsCacheEntry(onRelease func(_ *ArgsEnvsCacheEntry)) *ArgsEnvsCacheEntry {
	entry := &ArgsEnvsCacheEntry{
		onRelease: onRelease,
	}

	return entry
}

func (p *ArgsEnvsCacheEntry) toArray() ([]string, bool) {
	entry := p

	var values []string
	var truncated bool

	for entry != nil {
		v, err := UnmarshalStringArray(entry.ValuesRaw[:entry.Size])
		if err != nil || entry.Size == MaxArgEnvSize {
			if len(v) > 0 {
				v[len(v)-1] = v[len(v)-1] + "..."
			}
			truncated = true
		}
		if len(v) > 0 {
			values = append(values, v...)
		}

		entry = entry.next
	}

	return values, truncated
}

// ArgsEntry defines a args cache entry
type ArgsEntry struct {
	cacheEntry *ArgsEnvsCacheEntry

	values    []string
	truncated bool

	parsed bool
}

// NewEnvsEntry returns a new entry
func NewArgsEntry(cacheEntry *ArgsEnvsCacheEntry) *ArgsEntry {
	return &ArgsEntry{
		cacheEntry: cacheEntry,
	}
}

// SetValues set the values
func (p *ArgsEntry) SetValues(values []string) {
	p.values = values
	p.parsed = true
}

// Retain increment ref counter
func (p *ArgsEntry) Retain() {
	if p.cacheEntry != nil {
		p.cacheEntry.retain()
	}
}

// Release decrement and eventually release the entry
func (p *ArgsEntry) Release() {
	if p.cacheEntry != nil && p.cacheEntry.release() {
		p.cacheEntry = nil
	}
}

// ToArray returns args as array
func (p *ArgsEntry) ToArray() ([]string, bool) {
	if len(p.values) > 0 || p.parsed {
		return p.values, p.truncated
	}
	p.values, p.truncated = p.cacheEntry.toArray()
	p.parsed = true

	// now we have the cache we can force the free without having to check the refcount
	if p.cacheEntry != nil {
		p.cacheEntry.forceReleaseAll()
		p.cacheEntry = nil
	}

	return p.values, p.truncated
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
	cacheEntry *ArgsEnvsCacheEntry

	values    []string
	truncated bool

	parsed       bool
	filteredEnvs []string
	kv           map[string]string
}

// NewEnvsEntry returns a new entry
func NewEnvsEntry(cacheEntry *ArgsEnvsCacheEntry) *EnvsEntry {
	return &EnvsEntry{
		cacheEntry: cacheEntry,
	}
}

// SetValues set the values
func (p *EnvsEntry) SetValues(values []string) {
	p.values = values
	p.parsed = true
}

// Retain increment ref counter
func (p *EnvsEntry) Retain() {
	if p.cacheEntry != nil {
		p.cacheEntry.retain()
	}
}

// Release decrement and eventually release the entry
func (p *EnvsEntry) Release() {
	if p.cacheEntry != nil && p.cacheEntry.release() {
		p.cacheEntry = nil
	}
}

// ToArray returns envs as an array
func (p *EnvsEntry) ToArray() ([]string, bool) {
	if p.parsed {
		return p.values, p.truncated
	}

	p.values, p.truncated = p.cacheEntry.toArray()
	p.parsed = true

	// now we have the cache we can force the free without having to check the refcount
	if p.cacheEntry != nil {
		p.cacheEntry.forceReleaseAll()
		p.cacheEntry = nil
	}

	return p.values, p.truncated
}

// FilterEnvs returns an array of envs, only the name of each variable is returned unless the variable name is part of the provided filter
func (p *EnvsEntry) FilterEnvs(envsWithValue map[string]bool) ([]string, bool) {
	if p.filteredEnvs != nil {
		return p.filteredEnvs, p.truncated
	}

	values, _ := p.ToArray()
	if len(values) == 0 {
		return nil, p.truncated
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

	return p.filteredEnvs, p.truncated
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
