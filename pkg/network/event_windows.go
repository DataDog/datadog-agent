// +build windows

package network

/*
#include "../ebpf/c/ddfilterapi.h"
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

func connDirection(flags C.uint32_t) ConnectionDirection {
	direction := (flags & C.FLOW_DIRECTION_MASK) >> C.FLOW_DIRECTION_BITS
	if (direction & C.FLOW_DIRECTION_INBOUND) == C.FLOW_DIRECTION_INBOUND {
		return INCOMING
	}
	if (direction & C.FLOW_DIRECTION_OUTBOUND) == C.FLOW_DIRECTION_OUTBOUND {
		return OUTGOING
	}
	return NONE
}

func isFlowClosed(flags C.uint32_t) bool {
	// Connection is closed
	if (flags & C.FLOW_CLOSED_MASK) == C.FLOW_CLOSED_MASK {
		return true
	}
	return false
}

func convertV4Addr(addr [16]C.uint8_t) util.Address {
	// We only read the first 4 bytes for v4 address
	return util.V4AddressFromBytes(C.GoBytes(unsafe.Pointer(&addr), net.IPv4len))
}

func convertV6Addr(addr [16]C.uint8_t) util.Address {
	// We read all 16 bytes for v6 address
	return util.V6AddressFromBytes(C.GoBytes(unsafe.Pointer(&addr), net.IPv6len))
}

// FlowToConnStat converts a C.struct__perFlowData into a ConnectionStats struct for use with the tracer
func FlowToConnStat(flow *C.struct__perFlowData) ConnectionStats {
	var (
		family         ConnectionFamily
		srcAddr        util.Address
		dstAddr        util.Address
		connectionType ConnectionType
	)
	family = connFamily(flow.addressFamily)
	connectionType = connType(flow.protocol)

	// V4 Address
	if family == AFINET {
		srcAddr, dstAddr = convertV4Addr(flow.localAddress), convertV4Addr(flow.remoteAddress)
	} else {
		// V6 Address
		srcAddr, dstAddr = convertV6Addr(flow.localAddress), convertV6Addr(flow.remoteAddress)
	}

	return ConnectionStats{
		Source:             srcAddr,
		Dest:               dstAddr,
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
		Direction:            connDirection(flow.flags),
	}
}
