// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder exposes the event platform forwarder for netflow.
package forwarder

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	epforwarder.EventPlatformForwarder
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(getForwarder),
)

// MockComponent is the type for mock components.
// It is a gomock-generated mock of EventPlatformForwarder.
type MockComponent interface {
	Component
	EXPECT() *epforwarder.MockEventPlatformForwarderMockRecorder
}

// MockModule defines a component with a mock forwarder
var MockModule = fxutil.Component(
	fx.Provide(
		getMockForwarder,
		// Provide the mock as the primary component as well
		func(c MockComponent) Component { return c },
	),
)
