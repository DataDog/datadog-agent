// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package traps

import (
	"github.com/gosnmp/gosnmp"
	"net"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content   *gosnmp.SnmpPacket
	Addr      *net.UDPAddr
	Namespace string
	Timestamp int64
}

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// GetTags returns a list of tags associated to an SNMP trap packet.
func (p *SnmpPacket) getTags() []string {
	return []string{
		"snmp_version:" + formatVersion(p.Content),
		"device_namespace:" + p.Namespace,
		"snmp_device:" + p.Addr.IP.String(),
	}
}
