// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostgpu exposes the interface for the component to generate the 'host_gpu' metadata payload for inventory.
package hostgpu

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// Refresh trigger a new payload to be sent while still respecting the minimal interval between two updates.
	Refresh()
}
