// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provider

import "time"

// This file represents a generic type aggregating all container stats.
// All fields are float64 as that's is required by the sender API.
// Common units: nanoseconds, bytes

// ContainerMemStats stores memory statistics.
type ContainerMemStats struct {
	// Common fields
	UsageTotal   *float64
	KernelMemory *float64
	Limit        *float64
	Softlimit    *float64

	// Linux-only fields
	RSS       *float64
	Cache     *float64
	Swap      *float64
	OOMEvents *float64 // Number of events where memory allocation failed

	// Windows-only fields
	PrivateWorkingSet *float64
	CommitBytes       *float64
	CommitPeakBytes   *float64
}

// ContainerCPUStats stores CPU stats.
type ContainerCPUStats struct {
	// Common fields
	Total  *float64
	System *float64
	User   *float64
	Limit  *float64 // Percentage 0-100*numCPU

	// Linux-only fields
	Shares           *float64
	ElapsedPeriods   *float64
	ThrottledPeriods *float64
	ThrottledTime    *float64
}

// DeviceIOStats stores Device IO stats.
type DeviceIOStats struct {
	// Common fields
	ReadBytes       *float64
	WriteBytes      *float64
	ReadOperations  *float64
	WriteOperations *float64
}

// ContainerIOStats store I/O statistics about a container.
type ContainerIOStats struct {
	// Common fields
	ReadBytes       *float64
	WriteBytes      *float64
	ReadOperations  *float64
	WriteOperations *float64

	Devices map[string]DeviceIOStats
}

// ContainerPIDStats stores stats about threads & processes.
type ContainerPIDStats struct {
	// Common fields
	PIDs        []int
	ThreadCount *float64
	ThreadLimit *float64
	OpenFiles   *float64
}

// InterfaceNetStats stores network statistics about a network interface
type InterfaceNetStats struct {
	BytesSent   *float64
	BytesRcvd   *float64
	PacketsSent *float64
	PacketsRcvd *float64
}

// ContainerNetworkStats stores network statistics about a container per interface
type ContainerNetworkStats struct {
	BytesSent               *float64
	BytesRcvd               *float64
	PacketsSent             *float64
	PacketsRcvd             *float64
	Interfaces              map[string]InterfaceNetStats
	NetworkIsolationGroupID *uint64
	UsingHostNetwork        *bool
}

// ContainerStats wraps all container metrics
type ContainerStats struct {
	Timestamp time.Time
	CPU       *ContainerCPUStats
	Memory    *ContainerMemStats
	IO        *ContainerIOStats
	PID       *ContainerPIDStats
}
