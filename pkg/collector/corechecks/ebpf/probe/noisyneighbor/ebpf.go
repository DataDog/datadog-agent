// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package noisyneighbor is for the noisy neighbor module
package noisyneighbor

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type runqEvent struct {
	PrevCgroupID   uint64
	CgroupID       uint64
	RunqLatency    uint64
	Timestamp      uint64
	PrevCgroupName string
	CgroupName     string
	Pid            uint64
	PrevPid        uint64
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler
func (r *runqEvent) UnmarshalBinary(data []byte) error {
	e := (*ebpfRunqEvent)(unsafe.Pointer(&data[0]))
	r.PrevCgroupID = e.Prev_cgroup_id
	r.CgroupID = e.Cgroup_id
	r.RunqLatency = e.Runq_lat
	r.Timestamp = e.Ts
	r.PrevCgroupName = unix.ByteSliceToString(e.Prev_cgroup_name[:])
	r.CgroupName = unix.ByteSliceToString(e.Cgroup_name[:])
	r.Pid = e.Pid
	r.PrevPid = e.Prev_pid
	return nil
}
