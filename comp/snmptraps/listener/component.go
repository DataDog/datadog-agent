// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package listener implements a component that listens for SNMP messages,
// parses them, and publishes messages on a channel.
package listener

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	// Start starts the listener
	Start() error
	// Stop stops the listener
	Stop()
	// Packets returns the channel to which the listener publishes traps packets.
	Packets() packet.PacketsChannel
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(NewTrapListener),
)

// MockComponent just holds a channel to which tests can write.
type MockComponent interface {
	Component
	Send(*packet.SnmpPacket)
}

// MockModule provides a MockComponent as the Component.
var MockModule = fxutil.Component(
	fx.Provide(
		newMock,
		func(m MockComponent) Component { return m },
	),
)
