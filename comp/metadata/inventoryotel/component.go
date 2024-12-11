// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryotel implements a component to generate the 'datadog_agent' metadata payload for inventory.
package inventoryotel

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// GetAsJSON returns the payload as a JSON string. Useful to be displayed in the CLI or added to a flare.
	GetAsJSON() ([]byte, error)
	// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
	Get() map[string]interface{}
}
