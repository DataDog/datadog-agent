// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements a component to run data collection checks in the Process Agent.
package server

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Component is the component type.
type Component interface {
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
// fx.Provide(newRunner),
)

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
// fx.Provide(newMock),
)
