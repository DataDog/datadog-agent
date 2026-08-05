// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package model contains the wire model for NUMA monitoring.
package model

// State is the operational state reported by the system-probe module.
type State string

const (
	StateActive      State = "active"
	StatePartial     State = "partial"
	StateUnsupported State = "unsupported"
	StateError       State = "error"
)

// Status describes the detected NUMA and resctrl capabilities.
type Status struct {
	State                State    `json:"state"`
	Architecture         string   `json:"architecture"`
	NUMANodes            []int    `json:"numa_nodes"`
	MonitorFeatures      []string `json:"monitor_features"`
	ActiveGroups         int      `json:"active_groups"`
	Capacity             int      `json:"capacity"`
	ForeignTaskConflicts uint64   `json:"foreign_task_conflicts"`
	ReadFailures         uint64   `json:"read_failures"`
	Message              string   `json:"message,omitempty"`
}

// DomainStats contains values read from one resctrl monitoring domain.
// Pointer fields distinguish a real zero from an unavailable event.
type DomainStats struct {
	Domain          string   `json:"domain"`
	LLCOccupancy    *float64 `json:"llc_occupancy,omitempty"`
	TotalBandwidth  *float64 `json:"total_bandwidth,omitempty"`
	LocalBandwidth  *float64 `json:"local_bandwidth,omitempty"`
	RemoteBandwidth *float64 `json:"remote_bandwidth,omitempty"`
}

// ContainerStats contains NUMA statistics for a container cgroup.
type ContainerStats struct {
	CgroupID          uint64          `json:"cgroup_id"`
	RuntimeShares     map[int]float64 `json:"runtime_shares,omitempty"`
	ResidentBytes     map[int]uint64  `json:"resident_bytes,omitempty"`
	Domains           []DomainStats   `json:"domains,omitempty"`
	RemoteRatio       *float64        `json:"remote_ratio,omitempty"`
	PlacementMismatch *float64        `json:"placement_mismatch,omitempty"`
	BadnessScore      *float64        `json:"badness_score,omitempty"`
}

// Response is returned by the system-probe check endpoint.
type Response struct {
	Containers []ContainerStats `json:"containers"`
	Status     Status           `json:"status"`
}
