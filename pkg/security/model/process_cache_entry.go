// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package model

import (
	"fmt"
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
	if len(parent.ContainerContext.ID) > 0 && len(child.ContainerContext.ID) == 0 {
		child.ContainerContext.ID = parent.ContainerContext.ID
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

	// keep some context if not present in the ebpf event
	if childEntry.ExecTime.IsZero() {
		childEntry.TTYName = pc.TTYName
		childEntry.Comm = pc.Comm
		childEntry.FileFields = pc.FileFields
		childEntry.PathnameStr = pc.PathnameStr
		childEntry.BasenameStr = pc.BasenameStr
		childEntry.ContainerPath = pc.ContainerPath
		childEntry.ExecTimestamp = pc.ExecTimestamp
		childEntry.Cookie = pc.Cookie

		copyProcessContext(pc, childEntry)
	}
}

func (pc *ProcessCacheEntry) String() string {
	s := fmt.Sprintf("filename: %s[%s] pid:%d ppid:%d\n", pc.PathnameStr, pc.Comm, pc.Pid, pc.PPid)
	ancestor := pc.Ancestor
	for i := 0; ancestor != nil; i++ {
		for j := 0; j <= i; j++ {
			s += "\t"
		}
		s += fmt.Sprintf("filename: %s[%s] pid:%d ppid:%d\n", ancestor.PathnameStr, ancestor.Comm, ancestor.Pid, ancestor.PPid)
		ancestor = ancestor.Ancestor
	}
	return s
}

// UnmarshalBinary reads the binary representation of itself
func (pc *ProcessCacheEntry) UnmarshalBinary(data []byte, unmarshalContext bool) (int, error) {
	var read int

	if unmarshalContext {
		offset, err := UnmarshalBinary(data, &pc.ContainerContext)
		if err != nil {
			return 0, err
		}
		read += offset
	}

	offset, err := pc.ExecEvent.UnmarshalBinary(data[read:])
	if err != nil {
		return 0, err
	}

	return read + offset, nil
}
