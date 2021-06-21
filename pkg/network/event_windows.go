// +build windows

package network

import (
	"bytes"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/driver"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var (
	ephemeralRanges = map[ConnectionFamily]map[ConnectionType]map[string]uint16{
		AFINET: {
			UDP: {
				"lo": 0,
				"hi": 0,
			},
			TCP: {
				"lo": 0,
				"hi": 0,
			},
		},
		AFINET6: {
			UDP: {
				"lo": 0,
				"hi": 0,
			},
			TCP: {
				"lo": 0,
				"hi": 0,
			},
		},
	}
)

func init() {
	var families = [...]ConnectionFamily{AFINET, AFINET6}
	var protos = [...]ConnectionType{UDP, TCP}
	for _, f := range families {
		for _, p := range protos {
			l, h, err := getEphemeralRange(f, p)
			if err == nil {
				ephemeralRanges[f][p]["lo"] = l
				ephemeralRanges[f][p]["hi"] = h
			}
		}
	}
}
func getEphemeralRange(f ConnectionFamily, t ConnectionType) (low, hi uint16, err error) {
	var protoarg string
	var familyarg string
	switch f {
	case AFINET6:
		familyarg = "ipv6"
	default:
		familyarg = "ipv4"
	}
	switch t {
	case TCP:
		protoarg = "tcp"
	default:
		protoarg = "udp"
	}
	cmd := exec.Command("netsh", "int", familyarg, "show", "dynamicport", protoarg)
	cmdOutput := &bytes.Buffer{}
	// output should be of the format
	/*
		Protocol tcp Dynamic Port Range
		---------------------------------
		Start Port      : 49000
		Number of Ports : 16000
	*/
	cmd.Stdout = cmdOutput
	err = cmd.Run()
	if err != nil {
		return
	}
	output := cmdOutput.Bytes()
	var startPortLine = regexp.MustCompile(`Start.*: (\d+)`)
	var numberLine = regexp.MustCompile(`Number.*: (\d+)`)

	startPort := startPortLine.FindStringSubmatch(string(output))
	rangeLen := numberLine.FindStringSubmatch(string(output))

	portstart, err := strconv.Atoi(startPort[1])
	if err != nil {
		return
	}
	plen, err := strconv.Atoi(rangeLen[1])
	if err != nil {
		return
	}
	// argh.  Windows defaults to
	/*
	 Protocol tcp Dynamic Port Range
	 ---------------------------------
	 Start Port      : 49152
	 Number of Ports : 16384

	 A quick bit of arithmetic says that adds up to 65536, which overflows the "hi" field.
	 A bit of hackery to compensate
	*/
	low = uint16(portstart)
	if portstart+plen > 0xFFFF {
		hi = uint16(0xFFFF)
	} else {
		hi = uint16(portstart + plen)
	}
	return
}

func isPortInEphemeralRange(f ConnectionFamily, t ConnectionType, p uint16) EphemeralPortType {
	rangeLow := ephemeralRanges[f][t]["lo"]
	rangeHi := ephemeralRanges[f][t]["hi"]
	if rangeLow == 0 || rangeHi == 0 {
		return EphemeralUnknown
	}
	if p >= rangeLow && p <= rangeHi {
		return EphemeralTrue
	}
	return EphemeralFalse
}
func connFamily(addressFamily uint16) ConnectionFamily {
	if addressFamily == syscall.AF_INET {
		return AFINET
	}
	return AFINET6
}

func connType(protocol uint16) ConnectionType {
	if protocol == syscall.IPPROTO_TCP {
		return TCP
	}
	return UDP
}

func connDirection(flags uint32) ConnectionDirection {
	direction := (flags & driver.FlowDirectionMask) >> driver.FlowDirectionBits
	if (direction & driver.FlowDirectionInbound) == driver.FlowDirectionInbound {
		return INCOMING
	}
	if (direction & driver.FlowDirectionOutbound) == driver.FlowDirectionOutbound {
		return OUTGOING
	}
	return OUTGOING
}

func isFlowClosed(flags uint32) bool {
	// Connection is closed
	return (flags & driver.FlowClosedMask) == driver.FlowClosedMask
}

func isTCPFlowEstablished(flags uint32) bool {
	return (flags & driver.TCPFlowEstablishedMask) == driver.TCPFlowEstablishedMask
}

func convertV4Addr(addr [16]uint8) util.Address {
	// We only read the first 4 bytes for v4 address
	return util.V4AddressFromBytes(addr[:net.IPv4len])
}

func convertV6Addr(addr [16]uint8) util.Address {
	// We read all 16 bytes for v6 address
	return util.V6AddressFromBytes(addr[:net.IPv6len])
}

// Monotonic values include retransmits and headers, while transport does not. We default to using transport
// values and must explicitly enable using monotonic counts in the config. This is consistent with the Linux probe
func monotonicOrTransportBytes(useMonotonicCounts bool, monotonic uint64, transport uint64) uint64 {
	if useMonotonicCounts {
		return monotonic
	}
	return transport
}

// FlowToConnStat converts a driver.PerFlowData into a ConnectionStats struct for use with the tracer
func FlowToConnStat(cs *ConnectionStats, flow *driver.PerFlowData, enableMonotonicCounts bool) {
	var (
		family         ConnectionFamily
		srcAddr        util.Address
		dstAddr        util.Address
		connectionType ConnectionType
	)
	family = connFamily(flow.AddressFamily)
	connectionType = connType(flow.Protocol)

	// V4 Address
	if family == AFINET {
		srcAddr, dstAddr = convertV4Addr(flow.LocalAddress), convertV4Addr(flow.RemoteAddress)
	} else {
		// V6 Address
		srcAddr, dstAddr = convertV6Addr(flow.LocalAddress), convertV6Addr(flow.RemoteAddress)
	}

	cs.Source = srcAddr
	cs.Dest = dstAddr
	// after lengthy discussion, use the transport bytes in/out.  monotonic
	// RecvBytes/SentBytes includes the size of the IP header and transport
	// header, transportBytes is the raw transport data.  At present,
	// the linux probe only reports the raw transport data.  So do that by default.
	cs.MonotonicSentBytes = monotonicOrTransportBytes(enableMonotonicCounts, flow.MonotonicSentBytes, flow.TransportBytesOut)
	cs.MonotonicRecvBytes = monotonicOrTransportBytes(enableMonotonicCounts, flow.MonotonicRecvBytes, flow.TransportBytesIn)
	cs.MonotonicSentPackets = flow.PacketsOut
	cs.MonotonicRecvPackets = flow.PacketsIn
	cs.LastUpdateEpoch = flow.Timestamp
	cs.Pid = uint32(flow.ProcessId)
	cs.SPort = flow.LocalPort
	cs.DPort = flow.RemotePort
	cs.Type = connectionType
	cs.Family = family
	cs.Direction = connDirection(flow.Flags)
	cs.SPortIsEphemeral = isPortInEphemeralRange(cs.Family, cs.Type, cs.SPort)

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
		tf := flow.TCPFlow()
		if tf != nil {
			cs.MonotonicRetransmits = uint32(tf.RetransmitCount)
			cs.RTT = uint32(tf.SRTT)
			cs.RTTVar = uint32(tf.RttVariance)
		}

		if isTCPFlowEstablished(flow.Flags) {
			cs.MonotonicTCPEstablished = 1
		}
		if isFlowClosed(flow.Flags) {
			cs.MonotonicTCPClosed = 1
		}
	}
}
