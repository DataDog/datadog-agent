// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package model contains types for service discovery.
package model

import (
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
)

// Service represents a listening process.
type Service struct {
	PID                      int                             `json:"pid"`
	LogFiles                 []string                        `json:"log_files,omitempty"`
	GeneratedName            string                          `json:"generated_name"`
	GeneratedNameSource      string                          `json:"generated_name_source"`
	AdditionalGeneratedNames []string                        `json:"additional_generated_names"`
	TracerMetadata           []tracermetadata.TracerMetadata `json:"tracer_metadata,omitempty"`
	DDService                string                          `json:"dd_service"`
	DDServiceInjected        bool                            `json:"dd_service_injected"`
	Ports                    []uint16                        `json:"ports"`
	APMInstrumentation       string                          `json:"apm_instrumentation"`
	Language                 string                          `json:"language"`
	Type                     string                          `json:"service_type"`
	CommandLine              []string                        `json:"cmdline"`
}

// ServicesResponse is the response for the system-probe /discovery/services endpoint.
type ServicesResponse struct {
	Services []Service `json:"services"`
}

// NetworkStatsResponse is the response for the system-probe /discovery/network-stats endpoint.
type NetworkStatsResponse struct {
	Stats map[int]NetworkStats `json:"stats"`
}

// NetworkStats contains network statistics for a process.
type NetworkStats struct {
	RxBytes uint64 `json:"rx_bytes"`
	TxBytes uint64 `json:"tx_bytes"`
}
