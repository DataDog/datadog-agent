// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package model

// NvmlState is the NVML release state pushed to the system-probe GPU
// monitoring module, shared between the core agent (client) and system-probe
// (server).
type NvmlState bool

const (
	// NvmlStateAcquired reports that NVML is (back) in use by the pushing
	// process: the release window is closed.
	NvmlStateAcquired NvmlState = false
	// NvmlStateReleased reports that NVML is deliberately released for a GPU
	// reset window.
	NvmlStateReleased NvmlState = true
)

// NvmlReleaseRequest is the payload of the system-probe GPU monitoring
// module's /nvml-release endpoint.
type NvmlReleaseRequest struct {
	// Released opens (NvmlStateReleased) or closes (NvmlStateAcquired) the
	// NVML release lease.
	Released NvmlState `json:"released"`

	// TTLSeconds is the lease duration the pushing process asks for while
	// released; the agent derives it from its check interval. 0 means the
	// default.
	TTLSeconds int `json:"ttl_seconds"`
}
