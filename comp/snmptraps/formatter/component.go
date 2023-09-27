// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package formatter provides a component for formatting SNMP traps.
package formatter

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	FormatPacket(packet *packet.SnmpPacket) ([]byte, error)
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(NewJSONFormatter),
)

// MockModule provides a dummy formatter that just hashes packets.
var MockModule = fxutil.Component(
	fx.Provide(newDummy),
)
