// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package model

// SocketContentionEntry identifies one low-cardinality contention bucket and its timings.
type SocketContentionEntry struct {
	ObjectKind string `json:"object_kind"`
	SocketType string `json:"socket_type"`
	Family     string `json:"family"`
	Protocol   string `json:"protocol"`
	LockSubtype string `json:"lock_subtype"`
	CgroupID   uint64 `json:"cgroup_id"`
	Flags      uint32 `json:"flags"`
	TotalTimeNS uint64 `json:"total_time_ns"`
	MinTimeNS   uint64 `json:"min_time_ns"`
	MaxTimeNS   uint64 `json:"max_time_ns"`
	Count       uint64 `json:"count"`
}

// SocketLockIdentity is a test-only view of one registered lock address.
type SocketLockIdentity struct {
	LockAddr    uint64 `json:"lock_addr"`
	SockPtr     uint64 `json:"sock_ptr"`
	SocketCookie uint64 `json:"socket_cookie"`
	CgroupID    uint64 `json:"cgroup_id"`
	Family      string `json:"family"`
	Protocol    string `json:"protocol"`
	SocketType  string `json:"socket_type"`
	LockSubtype string `json:"lock_subtype"`
}

// SocketContentionStats is the exported payload returned by the probe.
type SocketContentionStats []SocketContentionEntry
