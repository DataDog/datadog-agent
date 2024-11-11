// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package model contains the model for the GPU check, with types shared between the system-probe GPU probe and
// the gpu core agent check
package model

// MemoryMetrics contains the memory stats for a given memory type
type MemoryMetrics struct {
	CurrentBytes uint64 `json:"current_bytes"`
	MaxBytes     uint64 `json:"max_bytes"`
}

// UtilizationMetrics contains the GPU stats for a given device and process
type UtilizationMetrics struct {
	UtilizationPercentage float64       `json:"utilization_percentage"`
	Memory                MemoryMetrics `json:"memory"`
}

// Key is the key used to identify a GPUStats object
type Key struct {
	// PID is the process ID
	PID uint32 `json:"pid"`

	// DeviceUUID is the UUID of the device
	DeviceUUID string `json:"device_uuid"`
}

// GPUStats contains the past and current data for all streams, including kernel spans and allocations.
// This is the data structure that is sent to the agent
type GPUStats struct {
	MetricsMap map[Key]UtilizationMetrics `json:"metrics_map"`
}
