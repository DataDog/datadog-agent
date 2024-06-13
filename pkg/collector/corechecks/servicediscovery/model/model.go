// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model contains the types for the service_discovery check.
package model

// Port represents an open port in the current host.
type Port struct {
	PID         int    `json:"pid"`
	ProcessName string `json:"processName"`
	Port        int    `json:"port"`
	Proto       string `json:"proto"`
}

// OpenPortsResponse is the response for the system-probe /service_discovery/open_ports endpoint.
type OpenPortsResponse struct {
	Ports []*Port `json:"ports"`
}

// Proc represents a system process.
type Proc struct {
	PID     int      `json:"pid"`
	Environ []string `json:"environ"`
	CWD     string   `json:"cwd"`
}

// GetProcResponse is the response for the system-probe /service_discovery/procs/{pid} endpoint.
type GetProcResponse struct {
	Proc *Proc `json:"proc"`
}
