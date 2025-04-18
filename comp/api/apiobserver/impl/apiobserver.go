// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package apiobserverimpl implements the apiobserver component interface
package apiobserverimpl

import (
	apiobserver "github.com/DataDog/datadog-agent/comp/api/apiobserver/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the apiobserver component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the apiobserver component
type Provides struct {
	Comp apiobserver.Component
}

// NewComponent creates a new apiobserver component
func NewComponent(reqs Requires) (Provides, error) {
	// TODO: Implement the apiobserver component

	provides := Provides{}
	return provides, nil
}
