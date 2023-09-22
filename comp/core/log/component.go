// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log implements a component to handle logging internal to the agent.
//
// The component uses a number of values in BundleParams to decide how to
// initialize itself, reading values from the comp/core/config component when
// necessary.  At present, it configures and wraps the global logger in
// pkg/util/log, but will eventually be self-sufficient.
//
// The mock component does not read any configuration values, and redirects
// logging output to `t.Log(..)`, for ease of investigation when a test fails.
package log

import (
	"go.uber.org/fx"

	logModule "github.com/DataDog/datadog-agent/comp/core/log/module"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-shared-components

// Component is the component type.
type Component = logModule.Component

// Mock is the mocked component type.
type Mock interface {
	Component

	// no further methods are defined.
}

// Module defines the fx options for this component.
var Module fx.Option = fxutil.Component(
	fx.Provide(newAgentLogger),
)

// MockModule defines the fx options for the mock component.
var MockModule fx.Option = fxutil.Component(
	fx.Provide(newMockLogger),
)
