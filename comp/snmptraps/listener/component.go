// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package listener implements a component that listens for SNMP messages,
// parses them, and publishes messages on a channel.
package listener

import (
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
)

// team: ndm-core

// Component is the component type.
type Component interface {
	// Packets returns the channel to which the listener publishes traps packets.
	Packets() packet.PacketsChannel
}
