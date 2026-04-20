// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// TCPFlow returns the TCP-specific flow data
func (f PerFlowData) TCPFlow() *TCPFlowData {
	if f.Protocol == syscall.IPPROTO_TCP {
		return (*TCPFlowData)(unsafe.Pointer(&f.Protocol_u[0]))
	}
	return nil
}

// UDPFlow returns the UDP-specific flow data
func (f PerFlowData) UDPFlow() *UDPFlowData {
	if f.Protocol == syscall.IPPROTO_UDP {
		return (*UDPFlowData)(unsafe.Pointer(&f.Protocol_u[0]))
	}
	return nil
}

// GetInterfaceIndex returns the network interface index for this flow.
// The driver USER_FLOW_DATA struct uses #pragma pack(1), which places the
// uint32 interfaceIndex field at an unaligned offset (190). The C header
// declares it as a uint8_t[4] so Go can represent it without alignment
// padding; decode little-endian here.
func (f PerFlowData) GetInterfaceIndex() uint32 {
	return binary.LittleEndian.Uint32(f.InterfaceIndex[:])
}
