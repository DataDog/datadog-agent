// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package packagesigning implements a component to generate the 'signing' metadata payload for DD inventory (REDAPL).
package packagesigning

// team: agent-delivery

// Component is the component type.
type Component interface {
	// GetAsJSON returns the payload as a JSON string. Useful to be displayed in the CLI or added to a flare.
	GetAsJSON() ([]byte, error)
}
