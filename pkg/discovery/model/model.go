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
	TCPPorts                 []uint16                        `json:"tcp_ports,omitempty"`
	UDPPorts                 []uint16                        `json:"udp_ports,omitempty"`
	APMInstrumentation       bool                            `json:"apm_instrumentation"`
	Language                 string                          `json:"language"`
	Type                     string                          `json:"service_type"`
	UST                      UST                             `json:"ust"`
	HasNvidiaGPU             bool                            `json:"has_nvidia_gpu"`
}

// UST represents the Unified Service Tagging environment variables of a service.
type UST struct {
	Service string `json:"service"`
	Env     string `json:"env"`
	Version string `json:"version"`
}

// ServicesResponse is the response for the system-probe /discovery/services endpoint.
type ServicesResponse struct {
	Services     []Service `json:"services"`
	InjectedPIDs []int     `json:"injected_pids"`
}
