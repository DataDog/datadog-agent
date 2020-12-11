// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"bytes"
	"fmt"
)

// Copy returns a copy of the current ProcessCacheEntry
func (pc *ProcessCacheEntry) Copy() *ProcessCacheEntry {
	dup := *pc

	// reset pointers
	dup.Parent = nil
	dup.ProcessContext.Parent = nil
	dup.Children = make(map[uint32]*ProcessCacheEntry)
	return &dup
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

func (pc *ProcessCacheEntry) marshalJSON(resolvers *Resolvers, topLevelProcess bool) ([]byte, error) {
	var buf bytes.Buffer

	if !topLevelProcess {
		d, err := pc.ContainerContext.marshalJSON(nil)
		if err != nil {
			return nil, err
		}
		if d != nil && len(d) > 0 {
			fmt.Fprint(&buf, `"container":`)
			buf.Write(d)
			buf.WriteRune(',')
		}

		fmt.Fprintf(&buf, `"user":"%s",`, pc.ProcessContext.ResolveUser(nil))
		fmt.Fprintf(&buf, `"group":"%s",`, pc.ProcessContext.ResolveGroup(nil))
		fmt.Fprintf(&buf, `"uid":%d,`, pc.UID)
		fmt.Fprintf(&buf, `"gid":%d,`, pc.GID)
		fmt.Fprintf(&buf, `"pid":%d,`, pc.Pid)
		fmt.Fprintf(&buf, `"tid":%d,`, pc.Tid)
	}
	fmt.Fprintf(&buf, `"name":"%s",`, pc.Comm)
	fmt.Fprintf(&buf, `"filename":"%s",`, pc.PathnameStr)
	fmt.Fprintf(&buf, `"container_path":"%s",`, pc.ContainerPath)
	fmt.Fprintf(&buf, `"ppid":%d,`, pc.PPid)
	fmt.Fprintf(&buf, `"cookie":%d,`, pc.Cookie)
	fmt.Fprintf(&buf, `"tty":"%s",`, pc.TTYName)
	fmt.Fprintf(&buf, `"inode":%d,`, pc.Inode)
	fmt.Fprintf(&buf, `"mount_id":%d,`, pc.MountID)
	fmt.Fprintf(&buf, `"overlay_numlower":%d,`, pc.OverlayNumLower)
	fmt.Fprintf(&buf, `"fork_timestamp":"%s",`, pc.ForkTimestamp)
	fmt.Fprintf(&buf, `"exec_timestamp":"%s",`, pc.ExecTimestamp)
	fmt.Fprintf(&buf, `"exit_timestamp":"%s"`, pc.ExitTimestamp)
	return buf.Bytes(), nil
}
