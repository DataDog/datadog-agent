// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// PacketFilter describes a packet filter
type PacketFilter struct {
	expression string
	compiled   *pcap.BPF
}

// CompilePacketFilter returns a new compilted packet filter from the given expression
func CompilePacketFilter(expression string) (*PacketFilter, error) {
	compiled, err := pcap.NewBPF(layers.LinkTypeEthernet, 65535, expression)
	if err != nil {
		return nil, err
	}

	return &PacketFilter{
		expression: expression,
		compiled:   compiled,
	}, nil
}

// Matches returns true if the given packet matches the filter
func (pf *PacketFilter) Matches(pkt Packet) bool {
	ci := pkt.GetCaptureInfo()
	data := pkt.GetData()
	if ci == nil || data == nil {
		return false
	}
	return pf.compiled.Matches(*ci, data)
}

// GetExpression returns the expression of the filter
func (pf *PacketFilter) GetExpression() string {
	return pf.expression
}
