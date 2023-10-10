// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package common provides a flow type and a few standard helpers.
package common

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
)

// Flow contains flow info used for aggregation
// json annotations are used in AsJSONString() for debugging purpose
type Flow struct {
	Namespace    string
	FlowType     FlowType
	SequenceNum  uint32
	SamplingRate uint64
	Direction    uint32

	// Exporter information
	ExporterAddr []byte

	// Flow time
	StartTimestamp uint64 // in seconds
	EndTimestamp   uint64 // in seconds

	// Size of the sampled packet
	Bytes   uint64
	Packets uint64

	// Source/destination addresses
	SrcAddr []byte // FLOW KEY
	DstAddr []byte // FLOW KEY

	// Layer 3 protocol (IPv4/IPv6/ARP/MPLS...)
	EtherType uint32

	// Layer 4 protocol
	IPProtocol uint32 // FLOW KEY

	// Flags
	TCPFlags uint32 `json:"tcp_flags"`

	// Ports for UDP and TCP
	// Port number can be zero/positive or `-1` (ephemeral port)
	SrcPort int32 // FLOW KEY
	DstPort int32 // FLOW KEY

	// SNMP Interface Index
	InputInterface  uint32 // FLOW KEY
	OutputInterface uint32

	// Mac Address
	SrcMac uint64
	DstMac uint64

	// Mask
	SrcMask uint32
	DstMask uint32

	// Ethernet information
	Tos uint32 // FLOW KEY

	NextHop []byte // FLOW KEY
}

// AggregationHash return a hash used as aggregation key
func (f *Flow) AggregationHash() uint64 {
	h := fnv.New64()
	h.Write([]byte(f.Namespace))                           //nolint:errcheck
	h.Write(f.ExporterAddr)                                //nolint:errcheck
	h.Write(f.SrcAddr)                                     //nolint:errcheck
	h.Write(f.DstAddr)                                     //nolint:errcheck
	binary.Write(h, binary.LittleEndian, f.SrcPort)        //nolint:errcheck
	binary.Write(h, binary.LittleEndian, f.DstPort)        //nolint:errcheck
	binary.Write(h, binary.LittleEndian, f.IPProtocol)     //nolint:errcheck
	binary.Write(h, binary.LittleEndian, f.Tos)            //nolint:errcheck
	binary.Write(h, binary.LittleEndian, f.InputInterface) //nolint:errcheck
	return h.Sum64()
}

// IsEqualFlowContext check if the flow and another flow have equal values for all fields used in `AggregationHash`.
// This method is used for hash collision detection.
func IsEqualFlowContext(a Flow, b Flow) bool {
	if a.Namespace == b.Namespace &&
		bytes.Equal(a.ExporterAddr, b.ExporterAddr) &&
		bytes.Equal(a.SrcAddr, b.SrcAddr) &&
		bytes.Equal(a.DstAddr, b.DstAddr) &&
		a.SrcPort == b.SrcPort &&
		a.DstPort == b.DstPort &&
		a.IPProtocol == b.IPProtocol &&
		a.Tos == b.Tos &&
		a.InputInterface == b.InputInterface {
		return true
	}
	return false
}
