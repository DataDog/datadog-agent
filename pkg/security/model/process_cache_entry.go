// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package model

import (
	"strings"
	"time"
)

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
		child.ContainerPath = parent.ContainerPath
	}
}

// Exec replace a process
func (pc *ProcessCacheEntry) Exec(entry *ProcessCacheEntry) {
	entry.Ancestor = pc

	// empty and mark as exit previous entry
	pc.ExitTime = entry.ExecTime

	// keep some context
	copyProcessContext(pc, entry)
}

// Fork returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Fork(childEntry *ProcessCacheEntry) {
	childEntry.PPid = pc.Pid
	childEntry.Ancestor = pc
	childEntry.TTYName = pc.TTYName
	childEntry.Comm = pc.Comm
	childEntry.FileFields = pc.FileFields
	childEntry.PathnameStr = pc.PathnameStr
	childEntry.BasenameStr = pc.BasenameStr
	childEntry.Filesystem = pc.Filesystem
	childEntry.ContainerID = pc.ContainerID
	childEntry.ContainerPath = pc.ContainerPath
	childEntry.ExecTime = pc.ExecTime
	childEntry.Credentials = pc.Credentials
	childEntry.Cookie = pc.Cookie

	childEntry.ArgsEntry = pc.ArgsEntry
	childEntry.EnvsEntry = pc.EnvsEntry
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

// ArgsEnvsCacheEntry defines a args/envs base entry
type ArgsEnvsCacheEntry struct {
	ID        uint32
	Size      uint32
	ValuesRaw [128]byte
	Next      *ArgsEnvsCacheEntry
	Last      *ArgsEnvsCacheEntry
}

func (p *ArgsEnvsCacheEntry) toArray() ([]string, bool) {
	entry := p

	var values []string
	var truncated bool

	for entry != nil {
		v, err := UnmarshalStringArray(entry.ValuesRaw[:entry.Size])
		if err != nil || entry.Size == 128 {
			if len(v) > 0 {
				v[len(v)-1] = v[len(v)-1] + "..."
			}
			truncated = true
		}
		if len(v) > 0 {
			values = append(values, v...)
		}

		entry = entry.Next
	}

	return values, truncated
}

// ArgsEntry defines a args cache entry
type ArgsEntry struct {
	*ArgsEnvsCacheEntry

	Values    []string
	Truncated bool
}

// ToArray returns args as array
func (p *ArgsEntry) ToArray() ([]string, bool) {
	if len(p.Values) > 0 {
		return p.Values, p.Truncated
	}
	p.Values, p.Truncated = p.toArray()

	return p.Values, p.Truncated
}

// EnvsEntry defines a args cache entry
type EnvsEntry struct {
	*ArgsEnvsCacheEntry

	Values    map[string]string
	Truncated bool
}

// ToMap returns envs as map
func (p *EnvsEntry) ToMap() (map[string]string, bool) {
	if p.Values != nil {
		return p.Values, p.Truncated
	}

	values, truncated := p.toArray()

	envs := make(map[string]string, len(values))

	for _, env := range values {
		if els := strings.SplitN(env, "=", 2); len(els) == 2 {
			key := els[0]
			value := els[1]
			envs[key] = value
		}
	}
	p.Values, p.Truncated = envs, truncated

	return p.Values, p.Truncated
}

// Get returns the value for the given key
func (p *EnvsEntry) Get(key string) string {
	if p.Values == nil {
		p.ToMap()
	}
	return p.Values[key]
}
