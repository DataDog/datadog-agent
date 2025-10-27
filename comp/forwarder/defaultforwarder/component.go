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

// team: agent-metric-pipelines

// Component is the component type.
type Component interface {
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Forwarder inside this interface.
	Forwarder
}

// Module defines the fx options for this component.
func Module(params Params) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newForwarder),
		fx.Supply(params),
	)
}

// ModulWithOptionTMP defines the fx options for this component with an option.
// This is a temporary function to until configsync is cleanup.
func ModulWithOptionTMP(option fx.Option) fxutil.Module {
	return fxutil.Component(
		fx.Provide(newForwarder),
		option,
	)
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

// NoopModule provides a stub forwarder component that does nothing.
func NoopModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() Component { return NoopForwarder{} }))
}
