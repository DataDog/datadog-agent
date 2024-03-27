// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test
// +build test

// Package workloadmeta provides the workloadmeta component for the Datadog Agent
package workloadmeta

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: container-integrations

// Mock implements mock-specific methods.
type Mock interface {
	Component

	// The following are for testing purposes and should maybe be revisited
	// Set allows setting an entity in the workloadmeta store
	Set(entity Entity)

	// Unset removes an entity from the workloadmeta store
	Unset(entity Entity)

	// GetConfig returns a Config Reader for the internal injected config
	GetConfig() config.Reader

	// GetConfig returns a Config Reader for the internal injected config
	GetNotifiedEvents() []CollectorEvent

	// SubscribeToEvents returns a channel that receives events
	SubscribeToEvents() chan CollectorEvent
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newWorkloadMetaMock),
		fx.Provide(func(mock Mock) Component { return mock }),
		fx.Provide(func(mock Mock) optional.Option[Component] { return optional.NewOption[Component](mock) }),
	)
}

// TODO(components): For consistency, let's add an isV2 field to
//                   Params, and leverage that in the constructor
//                   to return the right implementation.

// MockModuleV2 defines the fx options for the mock component.
func MockModuleV2() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newWorkloadMetaMockV2),
		fx.Provide(func(mock Mock) Component { return mock }),
		fx.Provide(func(mock Mock) optional.Option[Component] { return optional.NewOption[Component](mock) }),
	)
}
