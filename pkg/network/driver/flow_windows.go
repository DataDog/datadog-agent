// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package driver

import (
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
