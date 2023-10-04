// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config implements a component to handle trace-agent configuration.  This
// component temporarily wraps pkg/trace/config.
//
// This component initializes pkg/config based on the bundle params, and
// will return the same results as that package.  This is to support migration
// to a component architecture.  When no code still uses pkg/config, that
// package will be removed.
//
// The mock component does nothing at startup, beginning with an empty config.
// It also overwrites the pkg/config.SystemProbe for the duration of the test.
package config

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-apm

// Component is the component type.
type Component interface {
	// Warnings returns config warnings collected during setup.
	Warnings() *config.Warnings

	// SetHandler sets http handler for config
	SetHandler() http.Handler

	// SetMaxMemCPU
	SetMaxMemCPU(isContainerized bool)

	// Object returns wrapped config
	Object() *traceconfig.AgentConfig
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newConfig),
	fx.Supply(Params{
		FailIfAPIKeyMissing: true,
	}),
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMock),
	fx.Supply(Params{
		FailIfAPIKeyMissing: true,
	}),
)
