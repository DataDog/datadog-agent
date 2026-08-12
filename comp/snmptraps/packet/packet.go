// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package packet defines an SNMP packet type and related helpers.
package packet

import (
	"net"

	"github.com/gosnmp/gosnmp"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content   *gosnmp.SnmpPacket
	Addr      *net.UDPAddr
	Namespace string
	Timestamp int64
	Tags      []string
}

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// GetTags returns a list of tags associated to an SNMP trap packet.
func (p *SnmpPacket) GetTags() []string {
	tags := make([]string, 0, 3+len(p.Tags))
	tags = append(tags,
		"snmp_version:"+formatVersion(p.Content),
		"device_namespace:"+p.Namespace,
		"snmp_device:"+p.Addr.IP.String(),
	)
	return append(tags, p.Tags...)
}

func formatVersion(packet *gosnmp.SnmpPacket) string {
	switch packet.Version {
	case gosnmp.Version3:
		return "3"
	case gosnmp.Version2c:
		return "2"
	case gosnmp.Version1:
		return "1"
	default:
		return "unknown"
	}
}
