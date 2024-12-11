// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryagent implements a component to generate the 'datadog_agent' metadata payload for inventory.
package inventoryagent

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// Set updates a metadata value in the payload. The given value will be stored in the cache without being copied. It is
	// up to the caller to make sure the given value will not be modified later.
	Set(name string, value interface{})
	// GetAsJSON returns the payload as a JSON string. Useful to be displayed in the CLI or added to a flare.
	GetAsJSON() ([]byte, error)
	// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
	Get() map[string]interface{}
}
