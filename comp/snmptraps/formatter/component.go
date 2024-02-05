// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package formatter provides a component for formatting SNMP traps.
package formatter

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
)

// team: network-device-monitoring

// Component is the component type.
type Component interface {
	FormatPacket(packet *packet.SnmpPacket) ([]byte, error)
}
