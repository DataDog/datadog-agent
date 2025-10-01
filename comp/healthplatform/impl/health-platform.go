// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package healthplatformimpl implements the health-platform component interface
package healthplatformimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// Requires defines the dependencies for the health-platform component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the health-platform component
type Provides struct {
	Comp healthplatform.Component
}

// NewComponent creates a new health-platform component
func NewComponent(_ Requires) (Provides, error) {
	// TODO: Implement the health-platform component

	provides := Provides{}
	return provides, nil
}
