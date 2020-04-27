// +build windows

package ebpf

/*
#include "c/ddfilterapi.h"
*/
import "C"
import (
	"net"
	"syscall"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

const (
	// TCPProtocol represents the IANA protocol number for TCP
	TCPProtocol = 17

	// UDPProtocol represents the IANA protocol number for UDP
	UDPProtocol = 6

	// TCPFlowDataLen represents the bytes filled for the protocol_u struct in _perFlowData.
	TCPFlowDataLen = 24

	// UDPFlowDataLen represents the bytes filled for the protocol_u struct in _perFlowData
	UDPFlowDataLen = 4
)

func connFamily(addressFamily C.uint16_t) ConnectionFamily {
	if addressFamily == syscall.AF_INET {
		return AFINET
	}
	return AFINET6
}

func connType(protocol C.uint16_t) ConnectionType {
	if protocol == TCPProtocol {
		return TCP
	}
	return UDP
}

func convertV4Addresses(local [16]C.uint8_t, remote [16]C.uint8_t) (localAddress util.Address, remoteAddress util.Address) {
	// We only read the first 4 bytes for v4 address
	localAddress = util.V4AddressFromBytes(C.GoBytes(unsafe.Pointer(&local), net.IPv4len))
	remoteAddress = util.V4AddressFromBytes(C.GoBytes(unsafe.Pointer(&remote), net.IPv4len))
	return
}

func convertV6Addresses(local [16]C.uint8_t, remote [16]C.uint8_t) (localAddress util.Address, remoteAddress util.Address) {
	// We read all 16 bytes for v6 address
	localAddress = util.V6AddressFromBytes(C.GoBytes(unsafe.Pointer(&local), net.IPv6len))
	remoteAddress = util.V6AddressFromBytes(C.GoBytes(unsafe.Pointer(&remote), net.IPv6len))
	return
}

func flowToConnStat(flow *C.struct__perFlowData) (connStat ConnectionStats) {
	var (
		family         ConnectionFamily
		source         util.Address
		dest           util.Address
		connectionType ConnectionType
	)
	family = connFamily(flow.addressFamily)
	connectionType = connType(flow.protocol)

	// V4 Address
	if family == AFINET {
		source, dest = convertV4Addresses(flow.localAddress, flow.remoteAddress)
	} else {
		// V6 Address
		source, dest = convertV6Addresses(flow.localAddress, flow.remoteAddress)
	}

	return ConnectionStats{
		Source:             source,
		Dest:               dest,
		MonotonicSentBytes: uint64(flow.monotonicSentBytes),
		MonotonicRecvBytes: uint64(flow.monotonicRecvBytes),
		LastUpdateEpoch:    0,
		// TODO: Driver needs to be updated to get retransmit values
		MonotonicRetransmits: 0,
		RTT:                  0,
		RTTVar:               0,
		Pid:                  uint32(flow.processId),
		SPort:                uint16(flow.localPort),
		DPort:                uint16(flow.remotePort),
		Type:                 connectionType,
		Family:               family,
		// TODO: Driver needs to be updated to send Direction
		Direction: 0,
	}
}

func flowsToConnStats(pfds []*C.struct__perFlowData) (connStats []ConnectionStats) {
	for _, pfd := range pfds {
		connStats = append(connStats, flowToConnStat(pfd))
	}
	return
}
