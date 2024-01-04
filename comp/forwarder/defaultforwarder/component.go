// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwarder implements a component to send payloads to the backend
package defaultforwarder

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type.
type Component interface {
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Forwarder inside this interface.
	Forwarder
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newForwarder))
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockForwarder))
}
