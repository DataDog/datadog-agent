// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessinit defines the serverless-init metadata provider.
package serverlessinit

// team: serverless-azure-and-gcp fleet-automation

// Component is the component type.
type Component interface {
	// ReportNow enqueues a payload with the provider's current metadata without
	// waiting for the collection interval. It returns without reporting when
	// metadata is disabled or stopped, when the provider has no resource
	// identity, when collection holds the payload past a short timeout, or when
	// collection sent a payload for the current identity during that wait.
	ReportNow()
}

// Params holds the process-level inputs to the payload.
type Params struct {
	// Sidecar selects the sidecar deployment model. The payload ignores
	// WrappedCommand in sidecar mode.
	Sidecar bool
	// Tags supply dd_env, dd_service, and dd_version.
	Tags map[string]string
	// WrappedCommand is the wrapped application argv. It feeds runtime
	// detection and the opt-in wrapped_command field.
	WrappedCommand []string
}
