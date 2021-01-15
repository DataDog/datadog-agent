// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
)

// Fork returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Fork(childEntry *ProcessCacheEntry) {
	pid := childEntry.Pid
	*childEntry = *pc
	childEntry.Pid = pid
	childEntry.PPid = pc.Pid
	childEntry.Parent = pc
	childEntry.Children = make(map[uint32]*ProcessCacheEntry)
	pc.Children[pid] = childEntry

	// inherit the container ID from the parent if necessary. If a container is already running when system-probe
	// starts, the in-kernel process cache will have out of sync container ID values for the processes of that
	// container (the snapshot doesn't update the in-kernel cache with the container IDs). This can also happen if
	// the proc_cache LRU ejects an entry.
	// WARNING: this is why the user space cache should not be used to detect container breakouts. Dedicated
	// in-kernel probes will need to be added.
	if len(pc.ContainerContext.ID) > 0 && len(childEntry.ContainerContext.ID) == 0 {
		childEntry.ContainerContext.ID = pc.ContainerContext.ID
	}
}

// IsEqual return whether entries are equals
func (pc *ProcessCacheEntry) IsEqual(e *ProcessCacheEntry) bool {
	return e != nil && e.Pid == pc.Pid && e.ForkTimestamp == pc.ForkTimestamp
}

func (pc *ProcessCacheEntry) String() string {
	s := fmt.Sprintf("filename: %s pid:%d ppid:%d\n", pc.FileEvent.PathnameStr, pc.Pid, pc.PPid)
	parent := pc.Parent
	for i := 0; parent != nil; i++ {
		for j := 0; j <= i; j++ {
			s += "\t"
		}
		s += fmt.Sprintf("filename: %s pid:%d ppid:%d\n", parent.FileEvent.PathnameStr, parent.Pid, parent.PPid)
		parent = parent.Parent
	}
	return s
}

// UnmarshalBinary reads the binary representation of itself
func (pc *ProcessCacheEntry) UnmarshalBinary(data []byte, resolvers *Resolvers, unmarshalContext bool) (int, error) {
	var read int

	if unmarshalContext {
		if len(data) < 200 {
			return 0, ErrNotEnoughData
		}

		offset, err := unmarshalBinary(data, &pc.ContainerContext)
		if err != nil {
			return 0, err
		}
		read += offset
	} else {
		if len(data) < 136 {
			return 0, ErrNotEnoughData
		}
	}

	offset, err := pc.ExecEvent.UnmarshalBinary(data[read:], resolvers)
	if err != nil {
		return 0, err
	}

	return read + offset, nil
}
