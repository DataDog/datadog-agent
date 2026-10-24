// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessinit defines the serverless-init metadata provider.
package serverlessinit

// team: serverless-azure-and-gcp fleet-automation

// Component is the component type.
type Component interface{}

// Params carries the process-level context the embedding binary resolves
// before Fx starts.
type Params struct {
	// Sidecar reports sidecar-container mode. Sidecar argv is not the workload,
	// so the embedding binary supplies no wrapped command in that mode.
	Sidecar bool
	// Tags are the merged telemetry tags; env, service, and version come from
	// here so the payload matches the values this container tags its telemetry
	// with.
	Tags map[string]string
	// WrappedCommand is the in-container wrapped application argv, used for
	// runtime detection and the opt-in wrapped_command field.
	WrappedCommand []string
}
