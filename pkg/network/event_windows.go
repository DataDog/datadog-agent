// +build windows

package network

/*
#include <winsock2.h>
#include "ddnpmapi.h"

uint32_t getTcp_sRTT(PER_FLOW_DATA *pfd)
{
	if(pfd->protocol != IPPROTO_TCP) {
		return 0;
	}
	return (uint32_t)pfd->protocol_u.tcp.sRTT;
}
uint32_t getTcp_rttVariance(PER_FLOW_DATA *pfd)
{
	if(pfd->protocol != IPPROTO_TCP) {
		return 0;
	}
	return (uint32_t)pfd->protocol_u.tcp.rttVariance;
}
uint32_t getTcp_retransmitCount(PER_FLOW_DATA *pfd)
{
	if(pfd->protocol != IPPROTO_TCP) {
		return 0;
	}
	return (uint32_t)pfd->protocol_u.tcp.retransmitCount;
}
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
	TCPProtocol = 6

	// UDPProtocol represents the IANA protocol number for UDP
	UDPProtocol = 17
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
	return (flags & C.FLOW_CLOSED_MASK) == C.FLOW_CLOSED_MASK
}

func isTCPFlowEstablished(flags C.uint32_t) bool {
	return (flags & C.TCP_FLOW_ESTABLISHED_MASK) == C.TCP_FLOW_ESTABLISHED_MASK
}

func convertV4Addr(addr [16]C.uint8_t) util.Address {
	// We only read the first 4 bytes for v4 address
	return util.V4AddressFromBytes((*[16]byte)(unsafe.Pointer(&addr))[:net.IPv4len])
}

func convertV6Addr(addr [16]C.uint8_t) util.Address {
	// We read all 16 bytes for v6 address
	return util.V6AddressFromBytes((*[16]byte)(unsafe.Pointer(&addr))[:net.IPv6len])
}

// Monotonic values include retransmits and headers, while transport does not. We default to using transport
// values and must explicitly enable using monotonic counts in the config. This is consistent with the Linux probe
func monotonicOrTransportBytes(useMonotonicCounts bool, monotonic C.uint64_t, transport C.uint64_t) uint64 {
	if useMonotonicCounts {
		return uint64(monotonic)
	}

	return uint64(transport)
}

// FlowToConnStat converts a C.struct__perFlowData into a ConnectionStats struct for use with the tracer
func FlowToConnStat(cs *ConnectionStats, flow *C.struct__perFlowData, enableMonotonicCounts bool) {
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

	cs.Source = srcAddr
	cs.Dest = dstAddr
	// after lengthy discussion, use the transport bytes in/out.  monotonic
	// RecvBytes/SentBytes includes the size of the IP header and transport
	// header, transportBytes is the raw transport data.  At present,
	// the linux probe only reports the raw transport data.  So do that by default.
	cs.MonotonicSentBytes = monotonicOrTransportBytes(enableMonotonicCounts, flow.monotonicSentBytes, flow.transportBytesOut)
	cs.MonotonicRecvBytes = monotonicOrTransportBytes(enableMonotonicCounts, flow.monotonicRecvBytes, flow.transportBytesIn)
	cs.MonotonicSentPackets = uint64(flow.packetsOut)
	cs.MonotonicRecvPackets = uint64(flow.packetsIn)
	cs.LastUpdateEpoch = uint64(flow.timestamp)
	cs.Pid = uint32(flow.processId)
	cs.SPort = uint16(flow.localPort)
	cs.DPort = uint16(flow.remotePort)
	cs.Type = connectionType
	cs.Family = family
	cs.Direction = connDirection(flow.flags)

	// reset other fields to default values
	cs.NetNS = 0
	cs.IPTranslation = nil
	cs.IntraHost = false
	cs.DNSSuccessfulResponses = 0
	cs.DNSFailedResponses = 0
	cs.DNSTimeouts = 0
	cs.DNSSuccessLatencySum = 0
	cs.DNSFailureLatencySum = 0
	cs.DNSCountByRcode = nil
	cs.LastSentBytes = 0
	cs.LastRecvBytes = 0
	cs.MonotonicRetransmits = 0
	cs.LastRetransmits = 0
	cs.MonotonicTCPEstablished = 0
	cs.LastTCPEstablished = 0
	cs.MonotonicTCPClosed = 0
	cs.LastTCPClosed = 0
	cs.RTT = 0
	cs.RTTVar = 0

	if connectionType == TCP {
		cs.MonotonicRetransmits = uint32(C.getTcp_retransmitCount(flow))
		cs.RTT = uint32(C.getTcp_sRTT(flow))
		cs.RTTVar = uint32(C.getTcp_rttVariance(flow))

		if isTCPFlowEstablished(flow.flags) {
			cs.MonotonicTCPEstablished = 1
		}
		if isFlowClosed(flow.flags) {
			cs.MonotonicTCPClosed = 1
		}
	}
}
