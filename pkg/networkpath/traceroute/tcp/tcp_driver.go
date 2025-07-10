// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tcp

import (
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/packets"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
)

type probeData struct {
	sendTime time.Time
	ttl      uint8
	packetID uint16
	seqNum   uint32
}

type tcpDriver struct {
	config *TCPv4

	sink packets.Sink

	source packets.Source
	buffer []byte
	parser *packets.FrameParser

	// mu guards against concurrent use of sentProbes
	mu         sync.Mutex
	sentProbes []probeData

	// if CompatibilityMode is enabled, we randomize the packet ID starting from this base
	basePacketID uint16
	// if CompatibilityMode is enabled, we store a single seqNum for the duration of the traceroute
	seqNum uint32
}

var _ common.TracerouteDriver = &tcpDriver{}

func newTCPDriver(config *TCPv4, sink packets.Sink, source packets.Source) *tcpDriver {
	var basePacketID uint16
	var seqNum uint32
	if !config.ParisTracerouteMode {
		basePacketID = packets.AllocPacketID(config.MaxTTL)
		seqNum = rand.Uint32()
	}

	return &tcpDriver{
		config: config,

		sink: sink,

		source: source,
		buffer: make([]byte, 1024),
		parser: packets.NewFrameParser(),

		sentProbes: nil,

		basePacketID: basePacketID,
		seqNum:       seqNum,
	}
}

func (t *tcpDriver) storeProbe(probeData probeData) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sentProbes = append(t.sentProbes, probeData)
}

func (t *tcpDriver) findMatchingProbe(packetID uint16, seqNum uint32) probeData {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, probe := range t.sentProbes {
		if probe.packetID == packetID && probe.seqNum == seqNum {
			return probe
		}
	}
	return probeData{}
}
func (t *tcpDriver) getLastSentProbe() (probeData, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.sentProbes) == 0 {
		return probeData{}, fmt.Errorf("getLastSentProbe was called before we sent anything")
	}
	return t.sentProbes[len(t.sentProbes)-1], nil
}

func (t *tcpDriver) getLocalAddrPort() netip.AddrPort {
	addr, _ := common.UnmappedAddrFromSlice(t.config.srcIP)
	return netip.AddrPortFrom(addr, t.config.srcPort)

}

func (t *tcpDriver) getTargetAddrPort() netip.AddrPort {
	addr, _ := common.UnmappedAddrFromSlice(t.config.Target)
	return netip.AddrPortFrom(addr, t.config.DestPort)
}

// GetDriverInfo returns metadata about this driver
func (t *tcpDriver) GetDriverInfo() common.TracerouteDriverInfo {
	return common.TracerouteDriverInfo{
		SupportsParallel: false,
	}
}

func (t *tcpDriver) getNextPacketIDAndSeqNum(ttl uint8) (uint16, uint32) {
	if t.config.ParisTracerouteMode {
		return 41821, rand.Uint32()
	}
	return t.basePacketID + uint16(ttl), t.seqNum
}

// SendProbe sends a traceroute packet with a specific TTL
func (t *tcpDriver) SendProbe(ttl uint8) error {
	packetID, seqNum := t.getNextPacketIDAndSeqNum(ttl)
	_, buffer, _, err := t.config.createRawTCPSynBuffer(packetID, seqNum, int(ttl))
	if err != nil {
		return fmt.Errorf("tcpDriver SendProbe failed to createRawTCPSynBuffer: %w", err)
	}

	log.Tracef("sending probe with ttl=%d, packetID=%d, seqNum=%d", ttl, packetID, seqNum)
	t.storeProbe(probeData{
		sendTime: time.Now(),
		ttl:      ttl,
		packetID: packetID,
		seqNum:   seqNum,
	})

	err = t.sink.WriteTo(buffer, t.getTargetAddrPort())
	if err != nil {
		return fmt.Errorf("tcpDriver SendProbe failed to write packet: %w", err)
	}
	return nil
}

// ReceiveProbe polls to get a traceroute response with a timeout.
func (t *tcpDriver) ReceiveProbe(timeout time.Duration) (*common.ProbeResponse, error) {
	err := t.source.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return nil, fmt.Errorf("tcpDriver failed to SetReadDeadline: %w", err)
	}

	err = packets.ReadAndParse(t.source, t.buffer, t.parser)
	if err != nil {
		return nil, err
	}

	return t.handleProbeLayers()
}

func (t *tcpDriver) ExpectedIPPair() packets.IPPair {
	// from the target to us
	return packets.IPPair{
		SrcAddr: t.getTargetAddrPort().Addr(),
		DstAddr: t.getLocalAddrPort().Addr(),
	}
}

func (t *tcpDriver) handleProbeLayers() (*common.ProbeResponse, error) {
	ipPair, err := t.parser.GetIPPair()
	if err != nil {
		return nil, fmt.Errorf("tcpDriver failed to get IP pair: %w", err)
	}

	var probe probeData
	var isDest bool

	switch t.parser.GetTransportLayer() {
	case layers.LayerTypeTCP:
		isSynack := t.parser.TCP.SYN && t.parser.TCP.ACK
		isRst := t.parser.TCP.RST
		isRstAck := t.parser.TCP.RST && t.parser.TCP.ACK
		// we only care about SYNACK and RST/RSTACK
		if !isSynack && !isRst && !isRstAck {
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		if ipPair != t.ExpectedIPPair() {
			log.Tracef("tcpDriver ignored packet which had another IP pair: expected=%+v, actual=%+v", t.ExpectedIPPair(), ipPair)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		// make sure the ports match
		if t.config.DestPort != uint16(t.parser.TCP.SrcPort) {
			log.Tracef("tcpDriver ignored packet which had another srcPort: expected=%d, actual=%d", t.config.DestPort, t.parser.TCP.SrcPort)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}
		if t.config.srcPort != uint16(t.parser.TCP.DstPort) {
			log.Tracef("tcpDriver ignored packet which had another dstPort: expected=%d, actual=%d", t.config.srcPort, t.parser.TCP.DstPort)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		expectedSeq := t.parser.TCP.Ack - 1

		lastProbe, err := t.getLastSentProbe()
		if err != nil {
			return nil, fmt.Errorf("tcpDriver handleProbeLayers failed to getLastSentProbe: %w", err)
		}
		// if we got an ack flag, the sent seq number should match the received ack number
		if (isSynack || isRstAck) && lastProbe.seqNum != expectedSeq {
			// in paris mode, seq number is randomized so this warning would have false positives
			if !t.config.ParisTracerouteMode {
				log.Warnf("got ack flag but couldn't find probe matching seqNum=%d", expectedSeq)
			}
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		probe = lastProbe
		isDest = true
	case layers.LayerTypeICMPv4:
		if !t.parser.IsTTLExceeded() {
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		icmpInfo, err := t.parser.GetICMPInfo()
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("tcpDriver failed to get ICMP info: %w", err)}
		}

		// make sure the source/destination match
		tcpInfo, err := packets.ParseTCPFirstBytes(icmpInfo.Payload)
		if err != nil {
			return nil, &common.BadPacketError{Err: fmt.Errorf("tcpDriver failed to parse TCP info: %w", err)}
		}

		icmpSrc := netip.AddrPortFrom(icmpInfo.ICMPPair.SrcAddr, tcpInfo.SrcPort)
		icmpDst := netip.AddrPortFrom(icmpInfo.ICMPPair.DstAddr, tcpInfo.DstPort)
		local := t.getLocalAddrPort()
		target := t.getTargetAddrPort()

		if icmpDst != target {
			log.Tracef("tcpDriver ignored ICMP packet which had another destination: expected=%s, actual=%s", target, icmpDst)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		if !t.config.LoosenICMPSrc && icmpSrc != local {
			log.Tracef("tcpDriver ignored packet which had another source: expected=%s, actual=%s", local, icmpSrc)
			return nil, common.ErrPacketDidNotMatchTraceroute
		}

		probe = t.findMatchingProbe(icmpInfo.WrappedPacketID, tcpInfo.Seq)
		if probe == (probeData{}) {
			log.Warnf("couldn't find probe matching packetID=%d and seqNum=%d", icmpInfo.WrappedPacketID, tcpInfo.Seq)
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
		IsDest: isDest,
	}, nil
}

// Close closes the tcpDriver
func (t *tcpDriver) Close() error {
	sinkErr := t.sink.Close()
	sourceErr := t.source.Close()
	return errors.Join(sinkErr, sourceErr)
}
