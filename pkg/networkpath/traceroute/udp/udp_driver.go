// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package udp

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
)

type probeID struct {
	packetID uint16
	checksum uint16
}
type probeData struct {
	sendTime time.Time
	ttl      uint8
}

type udpDriver struct {
	config *UDPv4

	sink packets.Sink

	source packets.Source
	buffer []byte
	parser *packets.FrameParser

	// mu guards against concurrent access to sentProbes
	mu         sync.Mutex
	sentProbes map[probeID]probeData
}

//nolint:unused // This is used, but not on all platforms yet
func newUDPDriver(config *UDPv4, sink packets.Sink, source packets.Source) *udpDriver {
	return &udpDriver{
		config: config,

		sink: sink,

		source: source,
		buffer: make([]byte, 1024),
		parser: packets.NewFrameParser(),

		sentProbes: make(map[probeID]probeData),
	}
}

func (u *udpDriver) storeProbe(probeID probeID, data probeData) bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	// refuse to store it if we somehow would overwrite
	if _, ok := u.sentProbes[probeID]; ok {
		return false
	}

	u.sentProbes[probeID] = data
	return true
}

func (u *udpDriver) findMatchingProbe(probeID probeID) (probeData, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()

	data, ok := u.sentProbes[probeID]
	return data, ok
}

func (u *udpDriver) getLocalAddrPort() netip.AddrPort {
	addr, _ := common.UnmappedAddrFromSlice(u.config.srcIP)
	return netip.AddrPortFrom(addr, u.config.srcPort)

}

func (u *udpDriver) getTargetAddrPort() netip.AddrPort {
	addr, _ := common.UnmappedAddrFromSlice(u.config.Target)
	return netip.AddrPortFrom(addr, u.config.TargetPort)
}

var _ common.TracerouteDriver = &udpDriver{}

// GetDriverInfo returns metadata about this driver
func (u *udpDriver) GetDriverInfo() common.TracerouteDriverInfo {
	return common.TracerouteDriverInfo{
		SupportsParallel: true,
	}
}

// SendProbe sends a traceroute packet with a specific TTL
func (u *udpDriver) SendProbe(ttl uint8) error {
	id, buffer, checksum, err := u.config.createRawUDPBuffer(u.config.srcIP, u.config.srcPort, u.config.Target, u.config.TargetPort, int(ttl))
	if err != nil {
		return fmt.Errorf("udpDriver SendProbe failed to createRawUDPBuffer: %w", err)
	}

	probeID := probeID{packetID: id, checksum: checksum}
	data := probeData{sendTime: time.Now(), ttl: ttl}
	log.Tracef("sending probe with ttl=%d, packetID=%d, checksum=%d", ttl, id, checksum)
	ok := u.storeProbe(probeID, data)
	if !ok {
		return fmt.Errorf("udpDriver Sendprobe tried to sent the same probe ID twice for ttl=%d", ttl)
	}

	err = u.sink.WriteTo(buffer, u.getTargetAddrPort())
	if err != nil {
		return fmt.Errorf("tcpDriver SendProbe failed to write packet: %w", err)
	}
	return nil
}

// ReceiveProbe polls to get a traceroute response with a timeouu.
func (u *udpDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	err := u.source.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("tcpDriver failed to SetReadDeadline: %w", err)
	}
	err = packets.ReadAndParse(u.source, u.buffer, u.parser)
	if err != nil {
		return nil, err
	}

	return u.handleProbeLayers()
}

func (u *udpDriver) handleProbeLayers() (*common.ProbeResponse, error) {
	ipPair, err := u.parser.GetIPPair()
	if err != nil {
		return nil, fmt.Errorf("udpDriver failed to get IP pair: %w", err)
	}

	var probe probeData

	switch u.parser.GetTransportLayer() {
	case layers.LayerTypeICMPv4, layers.LayerTypeICMPv6:
		if !u.parser.IsTTLExceeded() && !u.parser.IsDestinationUnreachable() {
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		icmpInfo, err := u.parser.GetICMPInfo()
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("udpDriver failed to get ICMP info: %w", err)}
		}

		// make sure the source/destination match
		udpInfo, err := packets.ParseUDPFirstBytes(icmpInfo.Payload)
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("udpDriver failed to parse UDP info: %w", err)}
		}

		icmpSrc := netip.AddrPortFrom(icmpInfo.ICMPPair.SrcAddr, udpInfo.SrcPort)
		icmpDst := netip.AddrPortFrom(icmpInfo.ICMPPair.DstAddr, udpInfo.DstPort)
		local := u.getLocalAddrPort()
		target := u.getTargetAddrPort()

		if icmpDst != target {
			log.Tracef("udpDriver ignored ICMP packet which had another destination: expected=%s, actual=%s", target, icmpDst)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		if !u.config.LoosenICMPSrc && icmpSrc != local {
			log.Tracef("udpDriver ignored packet which had another source: expected=%s, actual=%s", local, icmpSrc)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		id := icmpInfo.WrappedPacketID
		if icmpDst.Addr().Is6() {
			id = udpInfo.ID
		}
		probeID := probeID{packetID: id, checksum: udpInfo.Checksum}
		probe, _ = u.findMatchingProbe(probeID)
		if probe == (probeData{}) {
			log.Warnf("couldn't find probe matching packetID=%d and checksum=%d", probeID.packetID, probeID.checksum)
		}
	default:
		return nil, common.ErrPacketDidNotMatchTraceroute
	}

	if probe == (probeData{}) {
		return nil, common.ErrPacketDidNotMatchTraceroute
	}
	rtt := time.Since(probe.sendTime)

	return &common.ProbeResponse{
		TTL:    probe.ttl,
		IP:     ipPair.SrcAddr,
		RTT:    rtt,
		IsDest: ipPair.SrcAddr == u.getTargetAddrPort().Addr(),
	}, nil
}
