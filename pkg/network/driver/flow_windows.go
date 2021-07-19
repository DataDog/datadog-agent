package driver

import (
	"syscall"
	"unsafe"
)

// TCPFlow returns the TCP-specific flow data
func (f PerFlowData) TCPFlow() *TCPFlowData {
	if f.Protocol == syscall.IPPROTO_TCP {
		return (*TCPFlowData)(unsafe.Pointer(&f.U[0]))
	}
	return nil
}

// UDPFlow returns the UDP-specific flow data
func (f PerFlowData) UDPFlow() *UDPFlowData {
	if f.Protocol == syscall.IPPROTO_UDP {
		return (*UDPFlowData)(unsafe.Pointer(&f.U[0]))
	}
	return nil
}
