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
	PID                        int                             `json:"pid"`
	LogFiles                   []string                        `json:"log_files,omitempty"`
	GeneratedName              string                          `json:"generated_name"`
	GeneratedNameSource        string                          `json:"generated_name_source"`
	AdditionalGeneratedNames   []string                        `json:"additional_generated_names"`
	ContainerServiceName       string                          `json:"container_service_name"`
	ContainerServiceNameSource string                          `json:"container_service_name_source"`
	ContainerTags              []string                        `json:"container_tags,omitempty"`
	TracerMetadata             []tracermetadata.TracerMetadata `json:"tracer_metadata,omitempty"`
	DDService                  string                          `json:"dd_service"`
	DDServiceInjected          bool                            `json:"dd_service_injected"`
	CheckedContainerData       bool                            `json:"checked_container_data"`
	Ports                      []uint16                        `json:"ports"`
	APMInstrumentation         string                          `json:"apm_instrumentation"`
	Language                   string                          `json:"language"`
	Type                       string                          `json:"service_type"`
	RSS                        uint64                          `json:"rss"`
	CommandLine                []string                        `json:"cmdline"`
	StartTimeMilli             uint64                          `json:"start_time"`
	CPUCores                   float64                         `json:"cpu_cores"`
	ContainerID                string                          `json:"container_id"`
	LastHeartbeat              int64                           `json:"last_heartbeat"`
	RxBytes                    uint64                          `json:"rx_bytes"`
	TxBytes                    uint64                          `json:"tx_bytes"`
	RxBps                      float64                         `json:"rx_bps"`
	TxBps                      float64                         `json:"tx_bps"`
}

// ServicesResponse is the response for the system-probe /discovery/check endpoint.
type ServicesResponse struct {
	StartedServices      []Service `json:"started_services"`
	StoppedServices      []Service `json:"stopped_services"`
	HeartbeatServices    []Service `json:"heartbeat_services"`
	RunningServicesCount int       `json:"running_services_count"`
}

// ServicesEndpointResponse is the response for the system-probe /discovery/services endpoint.
type ServicesEndpointResponse struct {
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
