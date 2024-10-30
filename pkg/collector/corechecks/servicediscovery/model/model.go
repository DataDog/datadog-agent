// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package model contains types for service discovery.
package model

// Service represents a listening process.
type Service struct {
	PID                int      `json:"pid"`
	Name               string   `json:"name"`
	GeneratedName      string   `json:"generated_name"`
	DDService          string   `json:"dd_service"`
	DDServiceInjected  bool     `json:"dd_service_injected"`
	Ports              []uint16 `json:"ports"`
	APMInstrumentation string   `json:"apm_instrumentation"`
	Language           string   `json:"language"`
	RSS                uint64   `json:"rss"`
	CommandLine        []string `json:"cmdline"`
	StartTimeMilli     uint64   `json:"start_time"`
	CPUCores           float64  `json:"cpu_cores"`
	ContainerID        string   `json:"container_id"`
}

// ServicesResponse is the response for the system-probe /discovery/services endpoint.
type ServicesResponse struct {
	Services []Service `json:"services"`
}
