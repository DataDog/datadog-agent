// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forwarders implements a component to provide forwarders used by the process agent.
package forwarders

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: processes

// Component exported type should have comment or be unexported
type Component interface {
	GetEventForwarder() defaultforwarder.Component
	GetProcessForwarder() defaultforwarder.Component
	GetRTProcessForwarder() defaultforwarder.Component
	GetConnectionsForwarder() defaultforwarder.Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newForwarders),
)

// MockModule exported var should have comment or be unexported
var MockModule = fxutil.Component(
	fx.Provide(newMockForwarders),
)
