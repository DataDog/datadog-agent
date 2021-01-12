// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
)

// Copy returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Copy() *ProcessCacheEntry {
	dup := *pc

	// reset pointers
	dup.Parent = nil
	dup.Children = make(map[uint32]*ProcessCacheEntry)
	return &dup
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
