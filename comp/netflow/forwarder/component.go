// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package forwarder exposes the event platform forwarder.
package forwarder

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/golang/mock/gomock"
)

// team: network-device-monitoring

// Component is the component type.
type Component epforwarder.EventPlatformForwarder

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(getForwarder),
)

func getForwarder(agg aggregator.DemultiplexerWithAggregator) (Component, error) {
	return agg.GetEventPlatformForwarder()
}

// MockComponent is the type for mock components.
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

func getMockForwarder(t testing.TB) MockComponent {
	ctrl := gomock.NewController(t)
	return epforwarder.NewMockEventPlatformForwarder(ctrl)
}
