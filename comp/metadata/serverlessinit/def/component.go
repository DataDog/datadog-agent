// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessinit implements the serverless-init inventory metadata
// payload: it applies the platform's serverless fields to the inventoryagent
// payload and submits the first payload as soon as resource identity is known.
package serverlessinit

// team: team/serverless-azure-and-gcp

// Component is the component type.
type Component interface {
	// InventoryIdentityArrived applies the full serverless field set derived
	// from the provider's current inventory data and submits a payload
	// immediately. Platforms that learn resource identity only at runtime call
	// it from their identity event path; construction-time identity submits
	// without it.
	InventoryIdentityArrived()
}

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
