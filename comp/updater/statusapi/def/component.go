// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapi is the installer read-only status api component.
package statusapi

// team: fleet windows-products

// Component is the interface for the installer status api component.
//
// It is empty on purpose: the component's only job is to run the listener for the
// lifetime of the daemon, and exposing Start/Stop would let a second caller start it
// again behind the lifecycle's back.
type Component interface{}

// Status is the payload served by the installer's read-only status API.
//
// This package is the contract between the daemon serving that API and the Agent
// reading it, so it must stay free of installer dependencies: the Agent links it,
// and linking the daemon itself would pull the whole installer into every Agent
// flavor. Only add non-sensitive, read-only fields here — the endpoint is readable
// by the Agent user.
type Status struct {
	// InstallerVersion is the version of the running installer daemon.
	InstallerVersion string `json:"installer_version"`
	// AvailableDiskSpace is the free space, in bytes, on the partition holding
	// the packages directory. It is nil when the daemon could not determine it —
	// distinguishing "unknown" from a genuine zero matters, because zero free
	// bytes is a real precondition failure.
	AvailableDiskSpace *uint64 `json:"available_disk_space,omitempty"`
}
