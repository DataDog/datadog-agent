// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/tinylib/msgp -tests=false

package model

import (
	"container/list"
	"strings"
	"time"
)

const (
	maxArgEnvSize = 256
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

// ShareArgsEnvs share args and envs between the current entry and the given child entry
func (pc *ProcessCacheEntry) ShareArgsEnvs(childEntry *ProcessCacheEntry) {
	childEntry.ArgsEntry = pc.ArgsEntry
	if childEntry.ArgsEntry != nil && childEntry.ArgsEntry.ArgsEnvsCacheEntry != nil {
		childEntry.ArgsEntry.ArgsEnvsCacheEntry.Retain()
	}
	childEntry.EnvsEntry = pc.EnvsEntry
	if childEntry.EnvsEntry != nil && childEntry.EnvsEntry.ArgsEnvsCacheEntry != nil {
		childEntry.EnvsEntry.ArgsEnvsCacheEntry.Retain()
	}
}

// SetParent set the parent of a fork child
func (pc *ProcessCacheEntry) SetParent(parent *ProcessCacheEntry) {
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
//msgp:ignore ArgsEnvs
type ArgsEnvs struct {
	ID        uint32
	Size      uint32
	ValuesRaw [maxArgEnvSize]byte
}

// ArgsEnvsCacheEntry defines a args/envs base entry
//msgp:ignore ArgsEnvsCacheEntry
type ArgsEnvsCacheEntry struct {
	Size      uint32
	ValuesRaw []byte

	Container *list.Element

	next *ArgsEnvsCacheEntry
	last *ArgsEnvsCacheEntry

	refCount  uint64
	onRelease func(_ *ArgsEnvsCacheEntry)
}

// Reset the entry
func (p *ArgsEnvsCacheEntry) release() {
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
	if p.last != nil {
		p.last.next = entry
	} else {
		p.next = entry
	}
	p.last = entry
}

// Retain increment ref counter
func (p *ArgsEnvsCacheEntry) Retain() {
	p.refCount++
}

// Release decrement and eventually release the entry
func (p *ArgsEnvsCacheEntry) Release() {
	p.refCount--
	if p.refCount > 0 {
		return
	}

	p.release()
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
		if err != nil || entry.Size == maxArgEnvSize {
			if len(v) > 0 {
				v[len(v)-1] = v[len(v)-1] + "..."
			}
			truncated = true
		}
		if len(v) > 0 {
			values = append(values, v...)
		}
		entry.ValuesRaw = nil

		entry = entry.next
	}

	return values, truncated
}

func stringArraysEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// ArgsEntry defines a args cache entry
type ArgsEntry struct {
	*ArgsEnvsCacheEntry `msg:"-"`

	Values    []string `msg:"values"`
	Truncated bool     `msg:"-"`

	parsed bool
}

// ToArray returns args as array
func (p *ArgsEntry) ToArray() ([]string, bool) {
	if len(p.Values) > 0 || p.parsed {
		return p.Values, p.Truncated
	}
	p.Values, p.Truncated = p.toArray()
	p.parsed = true

	// now we have the cache we can free
	if p.ArgsEnvsCacheEntry != nil {
		p.release()
		p.ArgsEnvsCacheEntry = nil
	}

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

	return stringArraysEqual(pa, oa)
}

// EnvsEntry defines a args cache entry
type EnvsEntry struct {
	*ArgsEnvsCacheEntry `msg:"-"`

	Values    []string `msg:"values"`
	Truncated bool     `msg:"-"`

	parsed bool
	keys   []string
	kv     map[string]string
}

// ToArray returns envs as an array
func (p *EnvsEntry) ToArray() ([]string, bool) {
	if p.parsed {
		return p.Values, p.Truncated
	}

	p.Values, p.Truncated = p.toArray()
	p.parsed = true

	// now we have the cache we can free
	if p.ArgsEnvsCacheEntry != nil {
		p.release()
		p.ArgsEnvsCacheEntry = nil
	}

	return p.Values, p.Truncated
}

// Keys returns only keys
func (p *EnvsEntry) Keys() ([]string, bool) {
	if p.keys != nil {
		return p.keys, p.Truncated
	}

	values, _ := p.ToArray()
	if len(values) == 0 {
		return nil, p.Truncated
	}

	p.keys = make([]string, len(values))

	var i int
	for _, value := range values {
		kv := strings.SplitN(value, "=", 2)
		if len(kv) != 2 {
			continue
		}

		p.keys[i] = kv[0]
		i++
	}

	return p.keys, p.Truncated
}

func (p *EnvsEntry) toMap() {
	if p.kv != nil {
		return
	}

	values, _ := p.ToArray()
	p.kv = make(map[string]string, len(values))

	for _, value := range values {
		kv := strings.SplitN(value, "=", 2)
		k := kv[0]

		if len(kv) == 2 {
			p.kv[k] = kv[1]
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

	return stringArraysEqual(pa, oa)
}
